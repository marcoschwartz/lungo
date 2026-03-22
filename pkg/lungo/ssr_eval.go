package lungo

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/marcoschwartz/espresso"
)

// ─── SSR Page Cache ─────────────────────────────────────────────
// Caches transpiled + parsed page data so we don't redo it every request.

var (
	pageCacheMap   = make(map[string]*ssrPageCache)
	pageCacheMu sync.RWMutex
)

func (a *App) getPageCache(pagePath string) (*ssrPageCache, error) {
	if !a.opts.Dev {
		pageCacheMu.RLock()
		cached, ok := pageCacheMap[pagePath]
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

	hookCount := strings.Count(rawSource, "useState")
	isInteractive := hookCount > 3

	source, jsxErrors := TranspileJSXWithErrors(rawSource)
	if len(jsxErrors) > 0 {
		log.Printf("[Lungo] JSX errors in %s:", pagePath)
		for _, e := range jsxErrors {
			log.Printf("  - %s", e)
		}
		if a.opts.Dev {
			return nil, fmt.Errorf("JSX error in %s: %s", pagePath, jsxErrors[0])
		}
	}

	funcBody, funcParams, err := espresso.ExtractDefaultExport(source)
	if err != nil {
		return nil, fmt.Errorf("extract default export: %w", err)
	}

	localFuncs := espresso.ExtractFunctions(source)
	tokens := espresso.Tokenize(funcBody)

	// Extract top-level const/let/var declarations (e.g. navLinks, colorMap)
	topLevelVars := make(map[string]*espresso.Value)
	espresso.ExtractTopLevelVars(source, topLevelVars)

	entry := &ssrPageCache{
		funcBody:     funcBody,
		funcParams:   funcParams,
		localFuncs:   localFuncs,
		tokens:       tokens,
		interactive:  isInteractive,
		topLevelVars: topLevelVars,
	}

	if !a.opts.Dev {
		pageCacheMu.Lock()
		pageCacheMap[pagePath] = entry
		pageCacheMu.Unlock()
	}

	return entry, nil
}

// evaluatePageSSR renders a page's HTML on the server using espresso.
func (a *App) evaluatePageSSR(pagePath string, loaderData json.RawMessage, params map[string]string) (string, bool, error) {
	cached, err := a.getPageCache(pagePath)
	if err != nil {
		return "", false, err
	}

	scope := buildSSRScope(cached.localFuncs)

	// Inject top-level vars (const navLinks = [...], etc.)
	for k, v := range cached.topLevelVars {
		scope[k] = v
	}

	if loaderData != nil {
		scope["data"] = espresso.JsonToValue(loaderData)
	} else {
		scope["data"] = espresso.Null
	}

	paramsObj := make(map[string]*espresso.Value, len(params))
	for k, v := range params {
		paramsObj[k] = espresso.NewStr(v)
	}
	scope["params"] = espresso.NewObj(paramsObj)

	// Inject process.env with LUNGO_PUBLIC_* vars (like Next.js process.env)
	envObj := make(map[string]*espresso.Value, len(a.publicEnv))
	for k, v := range a.publicEnv {
		envObj[k] = espresso.NewStr(v)
	}
	scope["process"] = espresso.NewObj(map[string]*espresso.Value{
		"env": espresso.NewObj(envObj),
	})

	if strings.Contains(cached.funcParams, "{") {
		// Destructured — data/params already in scope
	} else if cached.funcParams != "" {
		propsObj := make(map[string]*espresso.Value)
		propsObj["data"] = scope["data"]
		propsObj["params"] = scope["params"]
		scope[cached.funcParams] = espresso.NewObj(propsObj)
	}

	// Evaluate using espresso
	tokens := make([]espresso.Tok, len(cached.tokens))
	copy(tokens, cached.tokens)
	ev := espresso.NewEval(tokens, scope)
	result := ev.EvalStatements()

	if result == nil || !result.IsCustom() {
		return "", false, fmt.Errorf("page did not return a vnode")
	}

	node, ok := result.Custom.(*ssrNode)
	if !ok || node == nil {
		return "", false, fmt.Errorf("page did not return a vnode")
	}

	return RenderSSRHTML(node), cached.interactive, nil
}

// wrapInLayoutsWithData evaluates each layout, fetching loader data if defined.
func (a *App) wrapInLayoutsWithData(pageHTML string, route *Route, isDark bool, r *http.Request) (string, map[string]json.RawMessage) {
	layoutDataMap := make(map[string]json.RawMessage)
	urlPath := route.Pattern
	if r != nil {
		urlPath = r.URL.Path
	}

	for _, layoutPath := range route.Layouts {
		var layoutData json.RawMessage
		if route.LayoutLoaders != nil {
			if sources, ok := route.LayoutLoaders[layoutPath]; ok && len(sources) > 0 {
				layoutData = a.fetchLayoutLoaderData(sources, r)
				layoutDataMap[layoutPath] = layoutData
			}
		}

		wrapped, err := a.evaluateLayoutWithData(layoutPath, pageHTML, isDark, urlPath, layoutData)
		if err != nil {
			continue
		}
		pageHTML = wrapped
	}
	return pageHTML, layoutDataMap
}

// wrapInLayouts is the backwards-compatible version without loader data.
func (a *App) wrapInLayouts(pageHTML string, layouts []string, isDark bool, urlPath ...string) string {
	p := "/"
	if len(urlPath) > 0 {
		p = urlPath[0]
	}
	for _, layoutPath := range layouts {
		wrapped, err := a.evaluateLayoutWithData(layoutPath, pageHTML, isDark, p, nil)
		if err != nil {
			continue
		}
		pageHTML = wrapped
	}
	return pageHTML
}

// fetchLayoutLoaderData fetches data for a layout's loader sources.
func (a *App) fetchLayoutLoaderData(sources []LoaderSource, r *http.Request) json.RawMessage {
	if len(sources) == 1 {
		return a.callHandler(sources[0].URL, sources[0].URL, r)
	}
	results := make(map[string]json.RawMessage, len(sources))
	type result struct {
		key  string
		data json.RawMessage
	}
	ch := make(chan result, len(sources))
	for _, s := range sources {
		go func(src LoaderSource) {
			data := a.callHandler(src.URL, src.URL, r)
			ch <- result{key: src.Key, data: data}
		}(s)
	}
	for range sources {
		res := <-ch
		if res.key != "" {
			results[res.key] = res.data
		}
	}
	merged, _ := json.Marshal(results)
	return merged
}

// evaluateLayoutWithData evaluates a layout component with optional loader data.
func (a *App) evaluateLayoutWithData(layoutPath string, childrenHTML string, isDark bool, urlPath string, loaderData json.RawMessage) (string, error) {
	cached, err := a.getPageCache(layoutPath)
	if err != nil {
		return "", err
	}

	scope := buildSSRScope(cached.localFuncs)
	stubHooksInScope(scope, urlPath)

	// Inject top-level vars (const navLinks = [...], etc.)
	for k, v := range cached.topLevelVars {
		scope[k] = v
	}

	// Override getInitialTheme for dark mode
	if isDark {
		scope["getInitialTheme"] = espresso.NewNativeFunc(func(args []*espresso.Value) *espresso.Value {
			return espresso.NewStr("dark")
		})
	}

	// children placeholder
	childrenNode := &ssrNode{Tag: "lungo-children", IsText: false}
	scope["children"] = espresso.NewCustom(childrenNode)

	// Inject layout loader data
	if loaderData != nil {
		scope["data"] = espresso.JsonToValue(loaderData)
	} else {
		scope["data"] = espresso.Null
	}

	// Inject process.env with LUNGO_PUBLIC_* vars
	envObj := make(map[string]*espresso.Value, len(a.publicEnv))
	for k, v := range a.publicEnv {
		envObj[k] = espresso.NewStr(v)
	}
	scope["process"] = espresso.NewObj(map[string]*espresso.Value{
		"env": espresso.NewObj(envObj),
	})

	// Handle destructured params
	if strings.Contains(cached.funcParams, "{") {
		// children and data already in scope
	} else if cached.funcParams != "" {
		propsObj := make(map[string]*espresso.Value)
		propsObj["children"] = scope["children"]
		propsObj["data"] = scope["data"]
		scope[cached.funcParams] = espresso.NewObj(propsObj)
	}

	tokens := make([]espresso.Tok, len(cached.tokens))
	copy(tokens, cached.tokens)
	ev := espresso.NewEval(tokens, scope)
	result := ev.EvalStatements()

	if result == nil || !result.IsCustom() {
		return "", fmt.Errorf("layout did not return a vnode")
	}

	node, ok := result.Custom.(*ssrNode)
	if !ok || node == nil {
		return "", fmt.Errorf("layout did not return a vnode")
	}

	html := RenderSSRHTML(node)
	html = strings.Replace(html, "<lungo-children></lungo-children>", childrenHTML, 1)
	return html, nil
}
