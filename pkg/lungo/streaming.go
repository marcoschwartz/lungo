package lungo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// serveStreamingSSR renders a page using chunked transfer encoding.
// It sends the HTML shell immediately, then streams in data as it resolves.
func (a *App) serveStreamingSSR(w http.ResponseWriter, r *http.Request, route *Route) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		// Fallback to regular SSR
		a.serveSSR(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Transfer-Encoding", "chunked")

	// Phase 1: Send the shell immediately
	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	sb.WriteString("  <meta charset=\"UTF-8\">\n")
	sb.WriteString("  <meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n")
	sb.WriteString("  <title>Lungo App</title>\n")
	sb.WriteString("  <link rel=\"stylesheet\" href=\"/static/styles.css\">\n")
	sb.WriteString("</head>\n<body>\n")
	sb.WriteString(`<div id="root">`)
	sb.WriteString(`<div data-hydrate="app">`)
	sb.WriteString(`<div id="rg-placeholder-content" class="rg-placeholder">Loading...</div>`)
	sb.WriteString("</div></div>\n")

	w.Write([]byte(sb.String()))
	flusher.Flush()

	// Phase 2: Fetch data and stream it in
	var loaderData json.RawMessage
	if route.HasLoader && route.LoaderURL != "" {
		loaderData = a.fetchLoaderData(route, r)
	}

	// Phase 3: Send the resolved content + boot script
	sb.Reset()

	// Send the data
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
	sb.WriteString("</script>\n")

	w.Write([]byte(sb.String()))
	flusher.Flush()

	// Phase 4: Replace placeholder with real content
	sb.Reset()

	// Resolve the placeholder
	sb.WriteString(`<template data-chunk="content">`)
	sb.WriteString(a.renderLayoutShell(route))
	sb.WriteString("</template>\n")
	sb.WriteString(`<script>window.__RG_RESOLVE("content");</script>`)
	sb.WriteString("\n")

	w.Write([]byte(sb.String()))
	flusher.Flush()

	// Phase 5: Boot script
	sb.Reset()
	sb.WriteString(`<script src="/runtime/lungo.js"></script>`)
	sb.WriteString("\n")
	sb.WriteString("<script type=\"module\">\n")
	sb.WriteString("  const { h, render, RouterView } = window.Lungo;\n\n")

	pURL := pageURL(route.PagePath)
	sb.WriteString(fmt.Sprintf("  const initialPage = await import('%s');\n", pURL))

	for i, l := range route.Layouts {
		sb.WriteString(fmt.Sprintf("  const layout%d = await import('%s');\n", i, pageURL(l)))
	}

	sb.WriteString("\n  window.__LUNGO_INITIAL_PATH__ = window.location.pathname;\n")
	sb.WriteString("  window.Lungo.__initialPage = initialPage.default;\n")
	sb.WriteString("  window.Lungo.__initialData = window.__LUNGO_DATA__ || {};\n\n")

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

	w.Write([]byte(sb.String()))
	flusher.Flush()
}
