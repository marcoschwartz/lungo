// Package reactgo provides a Next.js-like framework powered by Go.
package lungo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// safeJoin joins base + name and refuses paths that escape base via "..",
// absolute paths, or symlink-style tricks. Returns ("", false) on violation.
func safeJoin(base, name string) (string, bool) {
	if name == "" {
		return base, true
	}
	// Reject absolute paths and null bytes outright.
	if strings.ContainsRune(name, 0) || filepath.IsAbs(name) {
		return "", false
	}
	clean := filepath.Clean("/" + name)
	full := filepath.Join(base, clean)
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", false
	}
	absFull, err := filepath.Abs(full)
	if err != nil {
		return "", false
	}
	if absFull != absBase && !strings.HasPrefix(absFull, absBase+string(filepath.Separator)) {
		return "", false
	}
	return full, true
}

// contentETag returns a strong sha256-based ETag for the given bytes.
func contentETag(data []byte) string {
	sum := sha256.Sum256(data)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// Names Lungo makes available via window.Lungo. Transpiled JSX compiles to
// h(...) calls, so any module that omits h from its destructure will explode
// with "h is not defined" unless we inject it.
var lungoRuntimeNames = []string{
	"h", "Fragment", "useState", "useEffect", "useMemo", "useRef", "useRouter", "createPortal",
}

// lungoDestructureRE matches `const { ... } = window.Lungo;` (also let/var).
// Capture group 2 is the comma-separated name list inside the braces.
var lungoDestructureRE = regexp.MustCompile(`(const|let|var)\s*\{\s*([^}]+?)\s*\}\s*=\s*window\.Lungo\s*;?`)

// ensureLungoImports guarantees every runtime name is in scope. If the module
// already destructures from window.Lungo we merge in any missing names;
// otherwise we prepend a full destructure. This avoids the common footgun
// where `const { useRouter } = window.Lungo` leaves `h` undefined after JSX
// transpile.
func ensureLungoImports(src []byte) []byte {
	m := lungoDestructureRE.FindSubmatchIndex(src)
	if m == nil {
		header := "const { " + strings.Join(lungoRuntimeNames, ", ") + " } = window.Lungo;\n"
		return append([]byte(header), src...)
	}
	existing := string(src[m[4]:m[5]])
	have := map[string]bool{}
	for _, n := range strings.Split(existing, ",") {
		have[strings.TrimSpace(n)] = true
	}
	var missing []string
	for _, n := range lungoRuntimeNames {
		if !have[n] {
			missing = append(missing, n)
		}
	}
	if len(missing) == 0 {
		return src
	}
	merged := existing + ", " + strings.Join(missing, ", ")
	out := make([]byte, 0, len(src)+len(merged)-len(existing))
	out = append(out, src[:m[4]]...)
	out = append(out, []byte(merged)...)
	out = append(out, src[m[5]:]...)
	return out
}

// Middleware is a function that wraps an http.Handler.
type Middleware func(http.Handler) http.Handler

// Options configures the Lungo application.
type Options struct {
	// AppDir is the path to the app/ directory (used in dev mode with filesystem).
	AppDir string

	// StaticDir is the path to the static/ directory (used in dev mode with filesystem).
	StaticDir string

	// AppFS is an embedded filesystem for app/ files (used in production).
	AppFS fs.FS

	// StaticFS is an embedded filesystem for static/ files (used in production).
	StaticFS fs.FS

	// Dev enables hot module replacement and verbose logging.
	Dev bool

	// Cache configures page-level HTML caching.
	// If nil, all pages are rendered live (SSR) on every request.
	Cache *CacheOptions

	// DefaultTheme sets the SSR theme when no cookie is present.
	// "dark" or "light" (default: "light").
	DefaultTheme string

	// HeadExtra is raw HTML injected into <head> (e.g. extra <script> or <link> tags).
	HeadExtra string
}

// CacheOptions configures page-level HTML caching.
type CacheOptions struct {
	// DefaultMode is the default caching mode for pages.
	// "static" = cache forever until revalidated
	// "isr" = cache with TTL + stale-while-revalidate
	// "ssr" = always render fresh (default if empty)
	DefaultMode string

	// DefaultTTL is the default TTL in seconds for ISR mode.
	DefaultTTL int

	// Rules defines per-path caching rules. More specific rules take priority.
	Rules []CacheRule

	// RevalidateSecret is the secret required to call /__revalidate.
	// If empty, the endpoint is disabled.
	RevalidateSecret string
}

// CacheRule configures caching for a specific path pattern.
type CacheRule struct {
	// Path pattern. Exact paths or wildcard with trailing *.
	// Examples: "/", "/about", "/blog/*"
	Path string

	// Mode: "static", "isr", or "ssr"
	Mode string

	// TTL in seconds for ISR mode. 0 = use DefaultTTL.
	TTL int
}

// App is the main Lungo application instance.
type App struct {
	opts        Options
	router      *Router
	handler     http.Handler
	apiRoutes   map[string]http.HandlerFunc
	middlewares []Middleware
	hmr         *HMR
	htmlCache   map[string]*htmlCacheEntry
	htmlCacheMu sync.RWMutex
	jsxCache    map[string]jsxCacheEntry
	jsxCacheMu  sync.RWMutex
	publicEnv   map[string]string // LUNGO_PUBLIC_* env vars exposed to JSX
}

type htmlCacheEntry struct {
	html      string
	cachedAt  time.Time
	ttl       time.Duration // 0 = forever (static mode)
}

// jsxCacheEntry caches transpiled JSX so we don't re-run the transpiler on every request.
type jsxCacheEntry struct {
	data    []byte
	etag    string
	modTime time.Time
}

// loadEnvFile loads environment variables from a .env file.
// Variables already set in the environment are NOT overwritten.
// Supports KEY=VALUE, KEY="VALUE", KEY='VALUE', and # comments.
func loadEnvFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		// Strip surrounding quotes
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		// Don't overwrite existing env vars
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}

