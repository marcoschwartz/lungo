package lungo

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// TestAutoInjectH_MissingWindowLungo verifies that JSX files without
// "const { h } = window.Lungo" get it auto-injected when served.
// This prevents "TypeError: h is not a function" at runtime.
func TestAutoInjectH_MissingWindowLungo(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	// A component file with NO window.Lungo destructuring and NO React import.
	// The JSX transpiler will convert <div> to h("div",...) but h won't be in scope.
	os.WriteFile(dir+"/app/card.jsx", []byte(`
export default function Card({ title }) {
  return (<div class="card"><h2>{title}</h2></div>);
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})

	req := httptest.NewRequest("GET", "/app/card.js", nil)
	w := httptest.NewRecorder()
	app.serveAppFile(w, req, "card.js")

	body := w.Body.String()

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(body, "window.Lungo") {
		t.Errorf("expected auto-injected window.Lungo destructuring, got:\n%s", body)
	}
	if !strings.Contains(body, "const { h,") {
		t.Errorf("expected h in destructuring, got:\n%s", body)
	}
	// Verify the transpiled JSX uses h()
	if !strings.Contains(body, `h("div"`) {
		t.Errorf("expected transpiled h() calls, got:\n%s", body)
	}
}

// TestAutoInjectH_AlreadyHasWindowLungo verifies that JSX files that
// already have window.Lungo do NOT get a duplicate injection.
func TestAutoInjectH_AlreadyHasWindowLungo(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	os.WriteFile(dir+"/app/page.jsx", []byte(`
const { h, useState } = window.Lungo;
export default function Page() {
  return (<div><h1>Hello</h1></div>);
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})

	req := httptest.NewRequest("GET", "/app/page.js", nil)
	w := httptest.NewRecorder()
	app.serveAppFile(w, req, "page.js")

	body := w.Body.String()

	// Count occurrences of window.Lungo — should be exactly 1 (the original)
	count := strings.Count(body, "window.Lungo")
	if count != 1 {
		t.Errorf("expected exactly 1 window.Lungo reference, got %d in:\n%s", count, body)
	}
}

// TestAutoInjectH_ReactImportConverted verifies that files with React imports
// get converted by NextCompat and don't get double-injected.
func TestAutoInjectH_ReactImportConverted(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	os.WriteFile(dir+"/app/page.jsx", []byte(`
import { useState } from "react";
export default function Page() {
  const [count, setCount] = useState(0);
  return (<div><span>{count}</span></div>);
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})

	req := httptest.NewRequest("GET", "/app/page.js", nil)
	w := httptest.NewRecorder()
	app.serveAppFile(w, req, "page.js")

	body := w.Body.String()

	// NextCompat should convert the React import to window.Lungo
	if !strings.Contains(body, "window.Lungo") {
		t.Errorf("expected window.Lungo after NextCompat, got:\n%s", body)
	}
	// Should NOT be double-injected
	count := strings.Count(body, "window.Lungo")
	if count != 1 {
		t.Errorf("expected exactly 1 window.Lungo reference, got %d in:\n%s", count, body)
	}
}

// TestAutoInjectH_PlainJSNoJSX verifies that plain .js files (no JSX)
// are served without injection.
func TestAutoInjectH_PlainJSNoJSX(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	os.WriteFile(dir+"/app/utils.js", []byte(`
export function formatDate(d) {
  return d.toISOString().split("T")[0];
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})

	req := httptest.NewRequest("GET", "/app/utils.js", nil)
	w := httptest.NewRecorder()
	app.serveAppFile(w, req, "utils.js")

	body := w.Body.String()

	// Plain JS files should NOT get window.Lungo injected
	if strings.Contains(body, "window.Lungo") {
		t.Errorf("plain JS file should not get window.Lungo injection, got:\n%s", body)
	}
}

// TestAutoInjectH_NestedComponentFile tests a realistic scenario:
// a sub-component in a nested directory without its own imports.
func TestAutoInjectH_NestedComponentFile(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app/components", 0755)
	os.MkdirAll(dir+"/static", 0755)

	// A component that uses JSX + map (the exact pattern from the bug report)
	os.WriteFile(dir+"/app/components/list.jsx", []byte(`
export default function List({ items }) {
  return (
    <ul>
      {items.map(item => (
        <li class="item">{item.name}</li>
      ))}
    </ul>
  );
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})

	req := httptest.NewRequest("GET", "/app/components/list.js", nil)
	w := httptest.NewRecorder()
	app.serveAppFile(w, req, "components/list.js")

	body := w.Body.String()

	if !strings.Contains(body, "window.Lungo") {
		t.Errorf("nested component should get auto-injected window.Lungo, got:\n%s", body)
	}
	if !strings.Contains(body, `h("ul"`) {
		t.Errorf("expected transpiled h() calls for <ul>, got:\n%s", body)
	}
	if !strings.Contains(body, `h("li"`) {
		t.Errorf("expected transpiled h() calls for <li>, got:\n%s", body)
	}
}
