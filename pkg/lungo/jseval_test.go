package lungo

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEvalHCall(t *testing.T) {
	scope := make(map[string]*jsValue)
	result := jsEvalExpr(`h("div", {class: "foo"}, "hello")`, scope)
	if result.typ != jsTypeVNode {
		t.Fatalf("expected vnode, got %d", result.typ)
	}
	html := RenderSSRHTML(result.vnode)
	if !strings.Contains(html, `class="foo"`) {
		t.Errorf("expected class attr, got: %s", html)
	}
	if !strings.Contains(html, "hello") {
		t.Errorf("expected text content, got: %s", html)
	}
	if !strings.HasPrefix(html, "<div") {
		t.Errorf("expected div tag, got: %s", html)
	}
}

func TestEvalNested(t *testing.T) {
	scope := make(map[string]*jsValue)
	result := jsEvalExpr(`h("div", null, h("h1", null, "Title"), h("p", null, "Text"))`, scope)
	html := RenderSSRHTML(result.vnode)
	if !strings.Contains(html, "<h1>Title</h1>") {
		t.Errorf("expected h1, got: %s", html)
	}
	if !strings.Contains(html, "<p>Text</p>") {
		t.Errorf("expected p, got: %s", html)
	}
}

func TestEvalPropertyAccess(t *testing.T) {
	scope := map[string]*jsValue{
		"data": jvObj(map[string]*jsValue{
			"message": jvStr("Hello World"),
		}),
	}
	result := jsEvalExpr(`h("p", null, data.message)`, scope)
	html := RenderSSRHTML(result.vnode)
	if !strings.Contains(html, "Hello World") {
		t.Errorf("expected Hello World, got: %s", html)
	}
}

func TestEvalStringConcat(t *testing.T) {
	scope := map[string]*jsValue{
		"post": jvObj(map[string]*jsValue{
			"slug": jvStr("my-post"),
		}),
	}
	result := jsEvalExpr(`h("a", {href: "/blog/" + post.slug}, "Link")`, scope)
	html := RenderSSRHTML(result.vnode)
	if !strings.Contains(html, `href="/blog/my-post"`) {
		t.Errorf("expected href, got: %s", html)
	}
}

func TestEvalTernary(t *testing.T) {
	scope := map[string]*jsValue{
		"show": jvTrue,
	}
	result := jsEvalExpr(`show ? h("p", null, "visible") : null`, scope)
	if result.typ != jsTypeVNode {
		t.Fatalf("expected vnode, got %d", result.typ)
	}
	html := RenderSSRHTML(result.vnode)
	if !strings.Contains(html, "visible") {
		t.Errorf("expected visible, got: %s", html)
	}

	scope["show"] = jvFalse
	result = jsEvalExpr(`show ? h("p", null, "visible") : null`, scope)
	if result.typ != jsTypeNull {
		t.Errorf("expected null, got %d", result.typ)
	}
}

func TestEvalArrayMap(t *testing.T) {
	scope := map[string]*jsValue{
		"posts": jvArr([]*jsValue{
			jvObj(map[string]*jsValue{"title": jvStr("Post 1")}),
			jvObj(map[string]*jsValue{"title": jvStr("Post 2")}),
		}),
	}
	result := jsEvalExpr(`h("div", null, posts.map(post => h("p", null, post.title)))`, scope)
	html := RenderSSRHTML(result.vnode)
	if !strings.Contains(html, "<p>Post 1</p>") {
		t.Errorf("expected Post 1, got: %s", html)
	}
	if !strings.Contains(html, "<p>Post 2</p>") {
		t.Errorf("expected Post 2, got: %s", html)
	}
}

func TestEvalArrayIsArray(t *testing.T) {
	scope := map[string]*jsValue{
		"data": jvArr([]*jsValue{jvStr("a"), jvStr("b")}),
	}
	result := jsEvalExpr(`Array.isArray(data) ? data : []`, scope)
	if result.typ != jsTypeArray || len(result.array) != 2 {
		t.Errorf("expected array with 2 items, got %d items", len(result.array))
	}
}

func TestEvalVoidElements(t *testing.T) {
	scope := make(map[string]*jsValue)
	result := jsEvalExpr(`h("img", {src: "/photo.jpg"})`, scope)
	html := RenderSSRHTML(result.vnode)
	if !strings.Contains(html, `<img src="/photo.jpg" />`) {
		t.Errorf("expected self-closing img, got: %s", html)
	}
}

