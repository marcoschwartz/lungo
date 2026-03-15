package lungo

import (
	"os"
	"strings"
	"testing"
)

func TestLayoutWithTernaryAndChildren(t *testing.T) {
	src := `
const menuOpen = false;
const children = "PAGE_CONTENT";
return h("div", {class: "wrapper"},
  h("nav", null,
    h("a", {href: "/"}, "Home"),
    menuOpen ? h("div", null, "Menu") : null
  ),
  h("main", null, children),
  h("footer", null, "Footer")
);`
	scope := map[string]*jsValue{}
	ev := newJSEval(src, scope)
	result := ev.evalStatements()
	if result.typ != jsTypeVNode {
		t.Fatalf("expected vnode, got %d", result.typ)
	}
	html := RenderSSRHTML(result.vnode)
	if !strings.Contains(html, "<main>") {
		t.Errorf("missing <main> in: %s", html)
	}
	if !strings.Contains(html, "<footer>") {
		t.Errorf("missing <footer> in: %s", html)
	}
}

func TestArrowFuncInProps(t *testing.T) {
	src := `h("button", {onclick: () => doStuff(), class: "btn"}, "Click")`
	scope := map[string]*jsValue{}
	result := jsEvalExpr(src, scope)
	if result.typ != jsTypeVNode {
		t.Fatalf("expected vnode, got %d", result.typ)
	}
	html := RenderSSRHTML(result.vnode)
	if !strings.Contains(html, "Click") {
		t.Errorf("missing button text in: %s", html)
	}
}

func TestTernaryAsHChild(t *testing.T) {
	src := `h("div", null, false ? h("p", null, "yes") : null, h("span", null, "after"))`
	scope := map[string]*jsValue{}
	result := jsEvalExpr(src, scope)
	html := RenderSSRHTML(result.vnode)
	if !strings.Contains(html, "<span>after</span>") {
		t.Errorf("missing span in: %s", html)
	}
}

func TestRealLayoutSSR(t *testing.T) {
	data, err := os.ReadFile("../../_example/app/layout.jsx")
	if err != nil {
		t.Skip("example layout not found")
	}
	source := TranspileJSX(string(data))

	body, _, err := extractDefaultExport(source)
	if err != nil {
		t.Fatalf("extractDefaultExport: %v", err)
	}

	localFuncs := extractFunctions(source)
	scope := make(map[string]*jsValue)
	for name, fn := range localFuncs {
		scope[name] = fn
	}

	// Stub hooks
	scope["useState"] = &jsValue{typ: jsTypeFunc, str: "__hook_useState"}
	scope["useEffect"] = &jsValue{typ: jsTypeFunc, str: "__hook_useEffect"}
	scope["useRouter"] = &jsValue{typ: jsTypeFunc, str: "__hook_useRouter"}
	scope["useRef"] = &jsValue{typ: jsTypeFunc, str: "__hook_useRef"}

	// children placeholder
	scope["children"] = jvNode(&ssrNode{Tag: "lungo-children"})

	ev := newJSEval(body, scope)
	result := ev.evalStatements()
	if result.typ != jsTypeVNode {
		t.Fatalf("expected vnode, got type %d", result.typ)
	}
	html := RenderSSRHTML(result.vnode)
	t.Logf("HTML length: %d", len(html))
	t.Logf("HTML (first 500): %s", html[:min(500, len(html))])
	t.Logf("HTML (last 500): %s", html[max(0, len(html)-500):])

	if !strings.Contains(html, "<main") {
		t.Errorf("missing <main> in layout HTML")
	}
	if !strings.Contains(html, "<footer") {
		t.Errorf("missing <footer> in layout HTML")
	}
	if !strings.Contains(html, "lungo-children") {
		t.Errorf("missing children placeholder in layout HTML")
	}
	if !strings.Contains(html, "Lungo") {
		t.Errorf("missing brand name in layout HTML")
	}
}

func min(a, b int) int {
	if a < b { return a }
	return b
}
