package lungo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

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

	html := a.renderPage(route, loaderData)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// pageURL converts a relative path like "about/page.js" to "/app/about/page.js"
func pageURL(relPath string) string {
	return "/app/" + relPath
}

func (a *App) renderPage(route *Route, loaderData json.RawMessage) string {
	var sb strings.Builder

	// Extract metadata from page
	meta := a.extractMetadata(route.PagePath)

	// Inline theme detection script to prevent flash of wrong theme
	sb.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n")
	sb.WriteString("<script>(function(){var t=localStorage.getItem('theme');if(t==='dark'||(!t&&matchMedia('(prefers-color-scheme:dark)').matches))document.documentElement.classList.add('dark')})()</script>\n")
	sb.WriteString("<head>\n")
	sb.WriteString("  <meta charset=\"UTF-8\">\n")
	sb.WriteString("  <meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n")
	if meta != nil && meta.Title != "" {
		sb.WriteString(fmt.Sprintf("  <title>%s</title>\n", meta.Title))
	} else {
		sb.WriteString("  <title>Lungo App</title>\n")
	}
	if meta != nil && meta.Description != "" {
		sb.WriteString(fmt.Sprintf("  <meta name=\"description\" content=\"%s\">\n", meta.Description))
	}
	sb.WriteString("  <link rel=\"stylesheet\" href=\"/static/styles.css\">\n")
	sb.WriteString("</head>\n<body>\n")

	sb.WriteString(`<div id="root">`)
	sb.WriteString(a.renderLayoutShell(route))
	sb.WriteString("</div>\n\n")

	sb.WriteString("<script>\n")
	if a.opts.Dev {
		sb.WriteString("  window.__LUNGO_DEV__ = true;\n")
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

	sb.WriteString("</script>\n\n")

	sb.WriteString(`<script src="/runtime/lungo.js"></script>`)
	sb.WriteString("\n")

	sb.WriteString("<script type=\"module\">\n")
	sb.WriteString("  const { h, render, RouterView } = window.Lungo;\n\n")

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

	// Collect layout components
	sb.WriteString("  const layouts = [")
	for i := range route.Layouts {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("layout%d.default", i))
	}
	sb.WriteString("];\n\n")

	sb.WriteString("  render(h`<${RouterView} layouts=${layouts} />`, document.getElementById('root'));\n")
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

func (a *App) fetchLoaderData(route *Route, r *http.Request) json.RawMessage {
	url := route.LoaderURL
	for k, v := range route.Segments {
		url = strings.ReplaceAll(url, ":"+k, v)
		url = strings.ReplaceAll(url, "{"+k+"}", v)
	}

	// If there are segments and the URL didn't contain placeholders, append as query params
	if len(route.Segments) > 0 && url == route.LoaderURL {
		sep := "?"
		if strings.Contains(url, "?") {
			sep = "&"
		}
		for k, v := range route.Segments {
			url += sep + k + "=" + v
			sep = "&"
		}
	}

	handler, ok := a.apiRoutes[route.LoaderURL]
	if !ok {
		for pattern, h := range a.apiRoutes {
			if _, matched := matchPattern(pattern, url); matched {
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
	if route == nil || !route.HasLoader {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{}"))
		return
	}

	data := a.fetchLoaderData(route, r)
	w.Header().Set("Content-Type", "application/json")
	if data == nil {
		w.Write([]byte("{}"))
	} else {
		w.Write(data)
	}
}
