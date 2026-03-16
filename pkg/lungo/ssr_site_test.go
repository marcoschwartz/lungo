package lungo

import (
	"io/fs"
	"os"
	"strings"
	"testing"
)

// TestSSREvaluateLungoSitePages tests SSR evaluation against the actual lungo-site pages.
func TestSSREvaluateLungoSitePages(t *testing.T) {
	siteAppDir := "../../../lungo-site/app"
	if _, err := os.Stat(siteAppDir); err != nil {
		t.Skip("lungo-site not found at", siteAppDir)
	}

	app := New(Options{
		AppDir:    siteAppDir,
		StaticDir: "../../../lungo-site/static",
		Dev:       true, // skip cache so we always re-evaluate
	})

	pages := []string{
		"page.js",
		"features/page.js",
		"benchmarks/page.js",
		"about/page.js",
		"contact/page.js",
	}

	for _, page := range pages {
		t.Run(page, func(t *testing.T) {
			html, interactive, err := app.evaluatePageSSR(page, nil, nil)
			if err != nil {
				t.Errorf("evaluatePageSSR(%s) error: %v", page, err)
				return
			}
			if html == "" {
				t.Errorf("evaluatePageSSR(%s) returned empty HTML", page)
				return
			}
			t.Logf("%s: SSR OK (%d bytes, interactive=%v)", page, len(html), interactive)
			// Check it contains some expected content
			if page == "page.js" && !containsAny(html, "Lungo", "lungo") {
				t.Errorf("home page should contain 'Lungo', got: %s", truncate(html, 200))
			}
		})
	}
}

// TestSSREvaluateSimplePage tests SSR with a minimal page to isolate issues.
func TestSSREvaluateSimplePage(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	// Minimal page
	os.WriteFile(dir+"/app/page.jsx", []byte(`
const { h } = window.Lungo;
export default function Page() {
  return (<div><h1>Hello</h1></div>);
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})
	html, _, err := app.evaluatePageSSR("page.js", nil, nil)
	if err != nil {
		t.Fatalf("simple page SSR error: %v", err)
	}
	if html == "" {
		t.Fatal("simple page SSR returned empty HTML")
	}
	t.Logf("Simple page SSR: %s", html)
}

// TestSSREvaluatePageWithComponents tests SSR with helper components.
func TestSSREvaluatePageWithComponents(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	os.WriteFile(dir+"/app/page.jsx", []byte(`
const { h } = window.Lungo;

function Card({ title, description }) {
  return (
    <div class="card">
      <h3>{title}</h3>
      <p>{description}</p>
    </div>
  );
}

export default function Page() {
  return (
    <div>
      <h1>Features</h1>
      <Card title="SSR" description="Server rendering" />
      <Card title="Routing" description="File based" />
    </div>
  );
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})
	html, _, err := app.evaluatePageSSR("page.js", nil, nil)
	if err != nil {
		t.Fatalf("component page SSR error: %v", err)
	}
	if html == "" {
		t.Fatal("component page SSR returned empty HTML")
	}
	t.Logf("Component page SSR: %s", html)
	if !containsAny(html, "SSR", "Routing") {
		t.Errorf("expected card content in HTML: %s", truncate(html, 300))
	}
}

// TestSSREvaluatePageWithTernary tests SSR with ternary/conditional rendering.
func TestSSREvaluatePageWithTernary(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	os.WriteFile(dir+"/app/page.jsx", []byte(`
const { h } = window.Lungo;

export default function Page() {
  const show = true;
  return (
    <div>
      {show ? <p>visible</p> : null}
      <span>always</span>
    </div>
  );
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})
	html, _, err := app.evaluatePageSSR("page.js", nil, nil)
	if err != nil {
		t.Fatalf("ternary page SSR error: %v", err)
	}
	t.Logf("Ternary page SSR: %s", html)
}

// TestSSREvaluatePageWithMap tests SSR with .map() rendering.
func TestSSREvaluatePageWithMap(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	os.WriteFile(dir+"/app/page.jsx", []byte(`
const { h } = window.Lungo;

export default function Page() {
  const items = ["one", "two", "three"];
  return (
    <ul>
      {items.map(item => <li>{item}</li>)}
    </ul>
  );
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})
	html, _, err := app.evaluatePageSSR("page.js", nil, nil)
	if err != nil {
		t.Fatalf("map page SSR error: %v", err)
	}
	t.Logf("Map page SSR: %s", html)
}

// TestSSREvaluatePageWithSVG tests SSR with inline SVG (common in lungo-site).
func TestSSREvaluatePageWithSVG(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	os.WriteFile(dir+"/app/page.jsx", []byte(`
const { h } = window.Lungo;

export default function Page() {
  return (
    <div>
      <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path d="M5 12h14M12 5l7 7-7 7"/>
      </svg>
      <p>After SVG</p>
    </div>
  );
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})
	html, _, err := app.evaluatePageSSR("page.js", nil, nil)
	if err != nil {
		t.Fatalf("SVG page SSR error: %v", err)
	}
	t.Logf("SVG page SSR: %s", html)
}

// TestSSREvaluatePageWithTemplateLiteral tests SSR with template literal strings.
func TestSSREvaluatePageWithTemplateLiteral(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	os.WriteFile(dir+"/app/page.jsx", []byte(`
const { h } = window.Lungo;

export default function Page() {
  const name = "World";
  return (
    <div>
      <pre><code>{`+"`Hello ${name}!`"+`}</code></pre>
    </div>
  );
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})
	html, _, err := app.evaluatePageSSR("page.js", nil, nil)
	if err != nil {
		t.Fatalf("template literal page SSR error: %v", err)
	}
	t.Logf("Template literal page SSR: %s", html)
}

// TestSSRFullRenderPage tests the full renderPage pipeline.
func TestSSRFullRenderPage(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	os.WriteFile(dir+"/app/page.jsx", []byte(`
const { h } = window.Lungo;

export const metadata = {
  title: "Test Page",
  description: "A test"
};

export default function Page() {
  return (<div><h1>Hello SSR</h1></div>);
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})
	route := app.router.Match("/")
	if route == nil {
		t.Fatal("no route matched /")
	}

	html := app.renderPage(route, nil, nil)
	t.Logf("Full render (%d bytes): %s", len(html), truncate(html, 500))

	if !containsAny(html, "Hello SSR") {
		t.Error("renderPage should contain SSR content")
	}
	if !containsAny(html, "<title>Test Page</title>") {
		t.Error("renderPage should contain metadata title")
	}
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// Silence unused import
var _ fs.FS