// New creates a new Lungo application.
// Automatically loads .env file if present (like Next.js).
func New(opts Options) *App {
	// Load .env files (don't overwrite existing env vars)
	loadEnvFile(".env.local")
	loadEnvFile(".env")

	if opts.AppDir == "" {
		opts.AppDir = "./app"
	}
	abs, err := filepath.Abs(opts.AppDir)
	if err == nil {
		opts.AppDir = abs
	}
	if opts.StaticDir == "" {
		opts.StaticDir = "./static"
	}
	absStatic, err := filepath.Abs(opts.StaticDir)
	if err == nil {
		opts.StaticDir = absStatic
	}

	// Collect LUNGO_PUBLIC_* env vars for client/SSR access
	publicEnv := make(map[string]string)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "LUNGO_PUBLIC_") {
			if i := strings.IndexByte(kv, '='); i > 0 {
				publicEnv[kv[:i]] = kv[i+1:]
			}
		}
	}

	app := &App{
		opts:      opts,
		apiRoutes: make(map[string]http.HandlerFunc),
		htmlCache: make(map[string]*htmlCacheEntry),
		jsxCache:  make(map[string]jsxCacheEntry),
		publicEnv: publicEnv,
	}

	if opts.AppFS != nil {
		app.router = NewRouterFromFS(opts.AppFS)
	} else {
		app.router = NewRouter(opts.AppDir)
	}

	if opts.Dev {
		app.hmr = NewHMR(opts.AppDir, func() {
			app.router.Rescan()
		})
	}

	return app
}

// Use adds middleware to the application. Middleware runs in the order added,
// wrapping the core handler. Use this for auth, CORS, rate limiting, etc.
func (a *App) Use(mw Middleware) {
	a.middlewares = append(a.middlewares, mw)
	a.handler = nil // force rebuild
}

// API registers an API route handler.
func (a *App) API(pattern string, handler http.HandlerFunc) {
	a.apiRoutes[pattern] = handler
}

// Handler returns the http.Handler for embedding in existing servers.
func (a *App) Handler() http.Handler {
	if a.handler == nil {
		a.handler = a.buildHandler()
	}
	return a.handler
}

// ListenAndServe starts the HTTP server.
func (a *App) ListenAndServe(addr string) error {
	if a.opts.Dev {
		fmt.Printf("Lungo dev server running at http://localhost%s\n", addr)
	} else {
		fmt.Printf("Lungo server running at http://localhost%s\n", addr)
	}
	return http.ListenAndServe(addr, a.Handler())
}

// readAppFile reads a file from embedded FS or disk.
// Falls back to .jsx if .js is not found.
func (a *App) readAppFile(name string) ([]byte, error) {
	if a.opts.AppFS != nil {
		data, err := fs.ReadFile(a.opts.AppFS, name)
		if err != nil && strings.HasSuffix(name, ".js") {
			return fs.ReadFile(a.opts.AppFS, strings.TrimSuffix(name, ".js")+".jsx")
		}
		return data, err
	}
	data, err := os.ReadFile(filepath.Join(a.opts.AppDir, name))
	if err != nil && strings.HasSuffix(name, ".js") {
		return os.ReadFile(filepath.Join(a.opts.AppDir, strings.TrimSuffix(name, ".js")+".jsx"))
	}
	return data, err
}

