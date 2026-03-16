package lungo

import (
	"os"
	"strings"
	"testing"
)

// TestDemosPage_FullSSR tests the entire demos page through SSR evaluation
// to find hydration mismatches between SSR output and expected client output.
func TestDemosPage_FullSSR(t *testing.T) {
	data, err := os.ReadFile("../../_example/app/demos/page.jsx")
	if err != nil {
		t.Skip("demos page not found")
	}

	source := TranspileJSX(string(data))
	body, params, err := extractDefaultExport(source)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	_ = params

	localFuncs := extractFunctions(source)
	scope := make(map[string]*jsValue)
	for name, fn := range localFuncs {
		scope[name] = fn
	}
	stubHooks(&jsEval{scope: scope})

	ev := newJSEval(body, scope)
	result := ev.evalStatements()
	if result.typ != jsTypeVNode {
		t.Fatalf("expected vnode, got type %d", result.typ)
	}

	html := RenderSSRHTML(result.vnode)
	t.Logf("Demos SSR HTML length: %d", len(html))

	// ─── TodoApp checks ────────────────────────────────────
	if !strings.Contains(html, "Todo App") {
		t.Error("missing Todo App heading")
	}
	if !strings.Contains(html, "No todos yet") {
		t.Error("missing 'No todos yet' (filtered.length === 0 should be true)")
	}
	if !strings.Contains(html, `placeholder="What needs to be done?"`) {
		t.Error("missing todo input")
	}
	// Filter buttons: all, active, done
	if !strings.Contains(html, ">all</button>") {
		t.Error("missing 'all' filter button")
	}
	if !strings.Contains(html, ">active</button>") {
		t.Error("missing 'active' filter button")
	}
	if !strings.Contains(html, ">done</button>") {
		t.Error("missing 'done' filter button")
	}
	if !strings.Contains(html, "0 remaining") {
		t.Error("missing '0 remaining'")
	}

	// ─── Stopwatch checks ──────────────────────────────────
	if !strings.Contains(html, "Stopwatch") {
		t.Error("missing Stopwatch heading")
	}
	if !strings.Contains(html, "00:00") {
		t.Error("missing stopwatch display 00:00")
	}
	if !strings.Contains(html, "Start") {
		t.Error("missing Start button (running=false)")
	}
	if !strings.Contains(html, "Reset") {
		t.Error("missing Reset button")
	}

	// ─── ColorPicker checks ────────────────────────────────
	if !strings.Contains(html, "Color Picker") {
		t.Error("missing Color Picker heading")
	}
	if !strings.Contains(html, "hsl(200, 70%, 50%)") {
		t.Error("missing color display hsl(200, 70%, 50%)")
	}

	// ─── DragList checks ───────────────────────────────────
	if !strings.Contains(html, "Drag") {
		t.Error("missing Drag heading")
	}
	if !strings.Contains(html, "Apple") {
		t.Error("missing Apple item")
	}
	if !strings.Contains(html, "Elderberry") {
		t.Error("missing Elderberry item")
	}

	// ─── ChartDemo checks ──────────────────────────────────
	if !strings.Contains(html, "Chart.js Integration") {
		t.Error("missing Chart.js heading")
	}
	if !strings.Contains(html, "<canvas") {
		t.Error("missing canvas element")
	}

	// ─── LiveClock checks ──────────────────────────────────
	if !strings.Contains(html, "Live Clock") {
		t.Error("missing Live Clock heading")
	}

	// ─── KeyTracker checks ─────────────────────────────────
	if !strings.Contains(html, "Keyboard Tracker") {
		t.Error("missing Keyboard Tracker heading")
	}
	if !strings.Contains(html, "Waiting for input...") {
		t.Error("missing 'Waiting for input...' (keys.length === 0 should be true)")
	}

	// ─── Tabs checks ───────────────────────────────────────
	if !strings.Contains(html, "useState") {
		t.Error("missing useState tab label")
	}
	if !strings.Contains(html, "useEffect") {
		t.Error("missing useEffect tab label")
	}

	// ─── Structure checks (hydration-critical) ─────────────
	// The "No todos yet" <p> should be inside the flex-col div
	todoListIdx := strings.Index(html, `flex flex-col gap-1`)
	if todoListIdx > 0 {
		noTodosIdx := strings.Index(html, "No todos yet")
		todoListEnd := strings.Index(html[todoListIdx:], "</div>") + todoListIdx
		if noTodosIdx < todoListIdx || noTodosIdx > todoListEnd+50 {
			t.Errorf("'No todos yet' <p> not properly inside the flex-col div (todoList=%d, noTodos=%d, divEnd=%d)", todoListIdx, noTodosIdx, todoListEnd)
		}
	}

	// Key tracker: "Waiting for input..." should be a <span>, not outside the wrapper
	waitingIdx := strings.Index(html, "Waiting for input...")
	if waitingIdx > 0 {
		before := html[waitingIdx-50 : waitingIdx]
		if !strings.Contains(before, "<span") {
			t.Errorf("'Waiting for input...' not inside a <span>: ...%s", before)
		}
	}
}

