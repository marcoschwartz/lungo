package lungo

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Route represents a matched route with its page and layout chain.
type Route struct {
	Pattern     string
	PagePath    string            // relative path like "about/page.js"
	Layouts     []string          // relative paths like "layout.js"
	LoadingPath string            // relative path to loading.js if exists
	ErrorPath   string            // relative path to error.js if exists
	Segments    map[string]string // dynamic segments
	HasLoader   bool
	LoaderURL   string
}

// Router handles file-based routing.
type Router struct {
	appDir string // only used for disk-based routing
	appFS  fs.FS  // if set, read from embedded FS
	routes []routeEntry
}

type routeEntry struct {
	pattern     string
	segments    []string
	pagePath    string   // relative to app root (e.g., "about/page.js")
	layouts     []string // relative to app root
	loadingPath string
	errorPath   string
}

// NewRouter creates a router by scanning a directory on disk.
func NewRouter(appDir string) *Router {
	r := &Router{appDir: appDir}
	r.scanDisk()
	return r
}

// NewRouterFromFS creates a router by scanning an embedded fs.FS.
func NewRouterFromFS(appFS fs.FS) *Router {
	r := &Router{appFS: appFS}
	r.scanFS()
	return r
}

func (r *Router) Rescan() {
	r.routes = nil
	if r.appFS != nil {
		r.scanFS()
	} else {
		r.scanDisk()
	}
}

func (r *Router) scanDisk() {
	filepath.WalkDir(r.appDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || (d.Name() != "page.js" && d.Name() != "page.jsx") {
			return nil
		}
		rel, _ := filepath.Rel(r.appDir, path)
		rel = filepath.ToSlash(rel)
		r.addRoute(rel)
		return nil
	})
	r.sortRoutes()
}

func (r *Router) scanFS() {
	fs.WalkDir(r.appFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || (d.Name() != "page.js" && d.Name() != "page.jsx") {
			return nil
		}
		r.addRoute(path)
		return nil
	})
	r.sortRoutes()
}

func (r *Router) addRoute(relPath string) {
	// Normalize .jsx to .js for URL serving (transpiler handles conversion)
	if strings.HasSuffix(relPath, ".jsx") {
		relPath = strings.TrimSuffix(relPath, ".jsx") + ".js"
	}
	// relPath is like "about/page.js" or "blog/[slug]/page.js"
	dir := filepath.Dir(relPath)
	if dir == "." {
		dir = ""
	}

	parts := strings.Split(dir, "/")
	var urlParts []string
	var segments []string
	for _, p := range parts {
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "[") && strings.HasSuffix(p, "]") {
			param := p[1 : len(p)-1]
			segments = append(segments, param)
			urlParts = append(urlParts, ":"+param)
		} else {
			urlParts = append(urlParts, p)
		}
	}

	pattern := "/" + strings.Join(urlParts, "/")
	if pattern != "/" {
		pattern = strings.TrimSuffix(pattern, "/")
	}

	layouts := r.findLayouts(dir)
	loadingPath := r.findConventionFile(dir, "loading.js")
	errorPath := r.findConventionFile(dir, "error.js")

	r.routes = append(r.routes, routeEntry{
		pattern:     pattern,
		segments:    segments,
		pagePath:    relPath,
		layouts:     layouts,
		loadingPath: loadingPath,
		errorPath:   errorPath,
	})
}

// findConventionFile looks for a convention file (loading.js, error.js) in the
// route directory, walking up to root.
func (r *Router) findConventionFile(dir, filename string) string {
	current := dir
	for {
		path := filename
		if current != "" {
			path = current + "/" + filename
		}

		exists := false
		if r.appFS != nil {
			_, err := fs.Stat(r.appFS, path)
			exists = err == nil
		} else {
			_, err := os.Stat(filepath.Join(r.appDir, path))
			exists = err == nil
		}

		if exists {
			return path
		}

		// Also try .jsx variant
		jsxPath := strings.TrimSuffix(path, ".js") + ".jsx"
		if r.appFS != nil {
			_, err := fs.Stat(r.appFS, jsxPath)
			exists = err == nil
		} else {
			_, err := os.Stat(filepath.Join(r.appDir, jsxPath))
			exists = err == nil
		}
		if exists {
			// Return as .js — transpiler handles it
			return path
		}

		if current == "" {
			break
		}
		parent := filepath.Dir(current)
		if parent == "." {
			parent = ""
		}
		if parent == current {
			break
		}
		current = parent
	}
	return ""
}

