package lungo

import (
	"os"
	"strings"
	"testing"
)

func TestSSRLayoutWithInlineStyle(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	os.WriteFile(dir+"/app/layout.jsx", []byte(`
const { h } = window.Lungo;
export default function Layout({ children }) {
  return (
    <div class="app">
      <aside class="sidebar" style="overflow: visible;">
        <span>Nav</span>
      </aside>
      <main>{children}</main>
    </div>
  );
}
`), 0644)

	os.WriteFile(dir+"/app/page.jsx", []byte(`
const { h } = window.Lungo;
export default function Page() {
  return (<div>Hello</div>);
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})
	route := app.router.Match("/")
	if route == nil {
		t.Fatal("no route")
	}

	html := app.renderPage(route, nil, nil)
	t.Logf("HTML snippet: %s", truncate(html, 500))

	if !strings.Contains(html, `style="overflow: visible;"`) {
		t.Error("layout aside should have style attribute in SSR")
	}
	if !strings.Contains(html, "Hello") {
		t.Error("page content should be present")
	}
}
