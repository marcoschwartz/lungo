// Package reactgo provides a Next.js-like framework powered by Go.
package lungo

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

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
}

// App is the main Lungo application instance.
type App struct {
	opts        Options
	router      *Router
	handler     http.Handler
	apiRoutes   map[string]http.HandlerFunc
	middlewares []Middleware
	hmr         *HMR
}

// New creates a new Lungo application.
func New(opts Options) *App {
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

	app := &App{
		opts:      opts,
		apiRoutes: make(map[string]http.HandlerFunc),
	}

	if opts.AppFS != nil {
		app.router = NewRouterFromFS(opts.AppFS)
	} else {
		app.router = NewRouter(opts.AppDir)
	}

	if opts.Dev {
		app.hmr = NewHMR(opts.AppDir)
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
				w.Header().Set("Cache-Control", "public, max-age=31536000")
			}
			w.Write(runtimeJS)
			return
		}

		// 2. HMR WebSocket (dev only)
		if path == "/__hmr" && a.hmr != nil {
			a.hmr.ServeWS(w, r)
			return
		}

		// 3. API routes
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
				w.Header().Set("Cache-Control", "public, max-age=31536000")
			}
			w.Write(data)
			return
		}
	} else {
		filePath := filepath.Join(a.opts.StaticDir, name)
		if _, err := os.Stat(filePath); err == nil {
			http.ServeFile(w, r, filePath)
			return
		}
	}
	http.NotFound(w, r)
}

func (a *App) serveAppFile(w http.ResponseWriter, r *http.Request, name string) {
	needsTranspile := false

	// Try exact .js file on disk first
	var data []byte
	var err error

	if a.opts.AppFS != nil {
		data, err = fs.ReadFile(a.opts.AppFS, name)
	} else {
		data, err = os.ReadFile(filepath.Join(a.opts.AppDir, name))
	}

	if err != nil && strings.HasSuffix(name, ".js") {
		// .js not found — try .jsx
		jsxName := strings.TrimSuffix(name, ".js") + ".jsx"
		if a.opts.AppFS != nil {
			data, err = fs.ReadFile(a.opts.AppFS, jsxName)
		} else {
			data, err = os.ReadFile(filepath.Join(a.opts.AppDir, jsxName))
		}
		if err != nil {
			http.NotFound(w, r)
			return
		}
		needsTranspile = true
	} else if err != nil {
		http.NotFound(w, r)
		return
	} else if strings.HasSuffix(name, ".jsx") {
		needsTranspile = true
	}

	if needsTranspile {
		data = []byte(TranspileJSX(string(data)))
	}

	w.Header().Set("Content-Type", "application/javascript")
	if a.opts.Dev {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=31536000")
	}
	w.Write(data)
}

func (a *App) serveSSRWithRecovery(w http.ResponseWriter, r *http.Request) {
	route := a.router.Match(r.URL.Path)
	if route == nil {
		a.serveNotFound(w, r)
		return
	}
	a.serveSSR(w, r)
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