func (r *Router) findLayouts(dir string) []string {
	var layouts []string
	current := dir
	for {
		layoutPath := current
		if layoutPath == "" {
			layoutPath = "layout.js"
		} else {
			layoutPath = layoutPath + "/layout.js"
		}

		exists := false
		if r.appFS != nil {
			_, err := fs.Stat(r.appFS, layoutPath)
			exists = err == nil
		} else {
			_, err := os.Stat(filepath.Join(r.appDir, layoutPath))
			exists = err == nil
		}

		if !exists {
			// Try .jsx
			jsxLayout := strings.TrimSuffix(layoutPath, ".js") + ".jsx"
			if r.appFS != nil {
				_, err := fs.Stat(r.appFS, jsxLayout)
				exists = err == nil
			} else {
				_, err := os.Stat(filepath.Join(r.appDir, jsxLayout))
				exists = err == nil
			}
		}

		if exists {
			layouts = append([]string{layoutPath}, layouts...)
		}

		if current == "" {
			break
		}
		parent := filepath.Dir(current)
		if parent == "." {
			parent = ""
		}
		if parent == current {
			break
		}
		current = parent
	}
	return layouts
}

func (r *Router) sortRoutes() {
	sort.Slice(r.routes, func(i, j int) bool {
		ci := strings.Count(r.routes[i].pattern, ":")
		cj := strings.Count(r.routes[j].pattern, ":")
		if ci != cj {
			return ci < cj
		}
		return r.routes[i].pattern < r.routes[j].pattern
	})
}

// Match finds a route matching the given URL path.
func (r *Router) Match(urlPath string) *Route {
	urlPath = strings.TrimSuffix(urlPath, "/")
	if urlPath == "" {
		urlPath = "/"
	}

	for _, entry := range r.routes {
		if segments, ok := matchPattern(entry.pattern, urlPath); ok {
			hasLoader, loaderURL := r.detectLoader(entry.pagePath)
			return &Route{
				Pattern:     entry.pattern,
				PagePath:    entry.pagePath,
				Layouts:     entry.layouts,
				LoadingPath: entry.loadingPath,
				ErrorPath:   entry.errorPath,
				Segments:    segments,
				HasLoader:   hasLoader,
				LoaderURL:   loaderURL,
			}
		}
	}
	return nil
}

func (r *Router) Routes() []string {
	var patterns []string
	for _, e := range r.routes {
		patterns = append(patterns, e.pattern)
	}
	return patterns
}

func matchPattern(pattern, urlPath string) (map[string]string, bool) {
	if pattern == "/" && urlPath == "/" {
		return map[string]string{}, true
	}

	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	urlParts := strings.Split(strings.Trim(urlPath, "/"), "/")

	if len(patternParts) != len(urlParts) {
		return nil, false
	}

	segments := make(map[string]string)
	for i, pp := range patternParts {
		if strings.HasPrefix(pp, ":") {
			segments[pp[1:]] = urlParts[i]
		} else if pp != urlParts[i] {
			return nil, false
		}
	}
	return segments, true
}

func (r *Router) detectLoader(pagePath string) (bool, string) {
	var content string
	if r.appFS != nil {
		data, err := fs.ReadFile(r.appFS, pagePath)
		if err != nil && strings.HasSuffix(pagePath, ".js") {
			data, err = fs.ReadFile(r.appFS, strings.TrimSuffix(pagePath, ".js")+".jsx")
		}
		if err != nil {
			return false, ""
		}
		content = string(data)
	} else {
		data, err := os.ReadFile(filepath.Join(r.appDir, pagePath))
		if err != nil && strings.HasSuffix(pagePath, ".js") {
			data, err = os.ReadFile(filepath.Join(r.appDir, strings.TrimSuffix(pagePath, ".js")+".jsx"))
		}
		if err != nil {
			return false, ""
		}
		content = string(data)
	}

	if !strings.Contains(content, "export") || !strings.Contains(content, "loader") {
		return false, ""
	}

	idx := strings.Index(content, "loader")
	if idx < 0 {
		return false, ""
	}
	rest := content[idx:]
	urlIdx := strings.Index(rest, `url:`)
	if urlIdx < 0 {
		return true, ""
	}
	rest = rest[urlIdx+4:]
	q1 := strings.IndexAny(rest, `"'`)
	if q1 < 0 {
		return true, ""
	}
	rest = rest[q1+1:]
	q2 := strings.IndexAny(rest, `"'`)
	if q2 < 0 {
		return true, ""
	}
	return true, rest[:q2]
}
