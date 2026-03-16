package lungo

import (
	"strings"
	"testing"
)

func TestSSRDebugComponentProps(t *testing.T) {
	// Manually simulate what evaluatePageSSR does

	source := TranspileJSX(`
const { h } = window.Lungo;

function Greeting({ name }) {
  if (!name) return (h("div", {class: "no-name"}, "Anonymous"));
  return (h("div", {class: "has-name"}, "Hello ", name));
}

export default function Page() {
  return (h("div", null, h(Greeting, {name: "World"}), h(Greeting, null)));
}
`)
	t.Logf("Transpiled source:\n%s", source)

	// Extract functions
	funcs := extractFunctions(source)
	t.Logf("Functions found: %d", len(funcs))
	for name, fn := range funcs {
		t.Logf("  %s: params=%v body=%s", name, fn.fnParams, truncate(fn.fnBody, 100))
	}

	// Extract default export
	body, params, err := extractDefaultExport(source)
	if err != nil {
		t.Fatalf("extractDefaultExport: %v", err)
	}
	t.Logf("Default export params: %q", params)
	t.Logf("Default export body: %s", truncate(body, 200))

	// Build scope
	scope := make(map[string]*jsValue)
	scope["data"] = jvNull
	scope["params"] = jvObj(map[string]*jsValue{})
	stubHooks(&jsEval{scope: scope})
	for name, fn := range funcs {
		scope[name] = fn
	}

	// Try compiled path
	tokens := jsTokenize(body)
	compiled := compilePageTokens(tokens, funcs)
	if compiled != nil {
		html := compiled.renderHTML(scope)
		t.Logf("Compiled HTML: %s", html)
		if strings.Contains(html, "has-name") {
			t.Log("COMPILED: props work correctly")
		} else {
			t.Error("COMPILED: props NOT working — name prop lost")
		}
	} else {
		t.Log("Compilation failed, trying interpreted")
	}

	// Try interpreted path
	tokens2 := jsTokenize(body)
	ev := &jsEval{tokens: tokens2, pos: 0, scope: scope}
	result := ev.evalStatements()
	if result.typ == jsTypeVNode && result.vnode != nil {
		html := RenderSSRHTML(result.vnode)
		t.Logf("Interpreted HTML: %s", html)
		if strings.Contains(html, "has-name") {
			t.Log("INTERPRETED: props work correctly")
		} else {
			t.Error("INTERPRETED: props NOT working — name prop lost")
		}
	} else {
		t.Logf("Interpreted: no vnode returned (type=%d)", result.typ)
	}

	// Direct callFunc test
	t.Log("\n--- Direct callFunc test ---")
	greeting := funcs["Greeting"]
	if greeting == nil {
		t.Fatal("Greeting function not found")
	}
	t.Logf("Greeting fnParams: %v", greeting.fnParams)
	t.Logf("Greeting fnBody: %s", truncate(greeting.fnBody, 200))

	props := map[string]*jsValue{
		"name": jvStr("Direct"),
	}

	// Simulate what callFunc does manually
	childScope := make(map[string]*jsValue, len(scope)+len(props))
	for k, v := range scope {
		childScope[k] = v
	}
	for k, v := range props {
		childScope[k] = v
	}
	t.Logf("childScope['name'] = %v (type=%d, str=%q)", childScope["name"], childScope["name"].typ, childScope["name"].str)

	// Test !name evaluation directly
	notNameTokens := jsTokenize("!name")
	notNameEv := &jsEval{tokens: notNameTokens, pos: 0, scope: childScope}
	notNameResult := notNameEv.expr()
	t.Logf("!name evaluates to: type=%d, truthy=%v, str=%q, num=%f", notNameResult.typ, notNameResult.truthy(), notNameResult.str, notNameResult.num)

	nameTokens := jsTokenize("name")
	nameEv := &jsEval{tokens: nameTokens, pos: 0, scope: childScope}
	nameResult := nameEv.expr()
	t.Logf("name evaluates to: type=%d, truthy=%v, str=%q", nameResult.typ, nameResult.truthy(), nameResult.str)

	// Check tokens
	bodyTokens := jsTokenize(greeting.fnBody)
	t.Logf("Total tokens: %d", len(bodyTokens))
	for i := 0; i < 15 && i < len(bodyTokens); i++ {
		tk := bodyTokens[i]
		t.Logf("  tok[%d] t=%d v=%q n=%f", i, tk.t, tk.v, tk.n)
	}
	// Test skipSingleStatement
	skipTokens := jsTokenize(`return (h("div", {class: "no-name"}, "Anonymous")); return (h("div", {class: "yes"}, "Found"));`)
	skipEv := &jsEval{tokens: skipTokens, pos: 0, scope: childScope}
	skipEv.skipSingleStatement()
	t.Logf("After skipSingleStatement, pos=%d, next token: t=%d v=%q", skipEv.pos, skipEv.peek().t, skipEv.peek().v)

	// Evaluate the body directly
	bodyTokens = jsTokenize(greeting.fnBody)
	testEv := &jsEval{tokens: bodyTokens, pos: 0, scope: childScope}
	testResult := testEv.evalStatements()
	t.Logf("Direct eval result type: %d", testResult.typ)
	if testResult.typ == jsTypeVNode && testResult.vnode != nil {
		html := RenderSSRHTML(testResult.vnode)
		t.Logf("Direct eval HTML: %s", html)
	}

	ev2 := &jsEval{scope: scope}
	result2 := ev2.callFunc(greeting, props)
	t.Logf("callFunc result type: %d", result2.typ)
	if result2.typ == jsTypeVNode && result2.vnode != nil {
		html := RenderSSRHTML(result2.vnode)
		t.Logf("Direct callFunc HTML: %s", html)
		if strings.Contains(html, "has-name") {
			t.Log("DIRECT CALL: props work correctly")
		} else {
			t.Error("DIRECT CALL: props NOT working")
		}
	}
}
