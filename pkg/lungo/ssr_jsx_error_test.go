package lungo

import (
	"os"
	"strings"
	"testing"
)

func TestJSXMismatchShowsErrorOverlay(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	// Intentionally mismatched tags
	os.WriteFile(dir+"/app/page.jsx", []byte(`
const { h } = window.Lungo;
export default function Page() {
  return (<div><MyComponent name="test"></div></div>);
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})
	route := app.router.Match("/")
	if route == nil {
		t.Fatal("no route")
	}

	html := app.renderPage(route, nil, nil)
	t.Logf("HTML length: %d", len(html))

	if !strings.Contains(html, "BUILD ERROR") {
		t.Error("should show BUILD ERROR overlay in dev mode")
	}
	if !strings.Contains(html, "JSX") {
		t.Error("should mention JSX in error")
	}
	if !strings.Contains(html, "mismatch") {
		t.Error("should mention mismatch")
	}
}

func TestJSXMismatchDetection(t *testing.T) {
	source := `<div><Component name="x"></span></div>`
	_, errors := TranspileJSXWithErrors(source)
	if len(errors) == 0 {
		t.Error("should detect tag mismatch")
	} else {
		t.Logf("Error: %s", errors[0])
		if !strings.Contains(errors[0], "mismatch") {
			t.Error("error should mention mismatch")
		}
	}
}
