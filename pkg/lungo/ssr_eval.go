package lungo

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// ─── SSR Page Cache ─────────────────────────────────────────────
// Caches transpiled + parsed page data so we don't redo it every request.

type ssrPageCache struct {
	funcBody    string
	funcParams  string
	localFuncs  map[string]*jsValue
	tokens      []tok          // pre-tokenized function body
	compiled    *compiledPage  // compiled closures (nil if compilation failed)
	interactive bool           // true if page has many hooks — use render() not hydrate()
}

var (
	pageCache   = make(map[string]*ssrPageCache)
	pageCacheMu sync.RWMutex
)

func (a *App) getPageCache(pagePath string) (*ssrPageCache, error) {
	// In dev mode, skip cache (files change)
	if !a.opts.Dev {
		pageCacheMu.RLock()
		cached, ok := pageCache[pagePath]
		pageCacheMu.RUnlock()
		if ok {
			return cached, nil
		}
	}

	isJSX := false
	if strings.HasSuffix(pagePath, ".jsx") {
		isJSX = true
	} else if strings.HasSuffix(pagePath, ".js") {
		jsxPath := strings.TrimSuffix(pagePath, ".js") + ".jsx"
		if a.hasAppFile(jsxPath) {
			isJSX = true
		}
	}
	if !isJSX {
		return nil, fmt.Errorf("only .jsx pages support SSR evaluation")
	}

	data, err := a.readAppFile(pagePath)
	if err != nil {
		return nil, fmt.Errorf("read page: %w", err)
	}
	rawSource := string(data)

	// Detect interactive pages — too many hooks means hydration would mismatch,
	// so we'll SSR the content but use render() instead of hydrate() on the client.
	hookCount := strings.Count(rawSource, "useState")
	isInteractive := hookCount > 3

	source := TranspileJSX(rawSource)

	funcBody, funcParams, err := extractDefaultExport(source)
	if err != nil {
		return nil, fmt.Errorf("extract default export: %w", err)
	}

	localFuncs := extractFunctions(source)
	tokens := jsTokenize(funcBody)

	// Try to compile to closures for faster execution
	compiled := compilePageTokens(tokens, localFuncs)

	entry := &ssrPageCache{
		funcBody:    funcBody,
		funcParams:  funcParams,
		localFuncs:  localFuncs,
		tokens:      tokens,
		compiled:    compiled,
		interactive: isInteractive,
	}

	if !a.opts.Dev {
		pageCacheMu.Lock()
		pageCache[pagePath] = entry
		pageCacheMu.Unlock()
	}

	return entry, nil
}

// evaluatePageSSR attempts to render a page's HTML on the server.
// It transpiles the JSX, evaluates the default export with the given data,
// and returns the rendered HTML string.
// Returns empty string and error if evaluation fails (caller should fall back).
// evaluatePageSSR returns (html, interactive, error).
// interactive=true means the page has many hooks and should use render() not hydrate().
func (a *App) evaluatePageSSR(pagePath string, loaderData json.RawMessage, params map[string]string) (string, bool, error) {
	cached, err := a.getPageCache(pagePath)
	if err != nil {
		return "", false, err
	}

	// Build scope (fresh per request — only data/params change)
	scope := make(map[string]*jsValue, len(cached.localFuncs)+10)

	if loaderData != nil {
		scope["data"] = jsonToJSValue(loaderData)
	} else {
		scope["data"] = jvNull
	}

	paramsObj := make(map[string]*jsValue, len(params))
	for k, v := range params {
		paramsObj[k] = jvStr(v)
	}
	scope["params"] = jvObj(paramsObj)

	stubHooks(&jsEval{scope: scope})

	for name, fn := range cached.localFuncs {
		scope[name] = fn
	}

	if strings.Contains(cached.funcParams, "{") {
		// Destructured — data/params already in scope
	} else if cached.funcParams != "" {
		propsObj := make(map[string]*jsValue)
		propsObj["data"] = scope["data"]
		propsObj["params"] = scope["params"]
		scope[cached.funcParams] = jvObj(propsObj)
	}

	interactive := cached.interactive

	// Fast path: compiled closures → direct HTML (no vnode intermediary)
	if cached.compiled != nil {
		html := cached.compiled.renderHTML(scope)
		if html != "" {
			return html, interactive, nil
		}
	}

	// Fallback: interpreted token evaluation
	tokens := make([]tok, len(cached.tokens))
	copy(tokens, cached.tokens)
	ev := &jsEval{tokens: tokens, pos: 0, scope: scope}
	result := ev.evalStatements()

	if result.typ != jsTypeVNode || result.vnode == nil {
		return "", false, fmt.Errorf("page did not return a vnode")
	}

	return RenderSSRHTMLPooled(result.vnode), interactive, nil
}

