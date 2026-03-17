package lungo

import (
	"os"
	"strings"
	"testing"
)

func ssrQuick(t *testing.T, source string) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)
	os.WriteFile(dir+"/app/page.jsx", []byte(source), 0644)
	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})
	html, _, err := app.evaluatePageSSR("page.js", nil, nil)
	if err != nil {
		t.Fatalf("SSR error: %v", err)
	}
	return html
}

// ── Template Literals ──────────────────────────────────

func TestSSR_TemplateLiteral(t *testing.T) {
	html := ssrQuick(t, `const { h } = window.Lungo;
export default function Page() {
  const name = "World";
  const greeting = ` + "`Hello ${name}!`" + `;
  return (<div>{greeting}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "Hello World!") {
		t.Error("template literal should interpolate ${name}")
	}
}

func TestSSR_TemplateLiteralMultiple(t *testing.T) {
	html := ssrQuick(t, `const { h } = window.Lungo;
export default function Page() {
  const a = "foo";
  const b = 42;
  const s = ` + "`${a}-${b}-end`" + `;
  return (<div>{s}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "foo-42-end") {
		t.Error("should interpolate multiple expressions")
	}
}

func TestSSR_TemplateLiteralExpr(t *testing.T) {
	html := ssrQuick(t, `const { h } = window.Lungo;
export default function Page() {
  const x = 3;
  const s = ` + "`result: ${x * 2}`" + `;
  return (<div>{s}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "result: 6") {
		t.Error("should evaluate expression inside ${}")
	}
}

// ── For Loops ──────────────────────────────────────────

func TestSSR_ForLoop(t *testing.T) {
	html := ssrQuick(t, `const { h } = window.Lungo;
export default function Page() {
  let sum = 0;
  for (let i = 0; i < 5; i++) {
    sum += i;
  }
  return (<div>{sum}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "10") {
		t.Error("for loop should compute sum 0+1+2+3+4=10")
	}
}

func TestSSR_ForOfLoop(t *testing.T) {
	html := ssrQuick(t, `const { h } = window.Lungo;
export default function Page() {
  const items = ["a", "b", "c"];
  let result = "";
  for (const item of items) {
    result += item;
  }
  return (<div>{result}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "abc") {
		t.Error("for...of should concatenate items")
	}
}

func TestSSR_WhileLoopSimple(t *testing.T) {
	html := ssrQuick(t, `const { h } = window.Lungo;
export default function Page() {
  let count = 0;
  while (count < 3) {
    count++;
  }
  return (<div>{count}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "3") {
		t.Error("while loop should increment to 3")
	}
}

func TestSSR_WhileLoop(t *testing.T) {
	html := ssrQuick(t, `const { h } = window.Lungo;
export default function Page() {
  let n = 0;
  let i = 0;
  while (i < 5) {
    n += 1;
    i++;
  }
  return (<div>{n}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "5") {
		t.Error("while loop should compute 5")
	}
}

// ── JSON.parse ─────────────────────────────────────────

func TestSSR_JSONParse(t *testing.T) {
	html := ssrQuick(t, `const { h } = window.Lungo;
export default function Page() {
  const data = JSON.parse('{"name":"hello","count":3}');
  return (<div>{data.name} {data.count}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "hello") { t.Error("should parse name") }
	if !strings.Contains(html, "3") { t.Error("should parse count") }
}

// ── parseInt / parseFloat / Number ─────────────────────

func TestSSR_NumberConversion(t *testing.T) {
	html := ssrQuick(t, `const { h } = window.Lungo;
export default function Page() {
  const a = parseInt("42");
  const b = parseFloat("3.14");
  const c = Number("100");
  return (<div>{a} {b} {c}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "42") { t.Error("parseInt") }
	if !strings.Contains(html, "3.14") { t.Error("parseFloat") }
	if !strings.Contains(html, "100") { t.Error("Number") }
}

// ── Array.from ─────────────────────────────────────────

func TestSSR_ArrayFrom(t *testing.T) {
	html := ssrQuick(t, `const { h } = window.Lungo;
export default function Page() {
  const arr = Array.from("hello");
  return (<div>{arr.length} {arr[0]}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "5") { t.Error("Array.from string should have length 5") }
	if !strings.Contains(html, "h") { t.Error("first char should be h") }
}

// ── Try/Catch ──────────────────────────────────────────

func TestSSR_TryCatch(t *testing.T) {
	html := ssrQuick(t, `const { h } = window.Lungo;
export default function Page() {
  let result = "initial";
  try {
    result = "from try";
  } catch (e) {
    result = "from catch";
  }
  return (<div>{result}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "from try") {
		t.Error("try block should execute")
	}
}

// ── Console (no-op) ────────────────────────────────────

func TestSSR_ConsoleLog(t *testing.T) {
	html := ssrQuick(t, `const { h } = window.Lungo;
export default function Page() {
  console.log("test");
  console.warn("warning");
  return (<div>works</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "works") {
		t.Error("console.log should be no-op")
	}
}

// ── Increment/Decrement ────────────────────────────────

func TestSSR_IncrementDecrement(t *testing.T) {
	html := ssrQuick(t, `const { h } = window.Lungo;
export default function Page() {
  let a = 5;
  a++;
  let b = 10;
  b--;
  return (<div>{a} {b}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "6") { t.Error("a++ should be 6") }
	if !strings.Contains(html, "9") { t.Error("b-- should be 9") }
}

// ── Assignment operators ───────────────────────────────

func TestSSR_AssignmentOperators(t *testing.T) {
	html := ssrQuick(t, `const { h } = window.Lungo;
export default function Page() {
  let x = 10;
  x += 5;
  let y = 20;
  y -= 3;
  let s = "hello";
  s += " world";
  return (<div>{x} {y} {s}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "15") { t.Error("x += 5 should be 15") }
	if !strings.Contains(html, "17") { t.Error("y -= 3 should be 17") }
	if !strings.Contains(html, "hello world") { t.Error("s += should concat") }
}

// ── Reassignment ───────────────────────────────────────

func TestSSR_Reassignment(t *testing.T) {
	html := ssrQuick(t, `const { h } = window.Lungo;
export default function Page() {
  let val = "first";
  val = "second";
  return (<div>{val}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "second") { t.Error("reassignment should work") }
}