// hasAppFile checks if a file exists in the app directory.
func (a *App) hasAppFile(name string) bool {
	if a.opts.AppFS != nil {
		_, err := fs.Stat(a.opts.AppFS, name)
		return err == nil
	}
	_, err := os.Stat(filepath.Join(a.opts.AppDir, name))
	return err == nil
}

func (a *App) buildHandler() http.Handler {
	core := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// 1. Runtime JS
		if path == "/runtime/lungo.js" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/javascript")
			if a.opts.Dev {
				w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			} else {
				etag := contentETag(runtimeJS)
				w.Header().Set("Cache-Control", "public, max-age=60, must-revalidate")
				w.Header().Set("ETag", etag)
				if r.Header.Get("If-None-Match") == etag {
					w.WriteHeader(http.StatusNotModified)
					return
				}
			}
			w.Write(runtimeJS)
			return
		}

		// 2. HMR WebSocket (dev only)
		if path == "/__hmr" && a.hmr != nil {
			a.hmr.ServeWS(w, r)
			return
		}

		// 3. Revalidation endpoint
		if path == "/__revalidate" && r.Method == http.MethodPost {
			a.handleRevalidate(w, r)
			return
		}

		// 4. API routes
		for pattern, handler := range a.apiRoutes {
			if path == pattern {
				handler(w, r)
				return
			}
		}

		// 4. Static files
		if r.Method == http.MethodGet && strings.HasPrefix(path, "/static/") {
			a.serveStatic(w, r, strings.TrimPrefix(path, "/static/"))
			return
		}

		// 5. App JS files
		if r.Method == http.MethodGet && strings.HasPrefix(path, "/app/") {
			a.serveAppFile(w, r, strings.TrimPrefix(path, "/app/"))
			return
		}

		// 6. Loader data endpoint
		if r.Method == http.MethodGet && strings.HasPrefix(path, "/_data/") {
			a.serveLoaderData(w, r)
			return
		}

		// 6b. Server-rendered page fragment for client-side navigation
		if r.Method == http.MethodGet && strings.HasPrefix(path, "/_page/") {
			a.servePageFragment(w, r)
			return
		}

		// 7. Skip static file extensions
		if r.Method == http.MethodGet && filepath.Ext(path) != "" && path != "/" {
			http.NotFound(w, r)
			return
		}

		// 8. SSR with error recovery
		if r.Method == http.MethodGet {
			a.serveSSRWithRecovery(w, r)
			return
		}

		http.NotFound(w, r)
	})

	// Apply middleware in reverse order (first added = outermost)
	var handler http.Handler = core
	for i := len(a.middlewares) - 1; i >= 0; i-- {
		handler = a.middlewares[i](handler)
	}

	// Always add recovery and logging
	handler = recoveryMiddleware(handler, a)
	if a.opts.Dev {
		handler = devLogMiddleware(handler)
	}

	return handler
}

func (a *App) serveStatic(w http.ResponseWriter, r *http.Request, name string) {
	if a.opts.StaticFS != nil {
		data, err := fs.ReadFile(a.opts.StaticFS, name)
		if err == nil {
			w.Header().Set("Content-Type", detectContentType(name))
			if a.opts.Dev {
				w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			} else {
				w.Header().Set("Cache-Control", "public, max-age=60, must-revalidate")
			}
			w.Write(data)
			return
		}
	} else {
		filePath, ok := safeJoin(a.opts.StaticDir, name)
		if !ok {
			http.NotFound(w, r)
			return
		}
		data, err := os.ReadFile(filePath)
		if err == nil {
			w.Header().Set("Content-Type", detectContentType(name))
			if a.opts.Dev {
				w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			} else {
				w.Header().Set("Cache-Control", "public, max-age=60, must-revalidate")
			}
			w.Write(data)
			return
		}
	}
	http.NotFound(w, r)
}

