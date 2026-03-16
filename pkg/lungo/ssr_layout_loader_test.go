package lungo

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestLayoutLoader(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app/dashboard", 0755)
	os.MkdirAll(dir+"/static", 0755)

	// Layout with loader — receives data prop with projects
	os.WriteFile(dir+"/app/layout.jsx", []byte(`
const { h, useRouter } = window.Lungo;

export const loader = { url: "/api/projects" };

export default function Layout({ children, data }) {
  const projects = data && data.projects ? data.projects : [];
  return (
    <div class="app">
      <nav>
        {projects.map(p => <span class="project">{p.name}</span>)}
      </nav>
      <main>{children}</main>
    </div>
  );
}
`), 0644)

	os.WriteFile(dir+"/app/dashboard/page.jsx", []byte(`
const { h } = window.Lungo;
export default function Dashboard() {
  return (<div class="dashboard">Welcome</div>);
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})

	// Register the API endpoint the layout loader will call
	app.API("/api/projects", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"projects": []map[string]string{
				{"name": "Project Alpha"},
				{"name": "Project Beta"},
			},
		})
	})
	app.buildHandler()

	// Match the route
	route := app.router.Match("/dashboard")
	if route == nil {
		t.Fatal("no route for /dashboard")
	}

	// Check layout loaders detected
	if route.LayoutLoaders == nil {
		t.Fatal("LayoutLoaders should not be nil")
	}
	if _, ok := route.LayoutLoaders["layout.js"]; !ok {
		t.Fatal("layout.js should have a loader")
	}
	t.Logf("Layout loaders: %v", route.LayoutLoaders)

	// Render the page with layout data
	html := app.renderPage(route, nil, nil)
	t.Logf("HTML (first 500): %s", truncate(html, 500))

	if !strings.Contains(html, "Project Alpha") {
		t.Error("layout should contain Project Alpha from loader data")
	}
	if !strings.Contains(html, "Project Beta") {
		t.Error("layout should contain Project Beta from loader data")
	}
	if !strings.Contains(html, "Welcome") {
		t.Error("page content should be present")
	}
	if !strings.Contains(html, "__LUNGO_LAYOUT_DATA__") {
		t.Error("should embed layout data for client hydration")
	}
}

func TestLayoutLoaderMultiSource(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	os.WriteFile(dir+"/app/layout.jsx", []byte(`
const { h } = window.Lungo;

export const loader = {
  user: "/api/user",
  projects: "/api/projects"
};

export default function Layout({ children, data }) {
  const name = data && data.user ? data.user.name : "Guest";
  return (
    <div>
      <header class="user">{name}</header>
      <main>{children}</main>
    </div>
  );
}
`), 0644)

	os.WriteFile(dir+"/app/page.jsx", []byte(`
const { h } = window.Lungo;
export default function Page() {
  return (<div>Home</div>);
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})

	app.API("/api/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"name": "Alice"})
	})
	app.API("/api/projects", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]string{{"name": "P1"}})
	})
	app.buildHandler()

	route := app.router.Match("/")
	if route == nil {
		t.Fatal("no route for /")
	}

	html := app.renderPage(route, nil, nil)
	t.Logf("HTML (first 300): %s", truncate(html, 300))

	if !strings.Contains(html, "Alice") {
		t.Error("layout should contain user name from multi-source loader")
	}
}

func TestLayoutWithoutLoader(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	// Layout WITHOUT loader — should still work, data is null
	os.WriteFile(dir+"/app/layout.jsx", []byte(`
const { h } = window.Lungo;
export default function Layout({ children }) {
  return (<div class="wrap">{children}</div>);
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

	if route.LayoutLoaders != nil {
		t.Error("should have no layout loaders")
	}

	html := app.renderPage(route, nil, nil)
	t.Logf("HTML contains Hello: %v", strings.Contains(html, "Hello"))
	t.Logf("HTML contains LAYOUT_DATA: %v", strings.Contains(html, "__LUNGO_LAYOUT_DATA__"))
	if !strings.Contains(html, "Hello") {
		t.Error("page should render without layout loader")
	}
	// The boot script always references layoutData, but the actual data object
	// should only be embedded when there are layout loaders
	if strings.Contains(html, "window.__LUNGO_LAYOUT_DATA__ = {") {
		t.Error("should NOT embed layout data object when no layout loaders")
	}
}
