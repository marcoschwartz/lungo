package lungo

import (
	"fmt"
	"net/http"
	"strings"
)

// renderDevErrorOverlay generates a Next.js-style red error overlay for dev mode.
func renderDevErrorOverlay(title string, errors []string, filePath string) string {
	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	sb.WriteString("  <meta charset=\"UTF-8\">\n")
	sb.WriteString("  <meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n")
	sb.WriteString("  <title>Build Error</title>\n")
	sb.WriteString("  <style>\n")
	sb.WriteString("    * { margin: 0; padding: 0; box-sizing: border-box; }\n")
	sb.WriteString("    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #1a1a2e; color: #eee; }\n")
	sb.WriteString("    .overlay { position: fixed; inset: 0; display: flex; align-items: center; justify-content: center; padding: 24px; }\n")
	sb.WriteString("    .card { background: #16213e; border: 1px solid #e94560; border-radius: 12px; padding: 32px; max-width: 640px; width: 100%; box-shadow: 0 25px 50px rgba(233,69,96,0.15); }\n")
	sb.WriteString("    .badge { display: inline-block; background: #e94560; color: white; font-size: 11px; font-weight: 700; padding: 3px 10px; border-radius: 4px; letter-spacing: 0.5px; margin-bottom: 16px; }\n")
	sb.WriteString("    h1 { font-size: 20px; font-weight: 700; margin-bottom: 12px; color: #fff; }\n")
	sb.WriteString("    .file { font-size: 13px; color: #888; margin-bottom: 16px; font-family: monospace; }\n")
	sb.WriteString("    .error { background: #0f3460; border: 1px solid #e94560; border-radius: 8px; padding: 16px; margin-bottom: 12px; }\n")
	sb.WriteString("    .error-text { font-family: 'SF Mono', Menlo, monospace; font-size: 14px; color: #ff6b6b; line-height: 1.6; }\n")
	sb.WriteString("    .hint { font-size: 13px; color: #666; margin-top: 16px; }\n")
	sb.WriteString("  </style>\n")
	sb.WriteString("</head>\n<body>\n")
	sb.WriteString("<div class=\"overlay\"><div class=\"card\">\n")
	sb.WriteString("  <span class=\"badge\">BUILD ERROR</span>\n")
	sb.WriteString(fmt.Sprintf("  <h1>%s</h1>\n", title))
	if filePath != "" {
		sb.WriteString(fmt.Sprintf("  <div class=\"file\">%s</div>\n", filePath))
	}
	for _, e := range errors {
		sb.WriteString(fmt.Sprintf("  <div class=\"error\"><div class=\"error-text\">%s</div></div>\n", e))
	}
	sb.WriteString("  <div class=\"hint\">Fix the error and the page will auto-refresh.</div>\n")
	sb.WriteString("</div></div>\n")
	sb.WriteString("</body>\n</html>")
	return sb.String()
}

// serveNotFound renders a custom not-found.js page if it exists, otherwise a default 404.
func (a *App) serveNotFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)

	hasCustom := a.hasAppFile("not-found.js")
	if !hasCustom {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(a.renderErrorPage("404", "Page Not Found", "The page you're looking for doesn't exist.", nil)))
		return
	}

	// Render with custom not-found.js
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(a.renderCustomErrorPage("not-found.js", nil)))
}

// serveError renders a custom error.js page if it exists, otherwise a default error page.
func (a *App) serveError(w http.ResponseWriter, r *http.Request, errMsg string) {
	w.WriteHeader(http.StatusInternalServerError)

	hasCustom := a.hasAppFile("error.js")
	if !hasCustom {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		detail := ""
		if a.opts.Dev {
			detail = errMsg
		}
		w.Write([]byte(a.renderErrorPage("500", "Something went wrong", "An unexpected error occurred.", &detail)))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(a.renderCustomErrorPage("error.js", &errMsg)))
}

