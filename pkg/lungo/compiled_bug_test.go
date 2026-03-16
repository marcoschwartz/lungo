package lungo

import (
	"strings"
	"testing"
)

func TestCompiled_TernaryClassConcat(t *testing.T) {
	src := `h("span", {class: "base " + (x ? "green" : "red")}, h("span", {class: "dot " + (x ? "pulse" : "stop")}), x ? "Live" : "Off")`
	scope := map[string]*jsValue{"x": jvFalse}

	// Interpreted
	r1 := jsEvalExpr(src, scope)
	html1 := RenderSSRHTML(r1.vnode)
	t.Logf("Interpreted: %s", html1)

	// Compiled
	tokens := jsTokenize(src)
	tokens = append(tokens, tok{t: tokEOF})
	c := &compiler{tokens: tokens, pos: 0, funcs: map[string]*jsValue{}}
	node := c.compileHCallAsNode()
	if node == nil {
		t.Fatal("compileHCallAsNode returned nil")
	}
	scope2 := map[string]*jsValue{"x": jvFalse}
	var sb strings.Builder
	node.renderDirectHTML(scope2, &sb)
	html2 := sb.String()
	t.Logf("Compiled:    %s", html2)

	if !strings.Contains(html1, "base red") { t.Error("interpreted: wrong class") }
	if !strings.Contains(html1, "Off") { t.Error("interpreted: missing Off") }

	if !strings.Contains(html2, "base red") { t.Error("compiled: wrong class") }
	if !strings.Contains(html2, "Off") { t.Error("compiled: missing Off") }
	if strings.Contains(html2, `x=`) { t.Error("compiled: x leaked as attr") }
}

func TestCompiled_PropsWithParenTernary(t *testing.T) {
	// Simpler: just test that props parsing doesn't eat children
	src := `h("span", {class: "a " + (b ? "x" : "y")}, "child")`
	tokens := jsTokenize(src)
	tokens = append(tokens, tok{t: tokEOF})
	c := &compiler{tokens: tokens, pos: 0, funcs: map[string]*jsValue{}}
	node := c.compileHCallAsNode()
	if node == nil { t.Fatal("nil node") }
	scope := map[string]*jsValue{"b": jvFalse}
	var sb strings.Builder
	node.renderDirectHTML(scope, &sb)
	html := sb.String()
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "a y") { t.Error("wrong class") }
	if !strings.Contains(html, "child") { t.Error("missing child text") }
}