func TestEvalLogicalAnd(t *testing.T) {
	scope := map[string]*jsValue{
		"data": jvObj(map[string]*jsValue{
			"message": jvStr("hi"),
		}),
	}
	result := jsEvalExpr(`data.message && h("p", null, data.message)`, scope)
	if result.typ != jsTypeVNode {
		t.Fatalf("expected vnode, got %d", result.typ)
	}
	html := RenderSSRHTML(result.vnode)
	if !strings.Contains(html, "hi") {
		t.Errorf("expected hi, got: %s", html)
	}
}

func TestEvalStatements(t *testing.T) {
	scope := map[string]*jsValue{
		"data": jvArr([]*jsValue{
			jvObj(map[string]*jsValue{"title": jvStr("A")}),
		}),
	}
	src := `const posts = Array.isArray(data) ? data : [];
return h("div", null, posts.map(post => h("p", null, post.title)));`
	ev := newJSEval(src, scope)
	result := ev.evalStatements()
	if result.typ != jsTypeVNode {
		t.Fatalf("expected vnode, got %d", result.typ)
	}
	html := RenderSSRHTML(result.vnode)
	if !strings.Contains(html, "<p>A</p>") {
		t.Errorf("expected <p>A</p>, got: %s", html)
	}
}

func TestEvalFullBlogPage(t *testing.T) {
	source := `const { h } = window.Lungo;
export const metadata = { title: "Blog" };
export const loader = { url: "/api/blog" };

export default function BlogPage({ data }) {
  const posts = Array.isArray(data) ? data : [];
  return (
    h("div", null,
      h("h1", {class: "text-4xl"}, "Blog"),
      h("div", {class: "flex"},
        posts.map(post => (
          h("a", {href: "/blog/" + post.slug, class: "block"},
            h("h2", null, post.title),
            h("p", null, "By ", post.author)
          )
        ))
      )
    )
  );
}
`
	transpiled := TranspileJSX(source) // no-op since no JSX, already h() calls
	_ = transpiled

	loaderData := json.RawMessage(`[
		{"slug":"hello","title":"Hello World","author":"Marco"},
		{"slug":"bye","title":"Goodbye","author":"Alice"}
	]`)

	body, params, err := extractDefaultExport(source)
	if err != nil {
		t.Fatalf("extractDefaultExport: %v", err)
	}
	_ = params

	scope := map[string]*jsValue{
		"data": jsonToJSValue(loaderData),
	}

	ev := newJSEval(body, scope)
	result := ev.evalStatements()
	if result.typ != jsTypeVNode {
		t.Fatalf("expected vnode, got %d", result.typ)
	}
	html := RenderSSRHTML(result.vnode)

	checks := []string{
		"<h1",
		"Blog",
		`href="/blog/hello"`,
		"Hello World",
		"By Marco",
		`href="/blog/bye"`,
		"Goodbye",
		"By Alice",
	}
	for _, check := range checks {
		if !strings.Contains(html, check) {
			t.Errorf("missing %q in HTML:\n%s", check, html)
		}
	}
}

func TestEvalComponentFunction(t *testing.T) {
	source := `function PostCard({ post }) {
  return h("div", {class: "card"}, h("h3", null, post.title));
}

export default function Page({ data }) {
  const posts = Array.isArray(data) ? data : [];
  return h("div", null, posts.map(post => h(PostCard, {post: post})));
}
`
	loaderData := json.RawMessage(`[{"title":"Test Post"}]`)

	body, _, err := extractDefaultExport(source)
	if err != nil {
		t.Fatalf("extractDefaultExport: %v", err)
	}

	localFuncs := extractFunctions(source)
	scope := map[string]*jsValue{
		"data": jsonToJSValue(loaderData),
	}
	for name, fn := range localFuncs {
		scope[name] = fn
	}

	ev := newJSEval(body, scope)
	result := ev.evalStatements()
	if result.typ != jsTypeVNode {
		t.Fatalf("expected vnode, got %d", result.typ)
	}
	html := RenderSSRHTML(result.vnode)
	if !strings.Contains(html, "Test Post") {
		t.Errorf("expected Test Post in HTML:\n%s", html)
	}
	if !strings.Contains(html, `class="card"`) {
		t.Errorf("expected card class in HTML:\n%s", html)
	}
}