// wrapInLayouts evaluates each layout and wraps the page HTML inside it.
// Layouts use hooks (useState, useEffect, useRouter) which we stub out.
func (a *App) wrapInLayouts(pageHTML string, layouts []string, isDark bool, urlPath ...string) string {
	for _, layoutPath := range layouts {
		wrapped, err := a.evaluateLayout(layoutPath, pageHTML, isDark, urlPath...)
		if err != nil {
			// Layout eval failed — just return page content as-is
			continue
		}
		pageHTML = wrapped
	}
	return pageHTML
}

// evaluateLayout evaluates a layout component, injecting pageHTML as {children}.
func (a *App) evaluateLayout(layoutPath string, childrenHTML string, isDark bool, urlPath ...string) (string, error) {
	cached, err := a.getPageCache(layoutPath)
	if err != nil {
		return "", err
	}

	scope := make(map[string]*jsValue, len(cached.localFuncs)+10)
	for name, fn := range cached.localFuncs {
		scope[name] = fn
	}

	// Override getInitialTheme to return the cookie value for correct SSR
	if isDark {
		themeID := registerArrow(&arrowFunc{
			tokens: append(jsTokenize(`"dark"`), tok{t: tokEOF}),
			scope:  make(map[string]*jsValue),
		})
		scope["getInitialTheme"] = &jsValue{typ: jsTypeFunc, str: "__arrow", num: float64(themeID)}
	}

	// Stub out hooks — they return safe defaults
	// children is a special vnode containing the pre-rendered page HTML
	childrenNode := &ssrNode{
		Tag:    "lungo-children",
		IsText: false,
	}
	scope["children"] = jvNode(childrenNode)

	// Handle destructured params: function Layout({ children })
	if strings.Contains(cached.funcParams, "{") {
		// children already in scope
	} else if cached.funcParams != "" {
		propsObj := make(map[string]*jsValue)
		propsObj["children"] = scope["children"]
		scope[cached.funcParams] = jvObj(propsObj)
	}

	// Use pre-tokenized body
	tokens := make([]tok, len(cached.tokens))
	copy(tokens, cached.tokens)
	ev := &jsEval{tokens: tokens, pos: 0, scope: scope}
	if len(urlPath) > 0 {
		stubHooksWithPath(ev, urlPath[0])
	} else {
		stubHooks(ev)
	}

	result := ev.evalStatements()
	if result.typ != jsTypeVNode || result.vnode == nil {
		return "", fmt.Errorf("layout did not return a vnode")
	}

	// Render to HTML, replacing <lungo-children> placeholder with actual page content
	html := RenderSSRHTML(result.vnode)
	html = strings.Replace(html, "<lungo-children></lungo-children>", childrenHTML, 1)
	return html, nil
}

// stubHooks adds stub implementations for client-side hooks to the evaluator scope.
func stubHooks(ev *jsEval) {
	stubHooksWithPath(ev, "/")
}

// stubHooksWithPath adds hook stubs with the given URL path for useRouter.
func stubHooksWithPath(ev *jsEval, urlPath string) {
	ev.scope["useState"] = &jsValue{typ: jsTypeFunc, str: "__hook_useState"}
	ev.scope["useEffect"] = &jsValue{typ: jsTypeFunc, str: "__hook_useEffect"}
	ev.scope["useRouter"] = &jsValue{typ: jsTypeFunc, str: "__hook_useRouter"}
	ev.scope["useRef"] = &jsValue{typ: jsTypeFunc, str: "__hook_useRef"}
	ev.scope["useMemo"] = &jsValue{typ: jsTypeFunc, str: "__hook_useMemo"}
	// Store the URL path so useRouter() returns the correct pathname
	ev.scope["__ssr_pathname"] = jvStr(urlPath)
}

