package lungo

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestBenchmarkPageCompiles(t *testing.T) {
	source := `const { h } = window.Lungo;
export const loader = { url: "/api/posts" };
export default function Home({ data }) {
  const posts = Array.isArray(data) ? data : [];
  return (
    <div>
      <h1>Blog</h1>
      <div>
        {posts.map(post => (
          <div>
            <h2>{post.title}</h2>
            <p>{post.excerpt}</p>
            <span>By {post.author}</span>
          </div>
        ))}
      </div>
    </div>
  );
}`
	transpiled := TranspileJSX(source)
	body, _, err := extractDefaultExport(transpiled)
	if err != nil {
		t.Fatal(err)
	}
	tokens := jsTokenize(body)
	compiled := compilePageTokens(tokens, nil)
	if compiled == nil {
		t.Fatal("compilation returned nil")
	}
	if compiled.ReturnNode != nil {
		t.Log("ReturnNode is SET — direct rendering path active")
	} else if compiled.ReturnExpr != nil {
		t.Log("ReturnNode is nil — using closure path (slower)")
	} else {
		t.Fatal("both ReturnNode and ReturnExpr are nil")
	}

	// Test rendering
	loaderData := json.RawMessage(`[{"title":"Post 1","excerpt":"Exc 1","author":"Marco"}]`)
	scope := map[string]*jsValue{"data": jsonToJSValue(loaderData)}
	stubHooks(&jsEval{scope: scope})
	
	html := compiled.renderHTML(scope)
	fmt.Println("HTML length:", len(html))
	if len(html) == 0 {
		t.Fatal("empty HTML")
	}
	preview := html
	if len(preview) > 200 { preview = preview[:200] }
	t.Log("HTML:", preview)
}
