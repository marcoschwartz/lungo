package lungo

import (
	"os"
	"strings"
	"testing"
)

func TestSSRSelectElement(t *testing.T) {
	source := `
const { h } = window.Lungo;

export default function Page() {
  return (
    <div>
      <select name="color" class="my-select">
        <option value="red">Red</option>
        <option value="blue" selected>Blue</option>
        <option value="green">Green</option>
      </select>
    </div>
  );
}
`
	html := testSSRPageSource(t, source)
	t.Logf("HTML: %s", html)

	if !strings.Contains(html, "<select") {
		t.Error("should contain <select> element")
	}
	if !strings.Contains(html, "<option") {
		t.Error("should contain <option> elements")
	}
	if !strings.Contains(html, "Red") {
		t.Error("should contain Red")
	}
	if !strings.Contains(html, "Blue") {
		t.Error("should contain Blue")
	}
}

func TestSSRSelectWithDynamicOptions(t *testing.T) {
	source := `
const { h } = window.Lungo;

export default function Page() {
  const items = ["Apple", "Banana", "Cherry"];
  return (
    <div>
      <select name="fruit">
        {items.map(item => (
          <option value={item}>{item}</option>
        ))}
      </select>
    </div>
  );
}
`
	html := testSSRPageSource(t, source)
	t.Logf("HTML: %s", html)

	if !strings.Contains(html, "<select") {
		t.Error("should contain <select>")
	}
	if !strings.Contains(html, "Apple") {
		t.Error("should contain Apple")
	}
	if !strings.Contains(html, "Cherry") {
		t.Error("should contain Cherry")
	}
}

func TestSSRSelectInComponent(t *testing.T) {
	source := `
const { h } = window.Lungo;

function MySelect({ items, current }) {
  return (
    <div class="wrapper">
      <select name="pick">
        {items ? items.map(i => (
          <option value={i.id} selected={String(i.id) === String(current)}>
            {i.name}
          </option>
        )) : null}
      </select>
    </div>
  );
}

export default function Page() {
  const data = [
    {id: 1, name: "Project A"},
    {id: 2, name: "Project B"}
  ];
  return (
    <div>
      <MySelect items={data} current="2" />
    </div>
  );
}
`
	html := testSSRPageSource(t, source)
	t.Logf("HTML: %s", html)

	if !strings.Contains(html, "<select") {
		t.Error("should contain <select>")
	}
	if !strings.Contains(html, "Project A") {
		t.Error("should contain Project A")
	}
	if !strings.Contains(html, "Project B") {
		t.Error("should contain Project B")
	}
}

// helper — creates a temp app and runs SSR
func testSSRPageSource(t *testing.T, source string) string {
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
	if html == "" {
		t.Fatal("SSR returned empty HTML")
	}
	return html
}