// extractDefaultExport finds the default export function and returns its body and parameter string.
func extractDefaultExport(source string) (body string, params string, err error) {
	// Find "export default function" — but skip occurrences inside string literals
	idx := indexOutsideStrings(source, "export default function")
	if idx < 0 {
		return "", "", fmt.Errorf("no default export function found")
	}

	rest := source[idx+len("export default function"):]

	// Skip function name (optional)
	rest = strings.TrimSpace(rest)
	if len(rest) > 0 && rest[0] != '(' {
		nameEnd := strings.IndexAny(rest, "( ")
		if nameEnd < 0 {
			return "", "", fmt.Errorf("malformed function declaration")
		}
		rest = strings.TrimSpace(rest[nameEnd:])
	}

	// Extract parameters
	if len(rest) == 0 || rest[0] != '(' {
		return "", "", fmt.Errorf("expected ( after function name")
	}
	parenEnd := findMatchingParen(rest, 0)
	if parenEnd < 0 {
		return "", "", fmt.Errorf("unmatched ( in function params")
	}
	params = strings.TrimSpace(rest[1:parenEnd])
	rest = strings.TrimSpace(rest[parenEnd+1:])

	// Extract function body
	if len(rest) == 0 || rest[0] != '{' {
		return "", "", fmt.Errorf("expected { after function params")
	}
	braceEnd := findMatchingBrace(rest, 0)
	if braceEnd < 0 {
		return "", "", fmt.Errorf("unmatched { in function body")
	}
	body = rest[1:braceEnd] // strip outer { }
	return body, params, nil
}

// extractFunctions finds all non-exported function definitions and returns them as jsValue funcs.
func extractFunctions(source string) map[string]*jsValue {
	funcs := make(map[string]*jsValue)

	i := 0
	for i < len(source) {
		idx := strings.Index(source[i:], "function ")
		if idx < 0 {
			break
		}
		absIdx := i + idx

		// Check this isn't the default export
		prefix := source[maxInt(0, absIdx-30):absIdx]
		if strings.Contains(prefix, "export default") {
			i = absIdx + 9
			continue
		}

		rest := source[absIdx+9:]
		rest = strings.TrimSpace(rest)

		// Get function name
		nameEnd := strings.IndexAny(rest, "( \t\n")
		if nameEnd < 0 || nameEnd == 0 {
			i = absIdx + 9
			continue
		}
		name := strings.TrimSpace(rest[:nameEnd])
		if name == "" {
			i = absIdx + 9
			continue
		}

		rest = strings.TrimSpace(rest[nameEnd:])

		if len(rest) == 0 || rest[0] != '(' {
			i = absIdx + 9
			continue
		}
		parenEnd := findMatchingParen(rest, 0)
		if parenEnd < 0 {
			i = absIdx + 9
			continue
		}
		paramStr := strings.TrimSpace(rest[1:parenEnd])
		rest = strings.TrimSpace(rest[parenEnd+1:])

		if len(rest) == 0 || rest[0] != '{' {
			i = absIdx + 9
			continue
		}
		braceEnd := findMatchingBrace(rest, 0)
		if braceEnd < 0 {
			i = absIdx + 9
			continue
		}
		bodyStr := rest[1:braceEnd]

		var fnParams []string
		if paramStr != "" {
			fnParams = []string{paramStr}
		}

		funcs[name] = &jsValue{
			typ:      jsTypeFunc,
			fnParams: fnParams,
			fnBody:   bodyStr,
		}

		i = absIdx + 9 + len(rest[:braceEnd+1])
	}

	return funcs
}

func findMatchingParen(s string, start int) int {
	depth := 0
	inStr := byte(0)
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inStr != 0 {
			if ch == inStr && (i == 0 || s[i-1] != '\\') {
				inStr = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' || ch == '`' {
			inStr = ch
			continue
		}
		if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// indexOutsideStrings finds the first occurrence of needle in s that is not
// inside a string literal (single, double, or backtick quotes).
func indexOutsideStrings(s, needle string) int {
	inStr := byte(0)
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inStr != 0 {
			if ch == inStr && (i == 0 || s[i-1] != '\\') {
				inStr = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' || ch == '`' {
			inStr = ch
			continue
		}
		if i+len(needle) <= len(s) && s[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func findMatchingBrace(s string, start int) int {
	depth := 0
	inStr := byte(0)
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inStr != 0 {
			if ch == inStr && (i == 0 || s[i-1] != '\\') {
				inStr = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' || ch == '`' {
			inStr = ch
			continue
		}
		if ch == '{' {
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
