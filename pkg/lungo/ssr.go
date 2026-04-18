package lungo

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"html"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// servePageFragment renders just the page HTML (no shell/layout) for client-side navigation.
// This is similar to Next.js RSC payloads — the server renders the component and sends HTML
// so the client never needs to execute server-component code.
func (a *App) servePageFragment(w http.ResponseWriter, r *http.Request) {
	pagePath := strings.TrimPrefix(r.URL.Path, "/_page")
	if pagePath == "" {
		pagePath = "/"
	}

	route := a.router.Match(pagePath)
	if route == nil {
		http.NotFound(w, r)
		return
	}

	// Fetch loader data if the page has a loader
	var loaderData json.RawMessage
	if route.HasLoader && route.LoaderURL != "" {
		loaderData = a.fetchLoaderData(route, r)
	}

	// SSR-render just the page component
	pageHTML, _, err := a.evaluatePageSSR(route.PagePath, loaderData, route.Segments)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"` + err.Error() + `"}`))
		return
	}

	// Return JSON with the rendered HTML, loader data, and metadata so the
	// client can update <title> / <meta name=description> on nav.
	response := struct {
		HTML string          `json:"html"`
		Data json.RawMessage `json:"data,omitempty"`
		Meta *PageMetadata   `json:"meta,omitempty"`
	}{
		HTML: pageHTML,
		Data: loaderData,
		Meta: a.extractMetadata(route.PagePath),
	}

	w.Header().Set("Content-Type", "application/json")
	if !a.opts.Dev {
		w.Header().Set("Cache-Control", "public, max-age=5, stale-while-revalidate=30")
	}
	json.NewEncoder(w).Encode(response)
}

func (a *App) serveSSR(w http.ResponseWriter, r *http.Request) {
	route := a.router.Match(r.URL.Path)
	if route == nil {
		http.NotFound(w, r)
		return
	}

	if r.Header.Get("Accept") == "text/html; streaming" || r.URL.Query().Get("stream") == "1" {
		a.serveStreamingSSR(w, r, route)
		return
	}

	var loaderData json.RawMessage
	if route.HasLoader && route.LoaderURL != "" {
		loaderData = a.fetchLoaderData(route, r)
	}

	html := a.renderPage(route, loaderData, r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// pageURL converts a relative path like "about/page.js" to "/app/about/page.js"
func pageURL(relPath string) string {
	return "/app/" + relPath
}

// tagOpenRE extracts opening element tag names from HTML.
// It ignores close tags (</...), doctype, and comments (<!...).
var tagOpenRE = regexp.MustCompile(`<([a-zA-Z][a-zA-Z0-9-]*)`)

// ssrTagHash computes an FNV-1a 32-bit hash over the sequence of opening tag
// names in the SSR HTML. The client computes the same hash over the live DOM
// after hydration and warns on mismatch — catching silent hydration drift
// (e.g. components that branch on Date.now() or random input).
func ssrTagHash(htmlStr string) string {
	h := fnv.New32a()
	for _, m := range tagOpenRE.FindAllStringSubmatch(htmlStr, -1) {
		h.Write([]byte(strings.ToLower(m[1])))
		h.Write([]byte{','})
	}
	return fmt.Sprintf("%x", h.Sum32())
}

func (a *App) renderPage(route *Route, loaderData json.RawMessage, r *http.Request) string {
	var sb strings.Builder

	// Extract metadata from page
	meta := a.extractMetadata(route.PagePath)
	// Fall back to layout metadata if page has none
	if (meta == nil || meta.Title == "") && len(route.Layouts) > 0 {
		for _, layoutPath := range route.Layouts {
			layoutMeta := a.extractMetadata(layoutPath)
			if layoutMeta != nil && layoutMeta.Title != "" {
				if meta == nil {
					meta = layoutMeta
				} else {
					if meta.Title == "" {
						meta.Title = layoutMeta.Title
					}
					if meta.Description == "" {
						meta.Description = layoutMeta.Description
					}
				}
			}
		}
	}

	// Read theme cookie for SSR — render with correct theme to prevent flash
	isDark := a.opts.DefaultTheme == "dark"
	if r != nil {
		if c, err := r.Cookie("theme"); err == nil {
			isDark = c.Value == "dark"
		}
	}
	if isDark {
		sb.WriteString("<!DOCTYPE html>\n<html lang=\"en\" class=\"dark\">\n")
	} else {
		sb.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n")
	}
	sb.WriteString("<script>(function(){var t=localStorage.getItem('theme');if(t==='dark'||(!t&&matchMedia('(prefers-color-scheme:dark)').matches)){document.documentElement.classList.add('dark');document.cookie='theme=dark;path=/;max-age=31536000'}else{document.cookie='theme=light;path=/;max-age=31536000'}})()</script>\n")
	sb.WriteString("<head>\n")
	sb.WriteString("  <meta charset=\"UTF-8\">\n")
	sb.WriteString("  <meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n")
	if meta != nil && meta.Title != "" {
		sb.WriteString(fmt.Sprintf("  <title>%s</title>\n", html.EscapeString(meta.Title)))
	} else {
		sb.WriteString("  <title>Lungo App</title>\n")
	}
	if meta != nil && meta.Description != "" {
		sb.WriteString(fmt.Sprintf("  <meta name=\"description\" content=\"%s\">\n", html.EscapeString(meta.Description)))
	}
	sb.WriteString("  <link rel=\"stylesheet\" href=\"/static/styles.css\">\n")
	if a.opts.HeadExtra != "" {
		sb.WriteString(a.opts.HeadExtra)
		sb.WriteString("\n")
	}
	sb.WriteString("</head>\n<body>\n")

	sb.WriteString(`<div id="root">`)
	ssrHTML, _, ssrErr := a.evaluatePageSSR(route.PagePath, loaderData, route.Segments)
	hasSSR := ssrErr == nil && ssrHTML != ""

	// Dev mode: show error overlay for JSX/SSR errors
	if ssrErr != nil && a.opts.Dev && strings.Contains(ssrErr.Error(), "JSX error") {
		return renderDevErrorOverlay("JSX Error", []string{ssrErr.Error()}, route.PagePath)
	}

	var layoutDataMap map[string]json.RawMessage
	var innerHTML string
	if hasSSR {
		ssrHTML, layoutDataMap = a.wrapInLayoutsWithData(ssrHTML, route, isDark, r)
		innerHTML = ssrHTML
	} else {
		innerHTML = a.renderLayoutShell(route)
	}
	sb.WriteString(innerHTML)
	sb.WriteString("</div>\n\n")

	sb.WriteString("<script>\n")
	if a.opts.Dev {
		sb.WriteString("  window.__LUNGO_DEV__ = true;\n")
	}
	if hasSSR {
		sb.WriteString(fmt.Sprintf("  window.__LUNGO_SSR_HASH__ = %q;\n", ssrTagHash(innerHTML)))
	}

	// Inject LUNGO_PUBLIC_* env vars for client-side process.env access
	if len(a.publicEnv) > 0 {
		envJSON, _ := json.Marshal(a.publicEnv)
		sb.WriteString(fmt.Sprintf("  window.__LUNGO_ENV__ = %s;\n", envJSON))
	} else {
		sb.WriteString("  window.__LUNGO_ENV__ = {};\n")
	}

	routeInfo := map[string]interface{}{
		"pattern":  route.Pattern,
		"params":   route.Segments,
		"pagePath": pageURL(route.PagePath),
	}
	routeJSON, _ := json.Marshal(routeInfo)
	sb.WriteString(fmt.Sprintf("  window.__LUNGO_ROUTE__ = %s;\n", routeJSON))

	if loaderData != nil {
		sb.WriteString(fmt.Sprintf("  window.__LUNGO_DATA__ = %s;\n", loaderData))
	}

	allRoutes := a.buildClientRoutes()
	routesJSON, _ := json.Marshal(allRoutes)
	sb.WriteString(fmt.Sprintf("  window.__LUNGO_ROUTES__ = %s;\n", routesJSON))

	var layoutURLs []string
	for _, l := range route.Layouts {
		layoutURLs = append(layoutURLs, pageURL(l))
	}
	layoutsJSON, _ := json.Marshal(layoutURLs)
	sb.WriteString(fmt.Sprintf("  window.__LUNGO_LAYOUTS__ = %s;\n", layoutsJSON))

	// Embed layout loader data for client hydration
	if len(layoutDataMap) > 0 {
		clientLayoutData := make(map[string]json.RawMessage)
		for path, data := range layoutDataMap {
			clientLayoutData[pageURL(path)] = data
		}
		ldJSON, _ := json.Marshal(clientLayoutData)
		sb.WriteString(fmt.Sprintf("  window.__LUNGO_LAYOUT_DATA__ = %s;\n", ldJSON))
	}

	sb.WriteString("</script>\n\n")

	sb.WriteString(`<script src="/runtime/lungo.js"></script>`)
	sb.WriteString("\n")

	sb.WriteString("<script type=\"module\">\n")
	sb.WriteString("  const { h, render, hydrate, RouterView } = window.Lungo;\n\n")

	// Import initial page and layouts so first render is instant (no async fetch)
	sb.WriteString(fmt.Sprintf("  const initialPage = await import('%s');\n", pageURL(route.PagePath)))

	for i, l := range route.Layouts {
		sb.WriteString(fmt.Sprintf("  const layout%d = await import('%s');\n", i, pageURL(l)))
	}

	// Set initial state so RouterView doesn't re-fetch on first render
	sb.WriteString("\n  // Set initial page so RouterView uses it immediately\n")
	sb.WriteString("  window.__LUNGO_INITIAL_PATH__ = window.location.pathname;\n")
	sb.WriteString("  window.Lungo.__setInitialPage = function(Page, data) {\n")
	sb.WriteString("    window.Lungo.__initialPage = Page;\n")
	sb.WriteString("    window.Lungo.__initialData = data;\n")
	sb.WriteString("  };\n")
	sb.WriteString("  window.Lungo.__setInitialPage(initialPage.default, window.__LUNGO_DATA__ || {});\n\n")

	// Collect layout components with their loader data
	sb.WriteString("  const layoutData = window.__LUNGO_LAYOUT_DATA__ || {};\n")
	sb.WriteString("  const layouts = [")
	for i, l := range route.Layouts {
		if i > 0 {
			sb.WriteString(", ")
		}
		lURL := pageURL(l)
		sb.WriteString(fmt.Sprintf("{component: layout%d.default, data: layoutData['%s'] || null}", i, lURL))
	}
	sb.WriteString("];\n\n")

	if hasSSR {
		sb.WriteString("  hydrate(h`<${RouterView} layouts=${layouts} />`, document.getElementById('root'));\n")
	} else {
		sb.WriteString("  render(h`<${RouterView} layouts=${layouts} />`, document.getElementById('root'));\n")
	}
	sb.WriteString("</script>\n</body>\n</html>")

	return sb.String()
}

func (a *App) renderLayoutShell(route *Route) string {
	return `<div data-hydrate="app"></div>`
}

type clientRoute struct {
	Pattern     string `json:"pattern"`
	PagePath    string `json:"pagePath"`
	LoadingPath string `json:"loadingPath,omitempty"`
}

func (a *App) buildClientRoutes() []clientRoute {
	var routes []clientRoute
	for _, pattern := range a.router.Routes() {
		r := a.router.Match(patternToExample(pattern))
		if r != nil {
			cr := clientRoute{
				Pattern:  pattern,
				PagePath: pageURL(r.PagePath),
			}
			if r.LoadingPath != "" {
				cr.LoadingPath = pageURL(r.LoadingPath)
			}
			routes = append(routes, cr)
		}
	}
	return routes
}

func patternToExample(pattern string) string {
	parts := strings.Split(pattern, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, ":") {
			parts[i] = "__example__"
		}
	}
	return strings.Join(parts, "/")
}

// ─── ISR Cache ──────────────────────────────────────────────────

type cacheEntry struct {
	data      json.RawMessage
	fetchedAt time.Time
	ttl       time.Duration
}

var (
	loaderCache   = make(map[string]*cacheEntry)
	loaderCacheMu sync.RWMutex
)

func getCached(key string) (json.RawMessage, bool) {
	loaderCacheMu.RLock()
	entry, ok := loaderCache[key]
	loaderCacheMu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Since(entry.fetchedAt) > entry.ttl {
		return entry.data, false // stale — return data but signal revalidation needed
	}
	return entry.data, true
}

func setCache(key string, data json.RawMessage, ttl time.Duration) {
	loaderCacheMu.Lock()
	loaderCache[key] = &cacheEntry{data: data, fetchedAt: time.Now(), ttl: ttl}
	loaderCacheMu.Unlock()
}

// ─── Loader Data Fetching ───────────────────────────────────────

func (a *App) fetchLoaderData(route *Route, r *http.Request) json.RawMessage {
	// Multi-source loaders: fetch all in parallel, merge into { key: data, ... }
	if len(route.Loaders) > 1 {
		return a.fetchMultiLoaderData(route, r)
	}

	// Single-source loader
	source := LoaderSource{URL: route.LoaderURL}
	if len(route.Loaders) == 1 {
		source = route.Loaders[0]
	}

	return a.fetchSingleSource(source, route.Segments, r)
}

func (a *App) fetchMultiLoaderData(route *Route, r *http.Request) json.RawMessage {
	type result struct {
		key  string
		data json.RawMessage
	}

	results := make(chan result, len(route.Loaders))
	var wg sync.WaitGroup

	for _, src := range route.Loaders {
		wg.Add(1)
		go func(s LoaderSource) {
			defer wg.Done()
			data := a.fetchSingleSource(s, route.Segments, r)
			results <- result{key: s.Key, data: data}
		}(src)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// Merge results into a single object
	merged := make(map[string]json.RawMessage)
	for res := range results {
		if res.data != nil {
			merged[res.key] = res.data
		} else {
			merged[res.key] = json.RawMessage("{}")
		}
	}

	out, _ := json.Marshal(merged)
	return out
}

func (a *App) fetchSingleSource(source LoaderSource, segments map[string]string, r *http.Request) json.RawMessage {
	url := source.URL
	for k, v := range segments {
		url = strings.ReplaceAll(url, ":"+k, v)
		url = strings.ReplaceAll(url, "{"+k+"}", v)
	}

	// Append segments as query params if URL didn't have placeholders
	if len(segments) > 0 && url == source.URL {
		sep := "?"
		if strings.Contains(url, "?") {
			sep = "&"
		}
		for k, v := range segments {
			url += sep + k + "=" + v
			sep = "&"
		}
	}

	// Check ISR cache
	if source.Revalidate > 0 {
		cacheKey := url
		cached, fresh := getCached(cacheKey)
		if fresh {
			return cached // serve from cache
		}
		if cached != nil {
			// Stale — serve cached, revalidate in background
			go func() {
				data := a.callHandler(source.URL, url, r)
				if data != nil {
					setCache(cacheKey, data, time.Duration(source.Revalidate)*time.Second)
				}
			}()
			return cached
		}
		// No cache — fetch, cache, return
		data := a.callHandler(source.URL, url, r)
		if data != nil {
			setCache(cacheKey, data, time.Duration(source.Revalidate)*time.Second)
		}
		return data
	}

	return a.callHandler(source.URL, url, r)
}

func (a *App) callHandler(pattern, url string, r *http.Request) json.RawMessage {
	// Loader URLs often carry query strings like `/api/page?slug=home`. Strip
	// before looking up so they match the registered `/api/page` route.
	patternPath := pattern
	if i := strings.Index(patternPath, "?"); i >= 0 {
		patternPath = patternPath[:i]
	}
	matchURL := url
	if i := strings.Index(matchURL, "?"); i >= 0 {
		matchURL = matchURL[:i]
	}

	handler, ok := a.apiRoutes[patternPath]
	if !ok {
		for p, h := range a.apiRoutes {
			if _, matched := matchPattern(p, matchURL); matched {
				handler = h
				ok = true
				break
			}
		}
	}
	if !ok {
		return nil
	}

	rec := &responseRecorder{headers: make(http.Header)}
	fakeReq, _ := http.NewRequest("GET", url, nil)

	// Forward cookies and auth headers
	if r != nil {
		for _, cookie := range r.Cookies() {
			fakeReq.AddCookie(cookie)
		}
		if auth := r.Header.Get("Authorization"); auth != "" {
			fakeReq.Header.Set("Authorization", auth)
		}
	}

	handler(rec, fakeReq)
	return json.RawMessage(rec.body)
}

type responseRecorder struct {
	headers http.Header
	body    []byte
	status  int
}

func (r *responseRecorder) Header() http.Header        { return r.headers }
func (r *responseRecorder) Write(b []byte) (int, error) { r.body = append(r.body, b...); return len(b), nil }
func (r *responseRecorder) WriteHeader(code int)        { r.status = code }

func (a *App) serveLoaderData(w http.ResponseWriter, r *http.Request) {
	urlPath := strings.TrimPrefix(r.URL.Path, "/_data")
	if urlPath == "" {
		urlPath = "/"
	}

	route := a.router.Match(urlPath)
	w.Header().Set("Content-Type", "application/json")

	// Layout data request: /_data/path?_layouts=1
	if r.URL.Query().Get("_layouts") == "1" && route != nil && route.LayoutLoaders != nil {
		layoutData := make(map[string]json.RawMessage)
		for path, sources := range route.LayoutLoaders {
			data := a.fetchLayoutLoaderData(sources, r)
			layoutData[pageURL(path)] = data
		}
		result, _ := json.Marshal(layoutData)
		w.Write(result)
		return
	}

	if route == nil || !route.HasLoader {
		w.Write([]byte("{}"))
		return
	}

	data := a.fetchLoaderData(route, r)
	if data == nil {
		w.Write([]byte("{}"))
	} else {
		w.Write(data)
	}
}