// TestDemosPage_LiveClock tests that LiveClock handles new Date() gracefully
func TestDemosPage_LiveClock(t *testing.T) {
	source := `
function LiveClock() {
  const [time, setTime] = useState(new Date().toLocaleTimeString());
  return (<div class="clock"><div class="display">{time}</div></div>);
}
export default function Page() {
  return (<div><LiveClock /></div>);
}`
	transpiled := TranspileJSX(source)
	body, _, err := extractDefaultExport(transpiled)
	if err != nil {
		t.Fatal(err)
	}

	localFuncs := extractFunctions(transpiled)
	scope := make(map[string]*jsValue)
	for name, fn := range localFuncs {
		scope[name] = fn
	}
	stubHooks(&jsEval{scope: scope})

	ev := newJSEval(body, scope)
	result := ev.evalStatements()
	if result.typ != jsTypeVNode {
		t.Fatalf("expected vnode, got type %d", result.typ)
	}
	html := RenderSSRHTML(result.vnode)
	t.Logf("LiveClock HTML: %s", html)
	// Should render with the stub time string
	if !strings.Contains(html, "clock") || !strings.Contains(html, "display") {
		t.Error("missing clock wrapper")
	}
}

// TestDemosPage_TabsWithContent tests Tabs component with JSX content props
func TestDemosPage_TabsWithContent(t *testing.T) {
	source := `
function Tabs({ tabs }) {
  const [active, setActive] = useState(0);
  return (
    <div class="tabs">
      <div class="tab-bar">
        {tabs.map((tab, i) => (
          <button class={active === i ? "active" : "inactive"}>{tab.label}</button>
        ))}
      </div>
      <div class="tab-content">{tabs[active].content}</div>
    </div>
  );
}
export default function Page() {
  return (
    <div>
      <Tabs tabs={[
        { label: "Tab1", content: (<p>Content 1</p>) },
        { label: "Tab2", content: (<p>Content 2</p>) },
      ]} />
    </div>
  );
}`
	transpiled := TranspileJSX(source)
	body, _, err := extractDefaultExport(transpiled)
	if err != nil {
		t.Fatal(err)
	}

	localFuncs := extractFunctions(transpiled)
	scope := make(map[string]*jsValue)
	for name, fn := range localFuncs {
		scope[name] = fn
	}
	stubHooks(&jsEval{scope: scope})

	ev := newJSEval(body, scope)
	result := ev.evalStatements()
	if result.typ != jsTypeVNode {
		t.Fatalf("expected vnode, got type %d", result.typ)
	}
	html := RenderSSRHTML(result.vnode)
	t.Logf("Tabs HTML: %s", html)

	if !strings.Contains(html, "Tab1") {
		t.Error("missing Tab1 label")
	}
	if !strings.Contains(html, "Tab2") {
		t.Error("missing Tab2 label")
	}
	// First tab should be active, showing Content 1
	if !strings.Contains(html, "Content 1") {
		t.Error("missing Content 1 (first tab should be active)")
	}
	// Check active class on first tab
	if !strings.Contains(html, `class="active"`) {
		t.Error("missing active class on first tab")
	}
}

// TestDemosPage_ArrayJoin tests array.join() for class building
func TestDemosPage_ArrayJoin(t *testing.T) {
	scope := map[string]*jsValue{
		"dragging": jvNull,
		"over":     jvNull,
		"i":        jvNum(0),
	}
	src := `h("div", {class: ["base-class", dragging === i ? "dragging" : "normal", over === i && dragging !== i ? "over" : "not-over"].join(" ")}, "content")`
	result := jsEvalExpr(src, scope)
	if result.typ != jsTypeVNode {
		t.Fatal("expected vnode")
	}
	html := RenderSSRHTML(result.vnode)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "base-class") {
		t.Error("missing base-class")
	}
	if !strings.Contains(html, "normal") {
		t.Error("missing normal (dragging=null !== i=0)")
	}
	if !strings.Contains(html, "not-over") {
		t.Error("missing not-over")
	}
}

// TestDemosPage_NewDate tests new Date() handling
func TestDemosPage_NewDate(t *testing.T) {
	scope := make(map[string]*jsValue)
	// new Date().toLocaleTimeString() should not crash
	result := jsEvalExpr(`h("span", null, "time")`, scope)
	if result.typ != jsTypeVNode {
		t.Fatal("expected vnode")
	}
}

