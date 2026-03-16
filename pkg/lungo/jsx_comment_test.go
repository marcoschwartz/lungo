package lungo

import (
	"strings"
	"testing"
)

func TestJSXCommentStripped(t *testing.T) {
	input := `function App() { return (<div>{/* comment */}<p>hello</p></div>); }`
	out := TranspileJSX(input)
	t.Logf("Output: %s", out)
	if strings.Contains(out, "comment") {
		t.Errorf("JSX comment should be stripped, got: %s", out)
	}
	if !strings.Contains(out, `h("p"`) {
		t.Errorf("child element should remain, got: %s", out)
	}
}

func TestJSXCommentBetweenElements(t *testing.T) {
	input := `function App() { return (<div><p>a</p>{/* separator */}<p>b</p></div>); }`
	out := TranspileJSX(input)
	t.Logf("Output: %s", out)
	if strings.Contains(out, "separator") {
		t.Errorf("JSX comment should be stripped, got: %s", out)
	}
	// Should have two <p> children without stray commas
	count := strings.Count(out, `h("p"`)
	if count != 2 {
		t.Errorf("expected 2 <p> elements, got %d in: %s", count, out)
	}
}

func TestJSXCommentMultiple(t *testing.T) {
	input := `function App() { return (<div>{/* one */}{/* two */}<span>ok</span></div>); }`
	out := TranspileJSX(input)
	t.Logf("Output: %s", out)
	if strings.Contains(out, "one") || strings.Contains(out, "two") {
		t.Errorf("JSX comments should be stripped, got: %s", out)
	}
	if !strings.Contains(out, `h("span"`) {
		t.Errorf("child element should remain, got: %s", out)
	}
}

func TestJSXCommentOnlyChild(t *testing.T) {
	input := `function App() { return (<div>{/* only comment */}</div>); }`
	out := TranspileJSX(input)
	t.Logf("Output: %s", out)
	if strings.Contains(out, "only comment") {
		t.Errorf("JSX comment should be stripped, got: %s", out)
	}
	// Should produce h("div", null) with no children
	if strings.Contains(out, ", ,") {
		t.Errorf("should not have empty comma slots, got: %s", out)
	}
}

func TestPreTagPreservesWhitespace(t *testing.T) {
	input := `function App() { return (<pre>line one
line two
line three</pre>); }`
	out := TranspileJSX(input)
	t.Logf("Output: %s", out)
	if !strings.Contains(out, `\n`) {
		t.Errorf("pre tag should preserve newlines, got: %s", out)
	}
	if strings.Contains(out, "line one line two") {
		t.Errorf("pre tag should not collapse whitespace, got: %s", out)
	}
}

func TestPreTagWithChildElements(t *testing.T) {
	input := `function App() { return (<pre><code><span>$</span> mkdir app
<span>$</span> go run .</code></pre>); }`
	out := TranspileJSX(input)
	t.Logf("Output: %s", out)
	// The text between </span> and <span> should contain a newline, not collapsed
	if !strings.Contains(out, `\n`) {
		t.Errorf("pre tag should preserve newlines between child elements, got: %s", out)
	}
}

func TestPreTagVsDiv(t *testing.T) {
	// In a div, newlines should collapse
	divInput := `function App() { return (<div>hello
world</div>); }`
	divOut := TranspileJSX(divInput)
	t.Logf("Div output: %s", divOut)
	if strings.Contains(divOut, `\n`) {
		t.Errorf("div should collapse newlines, got: %s", divOut)
	}

	// In a pre, newlines should be preserved
	preInput := `function App() { return (<pre>hello
world</pre>); }`
	preOut := TranspileJSX(preInput)
	t.Logf("Pre output: %s", preOut)
	if !strings.Contains(preOut, `\n`) {
		t.Errorf("pre should preserve newlines, got: %s", preOut)
	}
}

func TestTextareaPreservesWhitespace(t *testing.T) {
	input := `function App() { return (<textarea>line one
line two</textarea>); }`
	out := TranspileJSX(input)
	t.Logf("Output: %s", out)
	if !strings.Contains(out, `\n`) {
		t.Errorf("textarea should preserve newlines, got: %s", out)
	}
}

func TestIsBlockComment(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"/* comment */", true},
		{"/* a */ /* b */", true},
		{"  /* spaced */  ", true},
		{"not a comment", false},
		{"/* partial", false},
		{"code /* with comment */", false},
		{"", true},
	}
	for _, tt := range tests {
		got := isBlockComment(tt.input)
		if got != tt.expected {
			t.Errorf("isBlockComment(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}
