package lungo

import (
	"os"
	"strings"
	"testing"
)

func TestSSRInlineStyleString(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	os.WriteFile(dir+"/app/page.jsx", []byte(`
const { h } = window.Lungo;
export default function Page() {
  return (
    <div style="overflow: visible; color: red;">
      <span style="font-weight: bold;">Hello</span>
    </div>
  );
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})
	html, _, err := app.evaluatePageSSR("page.js", nil, nil)
	if err != nil {
		t.Fatalf("SSR error: %v", err)
	}
	t.Logf("HTML: %s", html)

	if !strings.Contains(html, `style="overflow: visible`) {
		t.Error("should contain inline style string for div")
	}
	if !strings.Contains(html, `style="font-weight: bold`) {
		t.Error("should contain inline style string for span")
	}
}

func TestSSRInlineStyleOnComponent(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	os.WriteFile(dir+"/app/page.jsx", []byte(`
const { h } = window.Lungo;
export default function Page() {
  return (
    <aside class="sidebar" style="overflow: visible;">
      <div style="position: relative;">Content</div>
    </aside>
  );
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})
	html, _, err := app.evaluatePageSSR("page.js", nil, nil)
	if err != nil {
		t.Fatalf("SSR error: %v", err)
	}
	t.Logf("HTML: %s", html)

	if !strings.Contains(html, `style="overflow: visible;"`) {
		t.Error("aside should have style attribute")
	}
	if !strings.Contains(html, `style="position: relative;"`) {
		t.Error("div should have style attribute")
	}
}
