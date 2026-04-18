package lungo

import (
	"strings"
	"testing"
)

func TestParseImports_Named(t *testing.T) {
	src := `import { Hero, FeatureGrid as FG } from "/app/blocks.js";
import "/app/side.js";
import { Solo } from "./other";

export default function Page() { return h("div", null); }
`
	specs, cleaned := parseImports(src)
	if len(specs) != 3 {
		t.Fatalf("want 3 specs, got %d: %+v", len(specs), specs)
	}

	// All original import lines must be gone.
	if strings.Contains(cleaned, "import") {
		t.Errorf("cleaned source still contains 'import':\n%s", cleaned)
	}

	// First spec: two names, one aliased.
	if got := specs[0].names["Hero"]; got != "Hero" {
		t.Errorf("Hero mapping: want Hero, got %q", got)
	}
	if got := specs[0].names["FG"]; got != "FeatureGrid" {
		t.Errorf("aliased FG: want FeatureGrid, got %q", got)
	}
	if specs[0].path != "/app/blocks.js" {
		t.Errorf("path: got %q", specs[0].path)
	}

	// Find the side-effect import (order doesn't need to match source order).
	var sideEffect *importSpec
	for i := range specs {
		if len(specs[i].names) == 0 {
			sideEffect = &specs[i]
			break
		}
	}
	if sideEffect == nil {
		t.Fatal("side-effect import missing")
	}
	if sideEffect.path != "/app/side.js" {
		t.Errorf("side-effect path: got %q", sideEffect.path)
	}
}

func TestStripExportKeyword(t *testing.T) {
	src := `export function Hero(props) { return h("div"); }
export const registry = { Hero };
export default function Page() { return h("div"); }
`
	got := stripExportKeyword(src)
	if strings.Contains(got, "export function Hero") {
		t.Errorf("export function not stripped:\n%s", got)
	}
	if strings.Contains(got, "export const registry") {
		t.Errorf("export const not stripped:\n%s", got)
	}
	if !strings.Contains(got, "export default function Page") {
		t.Errorf("export default should be preserved for ExtractDefaultExport:\n%s", got)
	}
}

func TestResolveImportPath(t *testing.T) {
	cases := []struct {
		spec, importer, want string
	}{
		{"/app/blocks.js", "page.jsx", "blocks.js"},
		{"/app/lib/ui.js", "about/page.jsx", "lib/ui.js"},
		{"./blocks", "admin/page.jsx", "admin/blocks"},
		{"../blocks", "admin/page.jsx", "blocks"},
		{"blocks", "page.jsx", "blocks"},
	}
	for _, c := range cases {
		if got := resolveImportPath(c.spec, c.importer); got != c.want {
			t.Errorf("resolveImportPath(%q, %q) = %q, want %q", c.spec, c.importer, got, c.want)
		}
	}
}
