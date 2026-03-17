package lungo

import (
	"os"
	"strings"
	"testing"
)

func TestSSR_DangerouslySetInnerHTML_Literal(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	os.WriteFile(dir+"/app/page.jsx", []byte(`
const { h } = window.Lungo;
export default function Page() {
  return (<div><div dangerouslySetInnerHTML="<p>Hello <b>World</b></p>"></div></div>);
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})
	html, _, err := app.evaluatePageSSR("page.js", nil, nil)
	if err != nil {
		t.Fatalf("SSR error: %v", err)
	}
	t.Logf("HTML: %s", html)

	if !strings.Contains(html, "<p>Hello <b>World</b></p>") {
		t.Error("should render raw HTML from dangerouslySetInnerHTML literal")
	}
}

func TestSSR_DangerouslySetInnerHTML_Variable(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	os.WriteFile(dir+"/app/page.jsx", []byte(`
const { h } = window.Lungo;
export default function Page() {
  const content = "<p>Dynamic <em>content</em></p>";
  return (<div dangerouslySetInnerHTML={content}></div>);
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})
	html, _, err := app.evaluatePageSSR("page.js", nil, nil)
	if err != nil {
		t.Fatalf("SSR error: %v", err)
	}
	t.Logf("HTML: %s", html)

	if !strings.Contains(html, "<p>Dynamic <em>content</em></p>") {
		t.Error("should render raw HTML from variable")
	}
}

func TestSSR_DangerouslySetInnerHTML_FromData(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	os.WriteFile(dir+"/app/page.jsx", []byte(`
const { h } = window.Lungo;
export default function Page({ data }) {
  return (<div dangerouslySetInnerHTML={data.body}></div>);
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})
	loaderData := []byte(`{"body":"<h2>Title</h2><p>Body text</p>"}`)
	html, _, err := app.evaluatePageSSR("page.js", loaderData, nil)
	if err != nil {
		t.Fatalf("SSR error: %v", err)
	}
	t.Logf("HTML: %s", html)

	if !strings.Contains(html, "<h2>Title</h2><p>Body text</p>") {
		t.Error("should render raw HTML from loader data")
	}
}

func TestSSR_DangerouslySetInnerHTML_NestedObjectAccess(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	// This matches the blog template pattern:
	// data comes as {content: {data: {introduction: "<p>..."}}}
	os.WriteFile(dir+"/app/page.jsx", []byte(`
const { h } = window.Lungo;
export default function Page({ data }) {
  const post = data && data.content ? data.content : data;
  const d = post && post.data ? post.data : post;
  const introduction = d.introduction || "";
  return (
    <div>
      <h1>{d.title}</h1>
      <div class="content" dangerouslySetInnerHTML={introduction}></div>
    </div>
  );
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})
	loaderData := []byte(`{"content":{"data":{"title":"Test Post","introduction":"<p>Hello <a href=\"/\">link</a></p>"}}}`)
	html, _, err := app.evaluatePageSSR("page.js", loaderData, nil)
	if err != nil {
		t.Fatalf("SSR error: %v", err)
	}
	t.Logf("HTML: %s", html)

	if !strings.Contains(html, "Test Post") {
		t.Error("should contain title")
	}
	if !strings.Contains(html, `<p>Hello <a href="/">link</a></p>`) {
		t.Error("should render raw HTML from nested data access")
	}
}

func TestSSR_DangerouslySetInnerHTML_InComponent(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)

	os.WriteFile(dir+"/app/page.jsx", []byte(`
const { h } = window.Lungo;

function HtmlBlock({ html }) {
  return (<div class="block" dangerouslySetInnerHTML={html}></div>);
}

export default function Page() {
  const text = "<p>From <strong>component</strong></p>";
  return (<div><HtmlBlock html={text} /></div>);
}
`), 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})
	html, _, err := app.evaluatePageSSR("page.js", nil, nil)
	if err != nil {
		t.Fatalf("SSR error: %v", err)
	}
	t.Logf("HTML: %s", html)

	if !strings.Contains(html, "<p>From <strong>component</strong></p>") {
		t.Error("should render raw HTML passed through component prop")
	}
}
