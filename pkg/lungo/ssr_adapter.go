package lungo

// SSR adapter: bridges espresso JS engine with Lungo's SSR rendering.
// Provides hook stubs, h() call handling, and vnode support.

import (
	"strings"

	"github.com/marcoschwartz/espresso"
)

// ── SSR Scope Setup ─────────────────────────────────────

// buildSSRScope creates a scope with SSR hook stubs and local functions.
func buildSSRScope(localFuncs map[string]*espresso.Value) map[string]*espresso.Value {
	scope := make(map[string]*espresso.Value, len(localFuncs)+10)
	for name, fn := range localFuncs {
		scope[name] = fn
	}
	stubHooksInScope(scope, "/")
	return scope
}

// stubHooksInScope adds SSR hook stub functions to a scope.
func stubHooksInScope(scope map[string]*espresso.Value, pathname string) {
	// useState → returns [initialValue, noop]
	scope["useState"] = espresso.NewNativeFunc(func(args []*espresso.Value) *espresso.Value {
		initial := espresso.Undefined
		if len(args) > 0 {
			initial = args[0]
			// Lazy initializer: useState(() => expr) — call the function
			if initial.Type() == espresso.TypeFunc {
				initial = espresso.CallFunc(scope, initial, map[string]*espresso.Value{})
			}
		}
		return espresso.NewArr([]*espresso.Value{
			initial,
			espresso.NewNativeFunc(func(args []*espresso.Value) *espresso.Value { return espresso.Undefined }), // noop setter
		})
	})

	// useEffect → no-op on server
	scope["useEffect"] = espresso.NewNativeFunc(func(args []*espresso.Value) *espresso.Value {
		return espresso.Undefined
	})

	// useRouter → returns { pathname, query, push, replace, refresh }
	scope["useRouter"] = espresso.NewNativeFunc(func(args []*espresso.Value) *espresso.Value {
		return espresso.NewObj(map[string]*espresso.Value{
			"pathname": espresso.NewStr(pathname),
			"query":    espresso.NewObj(map[string]*espresso.Value{}),
			"push":     espresso.NewNativeFunc(func(args []*espresso.Value) *espresso.Value { return espresso.Undefined }),
			"replace":  espresso.NewNativeFunc(func(args []*espresso.Value) *espresso.Value { return espresso.Undefined }),
			"refresh":  espresso.NewNativeFunc(func(args []*espresso.Value) *espresso.Value { return espresso.Undefined }),
		})
	})

	// useRef → returns { current: initialValue }
	scope["useRef"] = espresso.NewNativeFunc(func(args []*espresso.Value) *espresso.Value {
		initial := espresso.Null
		if len(args) > 0 {
			initial = args[0]
		}
		return espresso.NewObj(map[string]*espresso.Value{"current": initial})
	})

	// useMemo → evaluates the function immediately on server
	scope["useMemo"] = espresso.NewNativeFunc(func(args []*espresso.Value) *espresso.Value {
		if len(args) > 0 {
			fn := args[0]
			if fn.Type() == espresso.TypeFunc {
				// Call the memo function
				return espresso.CallFunc(scope, fn, map[string]*espresso.Value{})
			}
		}
		return espresso.Undefined
	})

	// h() → creates SSR vnode
	scope["h"] = espresso.NewNativeFunc(func(args []*espresso.Value) *espresso.Value {
		return evalHCallFromArgs(scope, args)
	})

	// createPortal(children, container) — in SSR, just renders children inline (no DOM)
	scope["createPortal"] = espresso.NewNativeFunc(func(args []*espresso.Value) *espresso.Value {
		if len(args) == 0 {
			return espresso.Undefined
		}
		// Return children as-is — they render inline during SSR
		return args[0]
	})

	// Image component — renders <img> with loading/priority attributes
	scope["Image"] = espresso.NewNativeFunc(func(args []*espresso.Value) *espresso.Value {
		props := make(map[string]*espresso.Value)
		if len(args) > 0 && args[0].Type() == espresso.TypeObject {
			for k, v := range args[0].Object() {
				props[k] = v
			}
		}

		node := &ssrNode{Tag: "img", Props: make(map[string]*espresso.Value)}

		// Copy src, alt, class, width, height
		for _, attr := range []string{"src", "alt", "class", "width", "height", "style"} {
			if v, ok := props[attr]; ok && !v.IsUndefined() {
				node.Props[attr] = v
			}
		}

		// Priority images: eager loading + high fetch priority
		if p, ok := props["priority"]; ok && p.Truthy() {
			node.Props["loading"] = espresso.NewStr("eager")
			node.Props["fetchpriority"] = espresso.NewStr("high")
			node.Props["decoding"] = espresso.NewStr("sync")
		} else {
			// Default: lazy loading
			node.Props["loading"] = espresso.NewStr("lazy")
			node.Props["decoding"] = espresso.NewStr("async")
		}

		// Placeholder blur support
		if ph, ok := props["placeholder"]; ok && ph.String() == "blur" {
			if blur, ok := props["blurDataURL"]; ok {
				node.Props["style"] = espresso.NewStr("background-image:url(" + blur.String() + ");background-size:cover;background-repeat:no-repeat")
			}
		}

		return espresso.NewCustom(node)
	})
}

