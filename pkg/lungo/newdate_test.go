package lungo

import (
	"testing"
)

func TestNestedMathMinMax(t *testing.T) {
	scope := map[string]*jsValue{
		"value": jvNum(0),
		"max":   jvNum(100),
	}
	result := jsEvalExpr(`Math.min(100, Math.max(0, (value / max) * 100))`, scope)
	t.Logf("result = type=%d num=%f", result.typ, result.num)
	if result.typ != jsTypeNumber || result.num != 0 {
		t.Errorf("expected 0, got type=%d num=%f", result.typ, result.num)
	}
}

func TestGaugeComponent(t *testing.T) {
	source := `
function Gauge({ label, value, unit, color, max = 100 }) {
  const pct = Math.min(100, Math.max(0, (value / max) * 100));
  return (
    <div>
      <span class="label">{label}</span>
      <span class={"val " + color}>{value}<span class="unit">{unit}</span></span>
      <div style={{ width: pct + "%" }} />
    </div>
  );
}
export default function Page() {
  return (<div><Gauge label="CPU" value={50} unit="%" color="blue" /></div>);
}`
	transpiled := TranspileJSX(source)
	body, _, err := extractDefaultExport(transpiled)
	if err != nil { t.Fatal(err) }
	localFuncs := extractFunctions(transpiled)
	scope := make(map[string]*jsValue)
	for name, fn := range localFuncs { scope[name] = fn }
	stubHooks(&jsEval{scope: scope})
	ev := newJSEval(body, scope)
	result := ev.evalStatements()
	if result.typ != jsTypeVNode { t.Fatalf("expected vnode, got %d", result.typ) }
	html := RenderSSRHTML(result.vnode)
	t.Logf("Gauge HTML: %s", html)
	if html == "" { t.Error("empty HTML") }
	if !contains(html, "CPU") { t.Error("missing CPU label") }
	if !contains(html, "50") { t.Error("missing value 50") }
	if !contains(html, "%") { t.Error("missing unit %") }
	if !contains(html, "width:50%") { t.Error("missing width style") }
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub { return i }
	}
	return -1
}
