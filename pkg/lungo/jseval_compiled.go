package lungo

import (
	"strings"
	"sync"
)

// ─── Compiled SSR Evaluation ────────────────────────────────────
// Instead of walking tokens on every request, we compile the page's
// h() call tree into Go closures at startup. At request time, we
// execute the closures directly — no token parsing needed.

// compiledExpr evaluates to a jsValue given a scope.
type compiledExpr func(scope map[string]*jsValue) *jsValue

// compiledPage is the top-level compiled representation of a page function.
type compiledPage struct {
	Preamble   []compiledStmt
	ReturnExpr compiledExpr
	ReturnNode *compiledNode // direct HTML render path (nil if return isn't a simple h() tree)
}

type compiledStmt struct {
	// Simple assignment: name = expr
	Name string
	Expr compiledExpr
	// Array destructuring: [name1, name2] = expr
	IsArrayDestructure bool
	Names              []string
}

// compiledNode is a pre-analyzed SSR vnode.
type compiledNode struct {
	Tag          string
	StaticProps  map[string]string       // props known at compile time
	DynamicProps map[string]compiledExpr // props needing runtime eval
	Children     []compiledChild
	IsText       bool
	StaticText   string
	DynamicText  compiledExpr
}

type compiledChild struct {
	// Exactly one of these is set:
	Node    *compiledNode // static sub-element
	Expr    compiledExpr  // dynamic expression (text, vnode, array, ternary)
	MapExpr *compiledMap  // .map() pattern
}

type compiledMap struct {
	ArrayExpr compiledExpr
	ParamName string
	IndexName string
	Body      *compiledNode
}

// ─── Execution ──────────────────────────────────────────────────

func (cp *compiledPage) execute(scope map[string]*jsValue) *jsValue {
	// Run preamble (const declarations)
	for _, stmt := range cp.Preamble {
		if stmt.IsArrayDestructure {
			val := stmt.Expr(scope)
			if val.typ == jsTypeArray {
				for i, name := range stmt.Names {
					if i < len(val.array) {
						scope[name] = val.array[i]
					} else {
						scope[name] = jvUndefined
					}
				}
			} else {
				for _, name := range stmt.Names {
					scope[name] = jvUndefined
				}
			}
		} else {
			scope[stmt.Name] = stmt.Expr(scope)
		}
	}
	// Execute return expression
	return cp.ReturnExpr(scope)
}

func (cn *compiledNode) execute(scope map[string]*jsValue) *ssrNode {
	if cn.IsText {
		if cn.DynamicText != nil {
			val := cn.DynamicText(scope)
			if val == nil || val.typ == jsTypeUndefined || val.typ == jsTypeNull {
				return nil
			}
			return &ssrNode{Text: val.toStr(), IsText: true}
		}
		return &ssrNode{Text: cn.StaticText, IsText: true}
	}

	node := &ssrNode{Tag: cn.Tag}

	// Build props
	if len(cn.StaticProps) > 0 || len(cn.DynamicProps) > 0 {
		node.Props = make(map[string]*jsValue, len(cn.StaticProps)+len(cn.DynamicProps))
		for k, v := range cn.StaticProps {
			node.Props[k] = jvStr(v)
		}
		for k, expr := range cn.DynamicProps {
			node.Props[k] = expr(scope)
		}
	}

	// Build children
	for _, child := range cn.Children {
		if child.Node != nil {
			if n := child.Node.execute(scope); n != nil {
				node.Children = append(node.Children, n)
			}
		} else if child.MapExpr != nil {
			node.Children = append(node.Children, child.MapExpr.execute(scope)...)
		} else if child.Expr != nil {
			val := child.Expr(scope)
			node.Children = append(node.Children, jsValueToNodes(val)...)
		}
	}

	return node
}

func (m *compiledMap) execute(scope map[string]*jsValue) []*ssrNode {
	arr := m.ArrayExpr(scope)
	if arr.typ != jsTypeArray {
		return nil
	}

	nodes := make([]*ssrNode, 0, len(arr.array))
	for i, item := range arr.array {
		childScope := getPooledScope(scope)
		childScope[m.ParamName] = item
		if m.IndexName != "" {
			childScope[m.IndexName] = jvNum(float64(i))
		}
		if n := m.Body.execute(childScope); n != nil {
			nodes = append(nodes, n)
		}
		putPooledScope(childScope)
	}
	return nodes
}

// ─── Scope Pool ─────────────────────────────────────────────────

var scopePool = sync.Pool{
	New: func() interface{} {
		return make(map[string]*jsValue, 16)
	},
}

func getPooledScope(parent map[string]*jsValue) map[string]*jsValue {
	m := scopePool.Get().(map[string]*jsValue)
	for k := range m {
		delete(m, k)
	}
	for k, v := range parent {
		m[k] = v
	}
	return m
}

func putPooledScope(m map[string]*jsValue) {
	scopePool.Put(m)
}

// ─── String Builder Pool ────────────────────────────────────────

var sbPool = sync.Pool{
	New: func() interface{} {
		return &strings.Builder{}
	},
}

// ─── Direct HTML Rendering (skip vnode intermediary) ────────────
// These methods write HTML directly to a Builder, avoiding ssrNode allocations.

