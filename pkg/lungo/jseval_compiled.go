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
	// If statement
	IsIf      bool
	Condition compiledExpr
	IfBody    *compiledPage
	ElseBody  *compiledPage
	// For loop: for (init; cond; update) { body }
	IsForLoop  bool
	InitStmt   *compiledStmt   // init (const i = 0)
	LoopCond   compiledExpr    // condition
	LoopUpdate compiledExpr    // update expression (i++)
	LoopBody   *compiledPage   // body statements
	// For...of: for (const x of arr) { body }
	IsForOf    bool
	IterVar    string
	IterExpr   compiledExpr
	// While loop
	IsWhile    bool
	// Try/catch
	IsTryCatch bool
	TryBody    *compiledPage
	// Reassignment: name = expr (without const/let)
	IsReassign bool
	// Compound assignment: name += expr, name -= expr
	IsCompound bool
	CompoundOp string // "+=" or "-="
	// Increment/decrement: name++ or name--
	IsIncrement bool
	IncrDelta   float64 // +1 or -1
	// No-op (console.log etc.)
	IsNoop bool
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
	result := cp.executeStatements(scope)
	if result != nil {
		return result
	}
	if cp.ReturnExpr != nil {
		return cp.ReturnExpr(scope)
	}
	return jvUndefined
}

func (cp *compiledPage) executeStatements(scope map[string]*jsValue) *jsValue {
	for _, stmt := range cp.Preamble {
		if stmt.IsNoop {
			continue
		} else if stmt.IsIf {
			if stmt.Condition(scope).truthy() {
				if stmt.IfBody != nil {
					if result := stmt.IfBody.executeStatements(scope); result != nil {
						return result
					}
				}
			} else if stmt.ElseBody != nil {
				if result := stmt.ElseBody.executeStatements(scope); result != nil {
					return result
				}
			}
		} else if stmt.IsForLoop {
			// Execute init
			if stmt.InitStmt != nil && stmt.InitStmt.Expr != nil {
				scope[stmt.InitStmt.Name] = stmt.InitStmt.Expr(scope)
			}
			for iter := 0; iter < 10000; iter++ {
				if stmt.LoopCond != nil && !stmt.LoopCond(scope).truthy() {
					break
				}
				if stmt.LoopBody != nil {
					if result := stmt.LoopBody.executeStatements(scope); result != nil {
						return result
					}
				}
				if stmt.LoopUpdate != nil {
					stmt.LoopUpdate(scope)
				}
			}
		} else if stmt.IsForOf {
			arr := stmt.IterExpr(scope)
			if arr.typ == jsTypeArray {
				for _, item := range arr.array {
					scope[stmt.IterVar] = item
					if stmt.LoopBody != nil {
						if result := stmt.LoopBody.executeStatements(scope); result != nil {
							return result
						}
					}
				}
			}
		} else if stmt.IsWhile {
			for iter := 0; iter < 10000; iter++ {
				if stmt.LoopCond != nil && !stmt.LoopCond(scope).truthy() {
					break
				}
				if stmt.LoopBody != nil {
					if result := stmt.LoopBody.executeStatements(scope); result != nil {
						return result
					}
				}
			}
		} else if stmt.IsTryCatch {
			if stmt.TryBody != nil {
				if result := stmt.TryBody.executeStatements(scope); result != nil {
					return result
				}
			}
		} else if stmt.IsIncrement {
			if v, ok := scope[stmt.Name]; ok {
				scope[stmt.Name] = jvNum(v.toNum() + stmt.IncrDelta)
			}
		} else if stmt.IsCompound {
			if v, ok := scope[stmt.Name]; ok {
				val := stmt.Expr(scope)
				if stmt.CompoundOp == "+=" {
					if v.typ == jsTypeString || val.typ == jsTypeString {
						scope[stmt.Name] = jvStr(v.toStr() + val.toStr())
					} else {
						scope[stmt.Name] = jvNum(v.toNum() + val.toNum())
					}
				} else {
					scope[stmt.Name] = jvNum(v.toNum() - val.toNum())
				}
			}
		} else if stmt.IsReassign {
			scope[stmt.Name] = stmt.Expr(scope)
		} else if stmt.IsArrayDestructure {
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
		} else if stmt.Expr != nil {
			scope[stmt.Name] = stmt.Expr(scope)
		}
	}
	return nil
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
	// Run preamble — may contain if statements with early returns
	earlyResult := cp.executeStatements(scope)

	sb := sbPool.Get().(*strings.Builder)
	sb.Reset()

	// Check if an if-block returned early
	var val *jsValue
	if earlyResult != nil {
		val = earlyResult
	} else if cp.ReturnNode != nil {
		// Fast path: direct HTML rendering from compiled node tree
		cp.ReturnNode.renderDirectHTML(scope, sb)
		result := sb.String()
		sbPool.Put(sb)
		return result
	} else if cp.ReturnExpr != nil {
		val = cp.ReturnExpr(scope)
	}

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
