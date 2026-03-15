package lungo

import (
	"strings"
	"testing"
)

func TestTranspileTernaryWithNull(t *testing.T) {
	input := `function App() { return (<div>{show ? (<p>yes</p>) : null}</div>); }`
	out := TranspileJSX(input)
	t.Logf("Output: %s", out)
	if !strings.Contains(out, ": null") {
		t.Errorf("missing ': null' in output: %s", out)
	}
}

func TestTranspileTernaryWithNullMulti(t *testing.T) {
	input := `function App() { return (<div>{a ? (<p>A</p>) : null}{b ? (<span>B</span>) : null}</div>); }`
	out := TranspileJSX(input)
	t.Logf("Output: %s", out)
	count := strings.Count(out, ": null")
	if count != 2 {
		t.Errorf("expected 2 ': null', got %d in: %s", count, out)
	}
}
