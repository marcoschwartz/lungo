package lungo

import (
	"os"
	"strings"
	"testing"
)

func ssrPage(t *testing.T, source string) string {
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

func TestSSRArrayFind(t *testing.T) {
	html := ssrPage(t, `
const { h } = window.Lungo;
export default function Page() {
  const items = [{id: 1, name: "Alpha"}, {id: 2, name: "Beta"}, {id: 3, name: "Gamma"}];
  const found = items.find(p => p.id === 2);
  return (<div>{found ? found.name : "not found"}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "Beta") {
		t.Error("should find Beta by id === 2")
	}
}

func TestSSRArrayFindNotFound(t *testing.T) {
	html := ssrPage(t, `
const { h } = window.Lungo;
export default function Page() {
  const items = [{id: 1, name: "Alpha"}];
  const found = items.find(p => p.id === 99);
  return (<div>{found ? found.name : "not found"}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "not found") {
		t.Error("should return 'not found' when item doesn't exist")
	}
}

func TestSSRArrayFindWithStringComparison(t *testing.T) {
	html := ssrPage(t, `
const { h } = window.Lungo;
export default function Page() {
  const projects = [{id: 1, name: "Project A"}, {id: 2, name: "Project B"}];
  const currentId = "1";
  const current = projects.find(p => String(p.id) === String(currentId));
  return (<div>{current ? current.name : "none"}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "Project A") {
		t.Error("should find Project A with String() comparison")
	}
}

func TestSSRArraySome(t *testing.T) {
	html := ssrPage(t, `
const { h } = window.Lungo;
export default function Page() {
  const nums = [1, 2, 3, 4, 5];
  const hasEven = nums.some(n => n % 2 === 0);
  return (<div>{hasEven ? "yes" : "no"}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "yes") {
		t.Error("some should find even number")
	}
}

func TestSSRArrayEvery(t *testing.T) {
	html := ssrPage(t, `
const { h } = window.Lungo;
export default function Page() {
  const nums = [2, 4, 6];
  const allEven = nums.every(n => n % 2 === 0);
  return (<div>{allEven ? "all even" : "not all"}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "all even") {
		t.Error("every should confirm all even")
	}
}

func TestSSRArrayFindIndex(t *testing.T) {
	html := ssrPage(t, `
const { h } = window.Lungo;
export default function Page() {
  const items = ["a", "b", "c"];
  const idx = items.findIndex(x => x === "b");
  return (<div>{idx}</div>);
}`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "1") {
		t.Error("findIndex should return 1 for 'b'")
	}
}