func (cp *compiledPage) renderHTML(scope map[string]*jsValue) string {
	// Run preamble
	for _, stmt := range cp.Preamble {
		if stmt.IsArrayDestructure {
			val := stmt.Expr(scope)
			if val.typ == jsTypeArray {
				for i, name := range stmt.Names {
					if i < len(val.array) {
						scope[name] = val.array[i]
					} else {
						scope[name] = jvUndefined
					}
				}
			} else {
				for _, name := range stmt.Names {
					scope[name] = jvUndefined
				}
			}
		} else {
			scope[stmt.Name] = stmt.Expr(scope)
		}
	}

	sb := sbPool.Get().(*strings.Builder)
	sb.Reset()

	// Fast path: direct HTML rendering from compiled node tree
	if cp.ReturnNode != nil {
		cp.ReturnNode.renderDirectHTML(scope, sb)
		result := sb.String()
		sbPool.Put(sb)
		return result
	}

	// Fallback: execute to vnode then render
	val := cp.ReturnExpr(scope)
	if val == nil || val.typ != jsTypeVNode || val.vnode == nil {
		sbPool.Put(sb)
		return ""
	}
	renderNode(val.vnode, sb)
	result := sb.String()
	sbPool.Put(sb)
	return result
}

func (cn *compiledNode) renderDirectHTML(scope map[string]*jsValue, sb *strings.Builder) {
	if cn.IsText {
		if cn.DynamicText != nil {
			val := cn.DynamicText(scope)
			if val != nil && val.typ != jsTypeUndefined && val.typ != jsTypeNull {
				sb.WriteString(htmlEscape(val.toStr()))
			}
		} else {
			sb.WriteString(htmlEscape(cn.StaticText))
		}
		return
	}

	if cn.Tag == "" {
		// Fragment
		for _, child := range cn.Children {
			child.renderDirectHTML(scope, sb)
		}
		return
	}

	sb.WriteByte('<')
	sb.WriteString(cn.Tag)

	// Static props (no allocation needed)
	for k, v := range cn.StaticProps {
		if strings.HasPrefix(k, "on") && len(k) > 2 {
			continue // skip event handlers
		}
		sb.WriteByte(' ')
		sb.WriteString(k)
		sb.WriteString(`="`)
		sb.WriteString(htmlEscapeAttr(v))
		sb.WriteByte('"')
	}

	// Dynamic props
	for k, expr := range cn.DynamicProps {
		if strings.HasPrefix(k, "on") && len(k) > 2 {
			continue
		}
		if k == "ref" || k == "key" || k == "children" {
			continue
		}
		val := expr(scope)
		if val.typ == jsTypeUndefined || val.typ == jsTypeNull {
			continue
		}
		if k == "style" && val.typ == jsTypeObject {
			sb.WriteString(` style="`)
			renderStyleObject(val, sb)
			sb.WriteByte('"')
			continue
		}
		if val.typ == jsTypeBool {
			if val.bool {
				sb.WriteByte(' ')
				sb.WriteString(k)
			}
			continue
		}
		sb.WriteByte(' ')
		sb.WriteString(k)
		sb.WriteString(`="`)
		sb.WriteString(htmlEscapeAttr(val.toStr()))
		sb.WriteByte('"')
	}

	if isVoidElement(cn.Tag) {
		sb.WriteString(" />")
		return
	}

	sb.WriteByte('>')

	for _, child := range cn.Children {
		child.renderDirectHTML(scope, sb)
	}

	sb.WriteString("</")
	sb.WriteString(cn.Tag)
	sb.WriteByte('>')
}

func (cc compiledChild) renderDirectHTML(scope map[string]*jsValue, sb *strings.Builder) {
	if cc.Node != nil {
		cc.Node.renderDirectHTML(scope, sb)
	} else if cc.MapExpr != nil {
		cc.MapExpr.renderDirectHTML(scope, sb)
	} else if cc.Expr != nil {
		val := cc.Expr(scope)
		renderJSValueHTML(val, sb)
	}
}

func (m *compiledMap) renderDirectHTML(scope map[string]*jsValue, sb *strings.Builder) {
	arr := m.ArrayExpr(scope)
	if arr.typ != jsTypeArray {
		return
	}
	for i, item := range arr.array {
		childScope := getPooledScope(scope)
		childScope[m.ParamName] = item
		if m.IndexName != "" {
			childScope[m.IndexName] = jvNum(float64(i))
		}
		m.Body.renderDirectHTML(childScope, sb)
		putPooledScope(childScope)
	}
}

func renderJSValueHTML(v *jsValue, sb *strings.Builder) {
	if v == nil {
		return
	}
	switch v.typ {
	case jsTypeVNode:
		if v.vnode != nil {
			renderNode(v.vnode, sb)
		}
	case jsTypeString:
		if v.str != "" {
			sb.WriteString(htmlEscape(v.str))
		}
	case jsTypeNumber:
		sb.WriteString(v.toStr())
	case jsTypeArray:
		for _, item := range v.array {
			renderJSValueHTML(item, sb)
		}
	case jsTypeBool:
		if v.bool {
			sb.WriteString("true")
		}
	}
}

// RenderSSRHTMLPooled uses a pooled string builder.
func RenderSSRHTMLPooled(node *ssrNode) string {
	if node == nil {
		return ""
	}
	sb := sbPool.Get().(*strings.Builder)
	sb.Reset()
	renderNode(node, sb)
	result := sb.String()
	sbPool.Put(sb)
	return result
}
