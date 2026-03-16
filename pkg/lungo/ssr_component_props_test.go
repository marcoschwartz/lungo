package lungo

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

// TestSSRComponentWithArrayMapFromLoader tests that a component receiving
// array data from a layout loader can .map() over it during SSR.
func TestSSRComponentWithArrayMapFromLoader(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	os.WriteFile(dir+"/app/layout.jsx", []byte(`
const { h } = window.Lungo;

export const loader = { url: "/api/items" };

function ItemList({ items }) {
  if (!items || items.length < 1) return (<div class="empty"></div>);
  return (
    <ul class="item-list">
      {items.map(item => <li>{item.name}</li>)}
    </ul>
  );
}

export default function Layout({ children, data }) {
  const items = data && data.items ? data.items : [];
  return (
    <div class="layout">
      <ItemList items={items} />
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

	app.API("/api/items", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items": []map[string]string{
				{"name": "Alpha"},
				{"name": "Beta"},
				{"name": "Gamma"},
			},
		})
	})
	app.buildHandler()

	route := app.router.Match("/")
	if route == nil {
		t.Fatal("no route")
	}

	html := app.renderPage(route, nil, nil)
	t.Logf("HTML: %s", truncate(html, 800))

	if !strings.Contains(html, "Alpha") {
		t.Error("should contain Alpha from loader data")
	}
	if !strings.Contains(html, "Beta") {
		t.Error("should contain Beta from loader data")
	}
	if !strings.Contains(html, "item-list") {
		t.Error("should render item-list (not empty div)")
	}
	if !strings.Contains(html, "Hello") {
		t.Error("should contain page content")
	}
}

// TestSSRSelectWithOptionsFromMap tests <select> with <option> elements
// generated from .map() — the exact pattern that fails in ProjectSelector.
func TestSSRSelectWithOptionsFromMap(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	os.WriteFile(dir+"/app/layout.jsx", []byte(`
const { h } = window.Lungo;

export const loader = { url: "/api/projects" };

function ProjectSelector({ projects, currentId }) {
  if (!projects || projects.length < 2) return (<div class="no-projects"></div>);
  return (
    <div class="selector">
      <select name="project_id">
        {projects.map(p => (
          <option value={p.id}>{p.name}</option>
        ))}
      </select>
    </div>
  );
}

export default function Layout({ children, data }) {
  const projects = data && data.projects ? data.projects : [];
  const currentPid = data && data.current_project_id ? data.current_project_id : "";
  return (
    <div class="app">
      <ProjectSelector projects={projects} currentId={currentPid} />
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

	app.API("/api/projects", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"projects": []map[string]interface{}{
				{"id": 1, "name": "Project One"},
				{"id": 2, "name": "Project Two"},
				{"id": 3, "name": "Project Three"},
			},
			"current_project_id": "1",
		})
	})
	app.buildHandler()

	route := app.router.Match("/")
	if route == nil {
		t.Fatal("no route")
	}

	html := app.renderPage(route, nil, nil)
	t.Logf("HTML: %s", truncate(html, 800))

	if strings.Contains(html, "no-projects") {
		t.Error("should NOT render no-projects placeholder — 3 projects exist")
	}
	if !strings.Contains(html, "selector") {
		t.Error("should render selector div")
	}
	if !strings.Contains(html, "Project One") {
		t.Error("should contain Project One option")
	}
	if !strings.Contains(html, "Project Two") {
		t.Error("should contain Project Two option")
	}
	if !strings.Contains(html, "<option") {
		t.Error("should contain <option> elements")
	}
	if !strings.Contains(html, "Home") {
		t.Error("should contain page content")
	}
}

// TestSSRConditionalReturnInComponent tests that a component with
// if (condition) return <A/>; return <B/>; works in SSR.
func TestSSRConditionalReturnInComponent(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	os.WriteFile(dir+"/app/page.jsx", []byte(`
const { h } = window.Lungo;

function Greeting({ name }) {
  if (!name) return (<div class="no-name">Anonymous</div>);
  return (<div class="has-name">Hello {name}</div>);
}

export default function Page() {
  return (
    <div>
      <Greeting name="World" />
      <Greeting />
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

	if !strings.Contains(html, "has-name") {
		t.Error("should render has-name for Greeting with name")
	}
	if !strings.Contains(html, "Hello World") {
		t.Error("should contain Hello World")
	}
	if !strings.Contains(html, "no-name") {
		t.Error("should render no-name for Greeting without name")
	}
	if !strings.Contains(html, "Anonymous") {
		t.Error("should contain Anonymous")
	}
}
