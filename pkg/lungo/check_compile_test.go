package lungo

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckAllPagesCompile(t *testing.T) {
	pages, _ := filepath.Glob("../../_example/app/**/page.jsx")
	pages2, _ := filepath.Glob("../../_example/app/page.jsx")
	pages = append(pages, pages2...)

	for _, p := range pages {
		data, err := os.ReadFile(p)
		if err != nil { continue }
		source := TranspileJSX(string(data))
		body, _, err := extractDefaultExport(source)
		if err != nil { 
			fmt.Printf("  SKIP (no export): %s\n", p)
			continue 
		}
		localFuncs := extractFunctions(source)
		tokens := jsTokenize(body)
		compiled := compilePageTokens(tokens, localFuncs)
		
		rel, _ := filepath.Rel("../../_example/app", p)
		if compiled != nil && compiled.ReturnNode != nil {
			fmt.Printf("  COMPILED (direct): %s\n", rel)
		} else if compiled != nil {
			fmt.Printf("  COMPILED (expr):   %s\n", rel)
		} else {
			fmt.Printf("  INTERPRETED:       %s (%d funcs)\n", rel, len(localFuncs))
		}
	}
}