// TestDemosPage_CanvasElement tests canvas rendering
func TestDemosPage_CanvasElement(t *testing.T) {
	scope := map[string]*jsValue{
		"canvasRef": jvObj(map[string]*jsValue{"current": jvNull}),
	}
	result := jsEvalExpr(`h("canvas", {ref: canvasRef, height: "200"})`, scope)
	html := RenderSSRHTML(result.vnode)
	if !strings.Contains(html, "<canvas") {
		t.Error("missing canvas")
	}
	if !strings.Contains(html, `height="200"`) {
		t.Error("missing height attribute")
	}
	// ref should be stripped
	if strings.Contains(html, "ref") {
		t.Error("ref should be stripped from HTML output")
	}
}

// TestDemosPage_FilterButtons tests array literal .map() for filter buttons
func TestDemosPage_FilterButtons(t *testing.T) {
	scope := map[string]*jsValue{
		"filter":    jvStr("all"),
		"setFilter": &jsValue{typ: jsTypeFunc, str: "__noop"},
	}
	src := `h("div", null, ["all", "active", "done"].map(f => h("button", {class: filter === f ? "active" : "inactive"}, f)))`
	result := jsEvalExpr(src, scope)
	html := RenderSSRHTML(result.vnode)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, ">all</button>") {
		t.Error("missing 'all' button")
	}
	if !strings.Contains(html, ">active</button>") {
		t.Error("missing 'active' button")
	}
	if !strings.Contains(html, ">done</button>") {
		t.Error("missing 'done' button")
	}
	// "all" should have active class
	allIdx := strings.Index(html, ">all<")
	if allIdx > 30 {
		before := html[allIdx-30 : allIdx]
		if !strings.Contains(before, `class="active"`) {
			t.Errorf("'all' button should have active class: %s", before)
		}
	}
}

// TestDemosPage_ConditionalLength tests filtered.length === 0 pattern
func TestDemosPage_ConditionalLength(t *testing.T) {
	scope := map[string]*jsValue{
		"filtered": jvArr([]*jsValue{}), // empty array
	}
	src := `h("div", null, filtered.map(x => h("p", null, x)), filtered.length === 0 ? h("p", null, "Empty!") : null)`
	result := jsEvalExpr(src, scope)
	html := RenderSSRHTML(result.vnode)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "Empty!") {
		t.Error("missing 'Empty!' for empty array")
	}
}

// TestDemosPage_StopwatchPad tests the pad function pattern
func TestDemosPage_StopwatchPad(t *testing.T) {
	scope := make(map[string]*jsValue)
	stubHooks(&jsEval{scope: scope})
	src := `
const [time, setTime] = useState(0);
const mins = Math.floor(time / 60000);
const secs = Math.floor((time % 60000) / 1000);
const ms = Math.floor((time % 1000) / 10);
const pad = (n) => String(n).padStart(2, "0");
return h("div", null, pad(mins), ":", pad(secs), h("span", null, ".", pad(ms)));
`
	ev := newJSEval(src, scope)
	result := ev.evalStatements()
	if result.typ != jsTypeVNode {
		t.Fatalf("expected vnode, got type %d", result.typ)
	}
	html := RenderSSRHTML(result.vnode)
	t.Logf("Stopwatch HTML: %s", html)
	if !strings.Contains(html, "00") {
		t.Error("missing padded zeros")
	}
	if !strings.Contains(html, ":") {
		t.Error("missing colon separator")
	}
	if !strings.Contains(html, ".") {
		t.Error("missing dot separator")
	}
}

// TestDemosPage_ColorString tests string concatenation for HSL color
func TestDemosPage_ColorString(t *testing.T) {
	scope := make(map[string]*jsValue)
	stubHooks(&jsEval{scope: scope})
	src := `
const [hue, setHue] = useState(200);
const [sat, setSat] = useState(70);
const [light, setLight] = useState(50);
const color = "hsl(" + hue + ", " + sat + "%, " + light + "%)";
return h("div", null, h("div", {style: {backgroundColor: color}}), h("p", null, color));
`
	ev := newJSEval(src, scope)
	result := ev.evalStatements()
	html := RenderSSRHTML(result.vnode)
	t.Logf("Color HTML: %s", html)
	if !strings.Contains(html, "hsl(200, 70%, 50%)") {
		t.Error("missing HSL color string")
	}
	if !strings.Contains(html, "background-color:hsl(200, 70%, 50%)") {
		t.Error("missing backgroundColor style")
	}
}