// ── h() Call Handler ────────────────────────────────────

// evalHCallFromArgs implements h(tag, props, ...children) for SSR.
func evalHCallFromArgs(scope map[string]*espresso.Value, args []*espresso.Value) *espresso.Value {
	if len(args) == 0 {
		return espresso.Undefined
	}

	tag := args[0]
	var props *espresso.Value
	if len(args) > 1 {
		props = args[1]
	}

	// Component function call
	if tag.Type() == espresso.TypeFunc {
		callProps := make(map[string]*espresso.Value)
		if props != nil && props.Type() == espresso.TypeObject && props.Object() != nil {
			for k, v := range props.Object() {
				callProps[k] = v
			}
		}
		// Add children if present
		if len(args) > 2 {
			var childVals []*espresso.Value
			for _, arg := range args[2:] {
				childVals = append(childVals, arg)
			}
			callProps["children"] = espresso.NewArr(childVals)
		}
		return espresso.CallFunc(scope, tag, callProps)
	}

	// Regular HTML element
	tagStr := ""
	if tag.Type() == espresso.TypeString {
		tagStr = tag.String()
	}

	node := &ssrNode{Tag: tagStr}

	// Props
	if props != nil && props.Type() == espresso.TypeObject && props.Object() != nil {
		node.Props = make(map[string]*espresso.Value)
		for k, v := range props.Object() {
			node.Props[k] = v
		}
	}

	// Children
	for i := 2; i < len(args); i++ {
		children := valueToNodes(args[i])
		node.Children = append(node.Children, children...)
	}

	return espresso.NewCustom(node)
}

// ── Value → SSR Node conversion ─────────────────────────

// valueToNodes converts an espresso Value to SSR nodes.
func valueToNodes(v *espresso.Value) []*ssrNode {
	if v == nil || v.IsUndefined() || v.IsNull() {
		return nil
	}
	if v.IsCustom() {
		if node, ok := v.Custom.(*ssrNode); ok {
			return []*ssrNode{node}
		}
		return nil
	}
	if v.Type() == espresso.TypeBool {
		if !v.Truthy() {
			return nil // false renders nothing (like React)
		}
		return []*ssrNode{{Text: v.String(), IsText: true}}
	}
	if v.Type() == espresso.TypeArray {
		var nodes []*ssrNode
		for _, item := range v.Array() {
			nodes = append(nodes, valueToNodes(item)...)
		}
		return nodes
	}
	if v.Type() == espresso.TypeString || v.Type() == espresso.TypeNumber {
		s := v.String()
		if s == "" || s == "undefined" || s == "null" {
			return nil
		}
		return []*ssrNode{{Text: s, IsText: true}}
	}
	return nil
}

// ── SSR Node Type ───────────────────────────────────────

// ssrNode is the server-side virtual DOM node.
type ssrNode struct {
	Tag      string
	Props    map[string]*espresso.Value
	Children []*ssrNode
	Text     string
	IsText   bool
}

// ── Page/Layout Evaluation Helpers ──────────────────────

// ssrPageCache caches transpiled + parsed page data.
type ssrPageCache struct {
	funcBody     string
	funcParams   string
	localFuncs   map[string]*espresso.Value
	topLevelVars map[string]*espresso.Value
	tokens       []espresso.Tok
	interactive  bool
}

// extractMetadataFromSource extracts title/description from page source.
func extractMetadataFromSource(source string) *pageMetadata {
	titleIdx := strings.Index(source, "title:")
	if titleIdx < 0 {
		titleIdx = strings.Index(source, "title :")
	}
	if titleIdx < 0 {
		return nil
	}

	meta := &pageMetadata{}

	// Extract title
	rest := source[titleIdx+6:]
	rest = strings.TrimSpace(rest)
	if len(rest) > 0 && (rest[0] == '"' || rest[0] == '\'') {
		quote := rest[0]
		end := strings.IndexByte(rest[1:], quote)
		if end >= 0 {
			meta.Title = rest[1 : end+1]
		}
	}

	// Extract description
	descIdx := strings.Index(source, "description:")
	if descIdx < 0 {
		descIdx = strings.Index(source, "description :")
	}
	if descIdx >= 0 {
		rest = source[descIdx+12:]
		rest = strings.TrimSpace(rest)
		if len(rest) > 0 && (rest[0] == '"' || rest[0] == '\'') {
			quote := rest[0]
			end := strings.IndexByte(rest[1:], quote)
			if end >= 0 {
				meta.Description = rest[1 : end+1]
			}
		}
	}

	return meta
}

type pageMetadata struct {
	Title       string
	Description string
}

func min2(a, b int) int { if a < b { return a }; return b }