func (a *App) serveAppFile(w http.ResponseWriter, r *http.Request, name string) {
	// In-memory cache keyed by requested name. Invalidated by file modtime
	// (disk) or served forever (embedded FS, since bytes are immutable).
	cached, ok := a.jsxCacheGet(name)
	var data []byte
	var etag string
	var modTime time.Time

	if ok && a.opts.AppFS != nil {
		data, etag = cached.data, cached.etag
	} else {
		// Resolve + read the file, falling back from .js to .jsx.
		var err error
		var resolved string
		data, resolved, modTime, err = a.readWithFallback(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		if ok && !modTime.IsZero() && cached.modTime.Equal(modTime) {
			data, etag = cached.data, cached.etag
		} else {
			needsTranspile := strings.HasSuffix(resolved, ".jsx")
			if needsTranspile {
				result, jsxErrors := TranspileJSXWithErrors(string(data))
				if len(jsxErrors) > 0 && a.opts.Dev {
					log.Printf("[Lungo] JSX errors in %s:", name)
					for _, e := range jsxErrors {
						log.Printf("  - %s", e)
					}
					errorJS := "console.error('[Lungo JSX Error] " + strings.ReplaceAll(jsxErrors[0], "'", "\\'") + "');\n"
					data = []byte(errorJS + result)
				} else {
					data = []byte(result)
				}
				data = ensureLungoImports(data)
			}
			etag = contentETag(data)
			a.jsxCachePut(name, jsxCacheEntry{data: data, etag: etag, modTime: modTime})
		}
	}

	w.Header().Set("Content-Type", "application/javascript")
	if a.opts.Dev {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=60, must-revalidate")
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	w.Write(data)
}

// readWithFallback resolves a request path against AppFS/AppDir, falling back
// from .js to .jsx. It refuses paths that escape AppDir.
// Returns (contents, resolved name, modtime, err). modtime is zero for AppFS.
func (a *App) readWithFallback(name string) ([]byte, string, time.Time, error) {
	tryNames := []string{name}
	if strings.HasSuffix(name, ".js") {
		tryNames = append(tryNames, strings.TrimSuffix(name, ".js")+".jsx")
	}
	if a.opts.AppFS != nil {
		for _, n := range tryNames {
			if data, err := fs.ReadFile(a.opts.AppFS, n); err == nil {
				return data, n, time.Time{}, nil
			}
		}
		return nil, "", time.Time{}, os.ErrNotExist
	}
	for _, n := range tryNames {
		full, ok := safeJoin(a.opts.AppDir, n)
		if !ok {
			return nil, "", time.Time{}, os.ErrPermission
		}
		st, err := os.Stat(full)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		return data, n, st.ModTime(), nil
	}
	return nil, "", time.Time{}, os.ErrNotExist
}

func (a *App) jsxCacheGet(name string) (jsxCacheEntry, bool) {
	a.jsxCacheMu.RLock()
	defer a.jsxCacheMu.RUnlock()
	e, ok := a.jsxCache[name]
	return e, ok
}

func (a *App) jsxCachePut(name string, e jsxCacheEntry) {
	a.jsxCacheMu.Lock()
	a.jsxCache[name] = e
	a.jsxCacheMu.Unlock()
}

func (a *App) serveSSRWithRecovery(w http.ResponseWriter, r *http.Request) {
	route := a.router.Match(r.URL.Path)
	if route == nil {
		a.serveNotFound(w, r)
		return
	}

	// Check page cache
	if a.opts.Cache != nil && !a.opts.Dev {
		mode, ttl := a.resolveCacheMode(r.URL.Path, route)
		if mode == "static" || mode == "isr" {
			if html, ok := a.getHTMLCache(r.URL.Path, mode); ok {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("X-Lungo-Cache", "HIT")
				w.Write([]byte(html))
				return
			}
			// Cache miss — render, cache, serve
			html := a.renderPageFull(route, r)
			a.setHTMLCache(r.URL.Path, html, ttl)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("X-Lungo-Cache", "MISS")
			w.Write([]byte(html))
			return
		}
	}

	a.serveSSR(w, r)
}

// resolveCacheMode determines the cache mode and TTL for a path.
func (a *App) resolveCacheMode(urlPath string, route *Route) (string, time.Duration) {
	if a.opts.Cache == nil {
		return "ssr", 0
	}

	// Check specific rules (most specific first)
	var bestMatch *CacheRule
	bestLen := -1
	for i := range a.opts.Cache.Rules {
		rule := &a.opts.Cache.Rules[i]
		if rule.Path == urlPath {
			bestMatch = rule
			break // exact match wins
		}
		if strings.HasSuffix(rule.Path, "/*") {
			prefix := strings.TrimSuffix(rule.Path, "*")
			if strings.HasPrefix(urlPath, prefix) && len(prefix) > bestLen {
				bestMatch = rule
				bestLen = len(prefix)
			}
		}
	}

	if bestMatch != nil {
		ttl := time.Duration(bestMatch.TTL) * time.Second
		if bestMatch.Mode == "isr" && ttl == 0 {
			ttl = time.Duration(a.opts.Cache.DefaultTTL) * time.Second
		}
		return bestMatch.Mode, ttl
	}

	// Default mode
	mode := a.opts.Cache.DefaultMode
	if mode == "" {
		mode = "ssr"
	}
	if mode == "isr" {
		return mode, time.Duration(a.opts.Cache.DefaultTTL) * time.Second
	}
	return mode, 0
}

// getPageCache returns cached HTML if available and fresh.
func (a *App) getHTMLCache(urlPath, mode string) (string, bool) {
	a.htmlCacheMu.RLock()
	entry, ok := a.htmlCache[urlPath]
	a.htmlCacheMu.RUnlock()
	if !ok {
		return "", false
	}

	if mode == "static" {
		// Static: cached forever until revalidated
		return entry.html, true
	}

	// ISR: check TTL
	if entry.ttl > 0 && time.Since(entry.cachedAt) > entry.ttl {
		// Stale — serve stale, revalidate in background
		go func() {
			route := a.router.Match(urlPath)
			if route != nil {
				html := a.renderPageFull(route, nil)
				a.setHTMLCache(urlPath, html, entry.ttl)
				log.Printf("[Cache] revalidated %s in background", urlPath)
			}
		}()
		return entry.html, true // serve stale
	}

	return entry.html, true
}

// setPageCache stores rendered HTML in the cache.
func (a *App) setHTMLCache(urlPath, html string, ttl time.Duration) {
	a.htmlCacheMu.Lock()
	a.htmlCache[urlPath] = &htmlCacheEntry{
		html:     html,
		cachedAt: time.Now(),
		ttl:      ttl,
	}
	a.htmlCacheMu.Unlock()
	log.Printf("[Cache] cached %s (ttl=%v)", urlPath, ttl)
}

// renderPageFull renders a page to full HTML string.
func (a *App) renderPageFull(route *Route, r *http.Request) string {
	var loaderData json.RawMessage
	if route.HasLoader && route.LoaderURL != "" {
		loaderData = a.fetchLoaderData(route, r)
	}
	return a.renderPage(route, loaderData, r)
}

// handleRevalidate handles POST /__revalidate requests.
func (a *App) handleRevalidate(w http.ResponseWriter, r *http.Request) {
	if a.opts.Cache == nil || a.opts.Cache.RevalidateSecret == "" {
		http.NotFound(w, r)
		return
	}

	// Check auth
	auth := r.Header.Get("Authorization")
	if auth != "Bearer "+a.opts.Cache.RevalidateSecret {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		return
	}

	w.Header().Set("Content-Type", "application/json")

	var revalidated []string

	// Revalidate specific paths
	paths := r.URL.Query()["path"]
	for _, p := range paths {
		a.htmlCacheMu.Lock()
		delete(a.htmlCache, p)
		a.htmlCacheMu.Unlock()
		revalidated = append(revalidated, p)
	}

	// Revalidate by pattern
	patterns := r.URL.Query()["pattern"]
	for _, pattern := range patterns {
		prefix := strings.TrimSuffix(pattern, "*")
		a.htmlCacheMu.Lock()
		for path := range a.htmlCache {
			if strings.HasPrefix(path, prefix) {
				delete(a.htmlCache, path)
				revalidated = append(revalidated, path)
			}
		}
		a.htmlCacheMu.Unlock()
	}

	// "all" flag clears everything
	if r.URL.Query().Get("all") == "true" {
		a.htmlCacheMu.Lock()
		count := len(a.htmlCache)
		a.htmlCache = make(map[string]*htmlCacheEntry)
		a.htmlCacheMu.Unlock()
		log.Printf("[Cache] revalidated all (%d pages)", count)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"revalidated": count,
			"all":         true,
			"ok":          true,
		})
		return
	}

	log.Printf("[Cache] revalidated: %v", revalidated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"revalidated": revalidated,
		"ok":          true,
	})
}

func detectContentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".css"):
		return "text/css"
	case strings.HasSuffix(name, ".js"):
		return "application/javascript"
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	case strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(name, ".webp"):
		return "image/webp"
	case strings.HasSuffix(name, ".woff2"):
		return "font/woff2"
	case strings.HasSuffix(name, ".woff"):
		return "font/woff"
	case strings.HasSuffix(name, ".json"):
		return "application/json"
	default:
		return "application/octet-stream"
	}
}

func devLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

// recoveryMiddleware catches panics and serves an error page.
func recoveryMiddleware(next http.Handler, app *App) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("[Lungo] PANIC: %v", err)
				app.serveError(w, r, fmt.Sprintf("%v", err))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
