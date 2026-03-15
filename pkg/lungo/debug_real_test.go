package lungo

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestRealTranspiledLayout(t *testing.T) {
	data, err := os.ReadFile("../../_example/app/layout.jsx")
	if err != nil {
		t.Skip("layout not found")
	}
	transpiled := TranspileJSX(string(data))
	body, _, err := extractDefaultExport(transpiled)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	tokens := jsTokenize(body)

	// Simulate skipExpr starting at token 522 (the ( after menuOpen ?)
	depth := 0
	for i := 522; i < len(tokens); i++ {
		tk := tokens[i]
		switch tk.t {
		case tokLParen, tokLBrack, tokLBrace:
			depth++
		case tokRParen, tokRBrack, tokRBrace:
			if depth == 0 {
				fmt.Printf("skipExpr would stop at token %d (type=%d val=%q) - depth=0 closer\n", i, tk.t, tk.v)
				goto done
			}
			depth--
		case tokComma:
			if depth == 0 {
				fmt.Printf("skipExpr would stop at token %d (comma at depth 0)\n", i)
				goto done
			}
		case tokColon:
			if depth == 0 {
				fmt.Printf("skipExpr would stop at token %d (colon at depth 0)\n", i)
				goto done
			}
		case tokSemi:
			if depth == 0 {
				fmt.Printf("skipExpr would stop at token %d (semi at depth 0)\n", i)
				goto done
			}
		}
	}
	fmt.Println("skipExpr reached end of tokens")
done:
	
	// Check what token 522 is
	fmt.Printf("Token 522: type=%d val=%q\n", tokens[522].t, tokens[522].v)

	scope := make(map[string]*jsValue)
	scope["useState"] = &jsValue{typ: jsTypeFunc, str: "__hook_useState"}
	scope["useEffect"] = &jsValue{typ: jsTypeFunc, str: "__hook_useEffect"}
	scope["useRouter"] = &jsValue{typ: jsTypeFunc, str: "__hook_useRouter"}
	scope["children"] = jvNode(&ssrNode{Tag: "lungo-children"})
	localFuncs := extractFunctions(transpiled)
	for name, fn := range localFuncs {
		scope[name] = fn
	}
	ev := newJSEval(body, scope)
	result := ev.evalStatements()
	html := RenderSSRHTML(result.vnode)
	if !strings.Contains(html, "<main") {
		t.Errorf("missing <main>")
	}
}
