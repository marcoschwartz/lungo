package lungo

import (
	"os"
	"strings"
	"testing"
)

func ssrRender(t *testing.T, source string) string {
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

// ── Nullish Coalescing ──────────────────────────────────

func TestSSR_NullishCoalescing(t *testing.T) {
	html := ssrRender(t, `const { h } = window.Lungo;
export default function Page() {
  const a = null;
  const b = undefined;
  const c = "hello";
  const d = 0;
  return (<div>{a ?? "fallback"} {b ?? "default"} {c ?? "nope"} {d ?? "zero"}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "fallback") { t.Error("null ?? should use fallback") }
	if !strings.Contains(html, "default") { t.Error("undefined ?? should use fallback") }
	if !strings.Contains(html, "hello") { t.Error("string ?? should keep value") }
	if !strings.Contains(html, "0") { t.Error("0 ?? should keep 0 (not falsy like ||)") }
}

// ── String Methods ──────────────────────────────────────

func TestSSR_StringReplace(t *testing.T) {
	html := ssrRender(t, `const { h } = window.Lungo;
export default function Page() {
  const s = "hello world world";
  return (<div>{s.replace("world", "go")} | {s.replaceAll("world", "go")}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "hello go world") { t.Error("replace should only replace first") }
	if !strings.Contains(html, "hello go go") { t.Error("replaceAll should replace all") }
}

func TestSSR_StringStartsEndsWith(t *testing.T) {
	html := ssrRender(t, `const { h } = window.Lungo;
export default function Page() {
  const s = "/api/hello";
  return (<div>{s.startsWith("/api") ? "yes" : "no"} {s.endsWith("hello") ? "yes" : "no"} {s.startsWith("http") ? "yes" : "no"}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "yes") { t.Error("startsWith should work") }
}

func TestSSR_StringRepeat(t *testing.T) {
	html := ssrRender(t, `const { h } = window.Lungo;
export default function Page() {
  const s = "ab";
  return (<div>{s.repeat(3)}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "ababab") { t.Error("expected ababab") }
}

func TestSSR_StringCase(t *testing.T) {
	html := ssrRender(t, `const { h } = window.Lungo;
export default function Page() {
  const s = "Hello";
  return (<div>{s.toLowerCase()} {s.toUpperCase()}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "hello") { t.Error("expected lowercase hello") }
	if !strings.Contains(html, "HELLO") { t.Error("expected uppercase HELLO") }
}

func TestSSR_StringIndexOf(t *testing.T) {
	html := ssrRender(t, `const { h } = window.Lungo;
export default function Page() {
  const s = "hello";
  return (<div>{s.indexOf("ll")} {s.indexOf("xyz")}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "2") { t.Error("expected index 2") }
	if !strings.Contains(html, "-1") { t.Error("expected -1 for not found") }
}

func TestSSR_StringCharAt(t *testing.T) {
	html := ssrRender(t, `const { h } = window.Lungo;
export default function Page() {
  const s = "hello";
  return (<div>{s.charAt(1)}</div>);
}`)
	if !strings.Contains(html, "e") { t.Error("expected 'e'") }
}

func TestSSR_StringSubstring(t *testing.T) {
	html := ssrRender(t, `const { h } = window.Lungo;
export default function Page() {
  const s = "hello world";
  return (<div>{s.substring(6, 11)}</div>);
}`)
	if !strings.Contains(html, "world") { t.Error("expected 'world'") }
}

// ── Array Methods ───────────────────────────────────────

func TestSSR_ArrayReduce(t *testing.T) {
	html := ssrRender(t, `const { h } = window.Lungo;
export default function Page() {
  const nums = [1, 2, 3, 4, 5];
  const sum = nums.reduce((acc, n) => acc + n, 0);
  return (<div>{sum}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "15") { t.Error("expected sum 15") }
}

func TestSSR_ArrayReduceNoInit(t *testing.T) {
	html := ssrRender(t, `const { h } = window.Lungo;
export default function Page() {
  const nums = [10, 20, 30];
  const sum = nums.reduce((a, b) => a + b);
  return (<div>{sum}</div>);
}`)
	if !strings.Contains(html, "60") { t.Error("expected sum 60") }
}

func TestSSR_ArrayReduceToObject(t *testing.T) {
	html := ssrRender(t, `const { h } = window.Lungo;
export default function Page() {
  const items = [{id: "a", val: 1}, {id: "b", val: 2}];
  const count = items.reduce((acc, item) => acc + item.val, 0);
  return (<div>{count}</div>);
}`)
	if !strings.Contains(html, "3") { t.Error("expected 3") }
}

func TestSSR_ArrayConcat(t *testing.T) {
	html := ssrRender(t, `const { h } = window.Lungo;
export default function Page() {
  const a = [1, 2];
  const b = [3, 4];
  const c = a.concat(b);
  return (<div>{c.length}</div>);
}`)
	if !strings.Contains(html, "4") { t.Error("expected length 4") }
}

func TestSSR_ArrayReverse(t *testing.T) {
	html := ssrRender(t, `const { h } = window.Lungo;
export default function Page() {
  const arr = ["a", "b", "c"];
  const rev = arr.reverse();
  return (<div>{rev.join(",")}</div>);
}`)
	if !strings.Contains(html, "c,b,a") { t.Error("expected c,b,a") }
}

func TestSSR_ArrayFlat(t *testing.T) {
	html := ssrRender(t, `const { h } = window.Lungo;
export default function Page() {
  const arr = [[1, 2], [3, 4], [5]];
  const flat = arr.flat();
  return (<div>{flat.length}</div>);
}`)
	if !strings.Contains(html, "5") { t.Error("expected 5 elements") }
}

// ── Object Methods ──────────────────────────────────────

func TestSSR_ObjectValues(t *testing.T) {
	html := ssrRender(t, `const { h } = window.Lungo;
export default function Page() {
  const obj = {a: 1, b: 2, c: 3};
  const vals = Object.values(obj);
  return (<div>{vals.length}</div>);
}`)
	if !strings.Contains(html, "3") { t.Error("expected 3 values") }
}

func TestSSR_ObjectEntries(t *testing.T) {
	html := ssrRender(t, `const { h } = window.Lungo;
export default function Page() {
  const obj = {x: "hello", y: "world"};
  const entries = Object.entries(obj);
  return (<div>{entries.length}</div>);
}`)
	if !strings.Contains(html, "2") { t.Error("expected 2 entries") }
}

func TestSSR_ObjectAssign(t *testing.T) {
	html := ssrRender(t, `const { h } = window.Lungo;
export default function Page() {
  const a = {x: 1};
  const b = {y: 2};
  const merged = Object.assign({}, a, b);
  return (<div>{merged.x} {merged.y}</div>);
}`)
	if !strings.Contains(html, "1") { t.Error("expected x=1") }
	if !strings.Contains(html, "2") { t.Error("expected y=2") }
}
