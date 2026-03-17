package lungo

import (
	"github.com/marcoschwartz/lungo/pkg/espresso"
	"os"
	"strings"
	"testing"
)

func testSSRWithSource(t *testing.T, source string) string {
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

// Test: multiple helper functions + main export (like home page structure)
func TestSSRMultipleHelperFunctions(t *testing.T) {
	html := testSSRWithSource(t, `
const { h } = window.Lungo;

function StatCard({ value, label }) {
  return (
    <div class="stat">
      <div class="value">{value}</div>
      <div class="label">{label}</div>
    </div>
  );
}

function FeatureCard({ title, description }) {
  return (
    <div class="feature">
      <h3>{title}</h3>
      <p>{description}</p>
    </div>
  );
}

export default function HomePage() {
  return (
    <div>
      <StatCard value="156ms" label="Build Time" />
      <FeatureCard title="SSR" description="Server rendering" />
    </div>
  );
}
`)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "156ms") {
		t.Error("missing StatCard content")
	}
}

// Test: template literal in pre/code (like CodeBlock)
func TestSSRTemplateLiteralInPre(t *testing.T) {
	source := "const { h } = window.Lungo;\n\n" +
		"function CodeBlock() {\n" +
		"  return (\n" +
		"    <div class=\"code\">\n" +
		"      <pre><code>{`line one\nline two\nline three`}</code></pre>\n" +
		"    </div>\n" +
		"  );\n" +
		"}\n\n" +
		"export default function Page() {\n" +
		"  return (<div><CodeBlock /></div>);\n" +
		"}\n"

	html := testSSRWithSource(t, source)
	t.Logf("HTML: %s", html)
}

// Test: multiline template literal with backticks (the exact pattern in home page)
func TestSSRCodeBlockWithBackticks(t *testing.T) {
	source := "const { h } = window.Lungo;\n\n" +
		"export default function Page() {\n" +
		"  return (\n" +
		"    <div>\n" +
		"      <pre><code>{`import { useState } from 'react';\n\nfunction Counter() {\n  return <div>hello</div>;\n}`}</code></pre>\n" +
		"    </div>\n" +
		"  );\n" +
		"}\n"

	html := testSSRWithSource(t, source)
	t.Logf("HTML: %s", html)
}

// Test: the actual CodeBlock + TerminalBlock pattern from home page
func TestSSRHomePageCodeBlocks(t *testing.T) {
	source := `const { h } = window.Lungo;

function CodeBlock() {
  return (
    <div class="code-block">
      <div class="header">
        <span>app/page.jsx</span>
      </div>
      <pre><code>{"hello world"}</code></pre>
    </div>
  );
}

function TerminalBlock() {
  return (
    <div class="terminal">
      <pre><code>{"$ go run ."}</code></pre>
    </div>
  );
}

export default function HomePage() {
  return (
    <div>
      <CodeBlock />
      <TerminalBlock />
    </div>
  );
}
`

	html := testSSRWithSource(t, source)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "hello world") {
		t.Error("missing code block content")
	}
}

// Test: ComparisonRow with string concatenation
func TestSSRStringConcatenation(t *testing.T) {
	source := `const { h } = window.Lungo;

function Row({ label, value }) {
  return (
    <div class={"row " + label}>
      <span>{value}</span>
    </div>
  );
}

export default function Page() {
  return (
    <div>
      <Row label="first" value="one" />
      <Row label="second" value="two" />
    </div>
  );
}
`
	html := testSSRWithSource(t, source)
	t.Logf("HTML: %s", html)
}

// Test: the full home page from lungo-site (read actual file)
func TestSSRActualHomePage(t *testing.T) {
	siteAppDir := "../../../lungo-site/app"
	data, err := os.ReadFile(siteAppDir + "/page.jsx")
	if err != nil {
		t.Skip("lungo-site home page not found")
	}

	dir := t.TempDir()
	os.MkdirAll(dir+"/app", 0755)
	os.MkdirAll(dir+"/static", 0755)
	os.WriteFile(dir+"/app/page.jsx", data, 0644)

	app := New(Options{AppDir: dir + "/app", StaticDir: dir + "/static", Dev: true})

	// First, check that espresso.ExtractDefaultExport works
	source := TranspileJSX(string(data))
	funcBody, funcParams, err := espresso.ExtractDefaultExport(source)
	if err != nil {
		t.Fatalf("espresso.ExtractDefaultExport failed: %v", err)
	}
	t.Logf("funcParams: %s", funcParams)
	t.Logf("funcBody (first 300 chars): %s", truncate(funcBody, 300))

	// Check espresso.ExtractFunctions
	localFuncs := espresso.ExtractFunctions(source)
	t.Logf("Local functions found: %d", len(localFuncs))
	for name := range localFuncs {
		t.Logf("  - %s", name)
	}

	// Try SSR
	html, _, err := app.evaluatePageSSR("page.js", nil, nil)
	if err != nil {
		t.Logf("SSR error (expected): %v", err)
		// Don't fatal — we want to see what we learned above
	} else {
		t.Logf("SSR succeeded: %d bytes", len(html))
		t.Logf("HTML (first 500): %s", truncate(html, 500))
	}
}
