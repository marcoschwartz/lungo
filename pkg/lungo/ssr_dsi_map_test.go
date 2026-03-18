package lungo

import (
	"os"
	"strings"
	"testing"
)

func TestSSR_DangerouslySetInnerHTML_InMapCallback(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	os.WriteFile(dir+"/app/page.jsx", []byte(`
const { h } = window.Lungo;
export default function Page({ data }) {
  const sections = data.sections || [];
  return (
    <div>
      {sections.map(s => (
        <div>
          <h2>{s.title}</h2>
          {s.content ? <div class="content" dangerouslySetInnerHTML={s.content}></div> : null}
        </div>
      ))}
    </div>
  );
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})
	loaderData := []byte(`{"sections":[
		{"title":"Eagle","content":"The de facto software for designing"},
		{"title":"KiCad","content":"A completely open-source solution"}
	]}`)

	html, _, err := app.evaluatePageSSR("page.js", loaderData, nil)
	if err != nil {
		t.Fatalf("SSR error: %v", err)
	}
	t.Logf("HTML: %s", html)

	if !strings.Contains(html, "de facto software") {
		t.Error("should contain Eagle content via dangerouslySetInnerHTML in map")
	}
	if !strings.Contains(html, "open-source solution") {
		t.Error("should contain KiCad content via dangerouslySetInnerHTML in map")
	}
	if strings.Contains(html, `class="content"></div>`) {
		t.Error("content div should NOT be empty")
	}
}
