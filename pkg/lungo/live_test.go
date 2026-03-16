package lungo

import (
	"os"
	"strings"
	"testing"
)

func TestLivePageSSR_Interpreted(t *testing.T) {
	data, err := os.ReadFile("../../_example/app/live/page.jsx")
	if err != nil { t.Skip() }
	source := TranspileJSX(string(data))
	body, _, err := extractDefaultExport(source)
	if err != nil { t.Fatal(err) }
	localFuncs := extractFunctions(source)
	scope := make(map[string]*jsValue)
	for name, fn := range localFuncs { scope[name] = fn }
	stubHooks(&jsEval{scope: scope})
	ev := newJSEval(body, scope)
	result := ev.evalStatements()
	if result.typ != jsTypeVNode { t.Fatalf("got type %d", result.typ) }
	html := RenderSSRHTML(result.vnode)
	t.Logf("Interpreted HTML length: %d", len(html))
	if !strings.Contains(html, "Live Dashboard") { t.Error("missing heading") }
	if !strings.Contains(html, "Disconnected") { t.Error("missing Disconnected") }
	if strings.Contains(html, `connected="Live"`) { t.Error("corrupted: connected as attr") }
	if strings.Contains(html, `h="p"`) { t.Error("corrupted: h as attr") }
}

func TestLivePageSSR_FullPipeline(t *testing.T) {
	// Simulate the full evaluatePageSSR + wrapInLayouts pipeline
	data, err := os.ReadFile("../../_example/app/live/page.jsx")
	if err != nil { t.Skip() }
	source := TranspileJSX(string(data))
	body, funcParams, err := extractDefaultExport(source)
	if err != nil { t.Fatal(err) }
	localFuncs := extractFunctions(source)
	tokens := jsTokenize(body)
	compiled := compilePageTokens(tokens, localFuncs)

	// Build scope like evaluatePageSSR does
	scope := make(map[string]*jsValue)
	scope["data"] = jvNull
	scope["params"] = jvObj(map[string]*jsValue{})
	stubHooks(&jsEval{scope: scope})
	for name, fn := range localFuncs { scope[name] = fn }
	if strings.Contains(funcParams, "{") {
		// destructured
	}

	var html string

	// Try compiled path
	if compiled != nil {
		html = compiled.renderHTML(scope)
		if html != "" {
			t.Logf("Compiled path produced HTML length: %d", len(html))
		}
	}

	// Interpreted path
	if html == "" {
		tokens2 := make([]tok, len(tokens))
		copy(tokens2, tokens)
		ev := &jsEval{tokens: tokens2, pos: 0, scope: scope}
		result := ev.evalStatements()
		if result.typ == jsTypeVNode && result.vnode != nil {
			html = RenderSSRHTMLPooled(result.vnode)
		}
		t.Logf("Interpreted path produced HTML length: %d", len(html))
	}

	if !strings.Contains(html, "Live Dashboard") { t.Error("missing heading") }
	if !strings.Contains(html, "Disconnected") { t.Error("missing Disconnected") }
	if strings.Contains(html, `connected="Live"`) { t.Error("corrupted: connected as attr") }
	if strings.Contains(html, `h="p"`) { t.Error("corrupted: h as attr") }

	// Now wrap in layout
	layoutData, err := os.ReadFile("../../_example/app/layout.jsx")
	if err != nil { t.Skip("no layout") }
	layoutSource := TranspileJSX(string(layoutData))
	layoutBody, layoutParams, err := extractDefaultExport(layoutSource)
	if err != nil { t.Fatal("layout extract:", err) }
	layoutFuncs := extractFunctions(layoutSource)
	layoutTokens := jsTokenize(layoutBody)
	_ = layoutParams

	layoutScope := make(map[string]*jsValue)
	for name, fn := range layoutFuncs { layoutScope[name] = fn }
	stubHooks(&jsEval{scope: layoutScope})
	layoutScope["children"] = jvNode(&ssrNode{Tag: "lungo-children"})

	ltokens := make([]tok, len(layoutTokens))
	copy(ltokens, layoutTokens)
	lev := &jsEval{tokens: ltokens, pos: 0, scope: layoutScope}
	lresult := lev.evalStatements()
	if lresult.typ != jsTypeVNode { t.Fatal("layout not vnode") }
	layoutHTML := RenderSSRHTML(lresult.vnode)
	finalHTML := strings.Replace(layoutHTML, "<lungo-children></lungo-children>", html, 1)

	t.Logf("Final HTML length: %d", len(finalHTML))
	if !strings.Contains(finalHTML, "Live Dashboard") { t.Error("final: missing heading") }
	if !strings.Contains(finalHTML, "Disconnected") { t.Error("final: missing Disconnected") }
	if strings.Contains(finalHTML, `connected="Live"`) { t.Error("final: corrupted connected") }
	if strings.Contains(finalHTML, `h="p"`) { t.Error("final: corrupted h attr") }
}