// renderErrorPage generates a default error page with Tailwind styling.
func (a *App) renderErrorPage(code, title, message string, detail *string) string {
	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	sb.WriteString("  <meta charset=\"UTF-8\">\n")
	sb.WriteString("  <meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n")
	sb.WriteString(fmt.Sprintf("  <title>%s — %s</title>\n", code, title))
	sb.WriteString("  <link rel=\"stylesheet\" href=\"/static/styles.css\">\n")
	sb.WriteString("</head>\n<body>\n")
	sb.WriteString(`<div class="min-h-screen flex items-center justify-center bg-gray-50">`)
	sb.WriteString(`<div class="text-center px-6">`)
	sb.WriteString(fmt.Sprintf(`<h1 class="text-8xl font-extrabold text-gray-200 mb-4">%s</h1>`, code))
	sb.WriteString(fmt.Sprintf(`<h2 class="text-2xl font-bold text-gray-900 mb-2">%s</h2>`, title))
	sb.WriteString(fmt.Sprintf(`<p class="text-gray-500 mb-8">%s</p>`, message))
	if detail != nil && *detail != "" {
		sb.WriteString(fmt.Sprintf(`<pre class="text-left bg-red-50 border border-red-200 rounded-xl p-4 text-sm text-red-700 mb-8 max-w-xl mx-auto overflow-x-auto">%s</pre>`, *detail))
	}
	sb.WriteString(`<a href="/" class="inline-block px-6 py-3 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors">Go Home</a>`)
	sb.WriteString("</div></div>\n")
	sb.WriteString("</body>\n</html>")
	return sb.String()
}

// renderCustomErrorPage renders a custom error.js or not-found.js with the framework shell.
func (a *App) renderCustomErrorPage(pageName string, errMsg *string) string {
	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	sb.WriteString("  <meta charset=\"UTF-8\">\n")
	sb.WriteString("  <meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n")
	sb.WriteString("  <title>Error — Lungo</title>\n")
	sb.WriteString("  <link rel=\"stylesheet\" href=\"/static/styles.css\">\n")
	sb.WriteString("</head>\n<body>\n")
	sb.WriteString(`<div id="root"></div>`)
	sb.WriteString("\n<script>\n")
	if a.opts.Dev {
		sb.WriteString("  window.__LUNGO_DEV__ = true;\n")
	}
	if errMsg != nil {
		sb.WriteString(fmt.Sprintf("  window.__LUNGO_ERROR__ = %q;\n", *errMsg))
	}
	sb.WriteString("</script>\n")
	sb.WriteString(`<script src="/runtime/lungo.js"></script>`)
	sb.WriteString("\n<script type=\"module\">\n")
	sb.WriteString("  const { h, render } = window.Lungo;\n")
	sb.WriteString(fmt.Sprintf("  const mod = await import('/app/%s');\n", pageName))
	sb.WriteString("  const Page = mod.default;\n")
	sb.WriteString("  const error = window.__LUNGO_ERROR__ || null;\n")

	// Wrap in root layout if it exists
	layouts := a.router.findLayouts("")
	if len(layouts) > 0 {
		for i, l := range layouts {
			sb.WriteString(fmt.Sprintf("  const layout%d = await import('/app/%s');\n", i, l))
		}
		sb.WriteString("  function App() {\n")
		for i := range layouts {
			sb.WriteString(fmt.Sprintf("    const L%d = layout%d.default;\n", i, i))
		}
		sb.WriteString("    return h`")
		for i := range layouts {
			sb.WriteString(fmt.Sprintf("<${L%d}>", i))
		}
		sb.WriteString("<${Page} error=${error} />")
		for i := len(layouts) - 1; i >= 0; i-- {
			sb.WriteString(fmt.Sprintf("</${L%d}>", i))
		}
		sb.WriteString("`;\n  }\n")
		sb.WriteString("  render(h`<${App} />`, document.getElementById('root'));\n")
	} else {
		sb.WriteString("  render(h`<${Page} error=${error} />`, document.getElementById('root'));\n")
	}
	sb.WriteString("</script>\n</body>\n</html>")
	return sb.String()
}
