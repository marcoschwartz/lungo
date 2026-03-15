package lungo

import (
	"strings"
	"testing"
)

func TestNestedTernaryWithArrow(t *testing.T) {
	src := `h("div", null, h("nav", null, "nav"), h("main", null, "M"))`
	result := jsEvalExpr(src, map[string]*jsValue{})
	html := RenderSSRHTML(result.vnode)
	if !strings.Contains(html, "<main>") {
		t.Errorf("missing <main> in: %s", html)
	}
}

func TestLayoutWithHooks(t *testing.T) {
	// Simulates the actual layout function body with hooks
	src := `
  const [menuOpen, setMenuOpen] = useState(false);
  const [dark, setDark] = useState(() => getInitialTheme() === "dark");
  const router = useRouter();

  useEffect(() => {
    if (dark) {
      document.documentElement.classList.add("dark");
      localStorage.setItem("theme", "dark");
    } else {
      document.documentElement.classList.remove("dark");
      localStorage.setItem("theme", "light");
    }
  }, [dark]);

  useEffect(() => {
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const handler = (e) => {
      if (!localStorage.getItem("theme")) {
        setDark(e.matches);
      }
    };
    mq.addEventListener("change", handler);
    return () => mq.removeEventListener("change", handler);
  }, []);

  useEffect(() => {
    setMenuOpen(false);
  }, [router.pathname]);

  return (
    h("div", {class: "wrapper"},
      h("nav", null, 
        h("a", {href: "/"}, "Brand"),
        h("button", {onclick: () => setDark(!dark)}, "theme"),
        h("button", {onclick: () => setMenuOpen(!menuOpen)}, 
          menuOpen ? (h("span", null, "X")) : (h("span", null, "M"))
        ),
        menuOpen ? (
          h("div", null, h("a", {href: "/", onclick: () => setMenuOpen(false)}, "Home"))
        ) : null
      ),
      h("main", null, children),
      h("footer", null, "Footer")
    )
  );`
	scope := map[string]*jsValue{
		"useState":  &jsValue{typ: jsTypeFunc, str: "__hook_useState"},
		"useEffect": &jsValue{typ: jsTypeFunc, str: "__hook_useEffect"},
		"useRouter":  &jsValue{typ: jsTypeFunc, str: "__hook_useRouter"},
		"children":  jvStr("PAGE"),
		"getInitialTheme": &jsValue{typ: jsTypeFunc, str: "__noop"},
	}
	ev := newJSEval(src, scope)
	result := ev.evalStatements()
	if result.typ != jsTypeVNode {
		t.Fatalf("expected vnode, got type %d", result.typ)
	}
	html := RenderSSRHTML(result.vnode)
	t.Logf("HTML: %s", html)
	if !strings.Contains(html, "<main>") {
		t.Errorf("missing <main>")
	}
	if !strings.Contains(html, "<footer>") {
		t.Errorf("missing <footer>")
	}
	if !strings.Contains(html, "PAGE") {
		t.Errorf("missing PAGE content")
	}
}
