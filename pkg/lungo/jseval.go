package lungo

import (
	"encoding/json"
	"strconv"
	"strings"
)

// ─── Value Types ────────────────────────────────────────────────

type jsType int

const (
	jsTypeUndefined jsType = iota
	jsTypeNull
	jsTypeBool
	jsTypeNumber
	jsTypeString
	jsTypeArray
	jsTypeObject
	jsTypeVNode
	jsTypeFunc
)

type jsValue struct {
	typ    jsType
	bool   bool
	num    float64
	str    string
	array  []*jsValue
	object map[string]*jsValue
	vnode  *ssrNode
	// captured function (for .map callbacks, component functions)
	fnParams []string
	fnBody   string
	fnScope  *jsEval
}

// ssrNode is the server-side virtual DOM node.
type ssrNode struct {
	Tag      string
	Props    map[string]*jsValue
	Children []*ssrNode
	Text     string
	IsText   bool
}

var jvUndefined = &jsValue{typ: jsTypeUndefined}
var jvNull = &jsValue{typ: jsTypeNull}
var jvTrue = &jsValue{typ: jsTypeBool, bool: true}
var jvFalse = &jsValue{typ: jsTypeBool, bool: false}

func jvStr(s string) *jsValue   { return &jsValue{typ: jsTypeString, str: s} }
func jvNum(n float64) *jsValue  { return &jsValue{typ: jsTypeNumber, num: n} }
func jvArr(a []*jsValue) *jsValue { return &jsValue{typ: jsTypeArray, array: a} }
func jvObj(o map[string]*jsValue) *jsValue { return &jsValue{typ: jsTypeObject, object: o} }
func jvBool(b bool) *jsValue {
	if b {
		return jvTrue
	}
	return jvFalse
}
func jvNode(n *ssrNode) *jsValue { return &jsValue{typ: jsTypeVNode, vnode: n} }

func (v *jsValue) truthy() bool {
	switch v.typ {
	case jsTypeUndefined, jsTypeNull:
		return false
	case jsTypeBool:
		return v.bool
	case jsTypeNumber:
		return v.num != 0
	case jsTypeString:
		return v.str != ""
	case jsTypeArray, jsTypeObject, jsTypeVNode, jsTypeFunc:
		return true
	}
	return false
}

func (v *jsValue) toStr() string {
	switch v.typ {
	case jsTypeString:
		return v.str
	case jsTypeNumber:
		if v.num == float64(int64(v.num)) {
			return strconv.FormatInt(int64(v.num), 10)
		}
		return strconv.FormatFloat(v.num, 'f', -1, 64)
	case jsTypeBool:
		if v.bool {
			return "true"
		}
		return "false"
	case jsTypeNull:
		return "null"
	case jsTypeUndefined:
		return "undefined"
	}
	return ""
}

func (v *jsValue) toNum() float64 {
	switch v.typ {
	case jsTypeNumber:
		return v.num
	case jsTypeBool:
		if v.bool {
			return 1
		}
		return 0
	case jsTypeString:
		n, err := strconv.ParseFloat(v.str, 64)
		if err != nil {
			return 0
		}
		return n
	}
	return 0
}

func (v *jsValue) getProp(key string) *jsValue {
	switch v.typ {
	case jsTypeObject:
		if v.object != nil {
			if val, ok := v.object[key]; ok {
				return val
			}
		}
		return jvUndefined
	case jsTypeArray:
		switch key {
		case "length":
			return jvNum(float64(len(v.array)))
		}
		if idx, err := strconv.Atoi(key); err == nil && idx >= 0 && idx < len(v.array) {
			return v.array[idx]
		}
		return jvUndefined
	case jsTypeString:
		switch key {
		case "length":
			return jvNum(float64(len(v.str)))
		}
		return jvUndefined
	}
	return jvUndefined
}

// ─── Tokenizer ──────────────────────────────────────────────────

type tokType int

const (
	tokEOF tokType = iota
	tokIdent
	tokNum
	tokStr
	tokDot
	tokOptChain // ?.
	tokLParen
	tokRParen
	tokLBrack
	tokRBrack
	tokLBrace
	tokRBrace
	tokComma
	tokColon
	tokSemi
	tokPlus
	tokMinus
	tokStar
	tokSlash
	tokPercent
	tokEqEqEq
	tokNotEqEq
	tokEqEq
	tokNotEq
	tokGtEq
	tokLtEq
	tokGt
	tokLt
	tokAnd
	tokOr
	tokNot
	tokQuestion
	tokArrow // =>
	tokAssign
	tokSpread // ...
)

type tok struct {
	t   tokType
	v   string
	n   float64
}

func jsTokenize(src string) []tok {
	var tokens []tok
	i := 0
	for i < len(src) {
		// skip whitespace
		for i < len(src) && (src[i] == ' ' || src[i] == '\t' || src[i] == '\n' || src[i] == '\r') {
			i++
		}
		if i >= len(src) {
			break
		}
		ch := src[i]

		// line comment
		if ch == '/' && i+1 < len(src) && src[i+1] == '/' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		// block comment
		if ch == '/' && i+1 < len(src) && src[i+1] == '*' {
			i += 2
			for i+1 < len(src) {
				if src[i] == '*' && src[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			continue
		}

		// string
		if ch == '"' || ch == '\'' {
			i++
			var sb strings.Builder
			for i < len(src) && src[i] != ch {
				if src[i] == '\\' && i+1 < len(src) {
					i++
					switch src[i] {
					case 'n':
						sb.WriteByte('\n')
					case 't':
						sb.WriteByte('\t')
					case '\\':
						sb.WriteByte('\\')
					default:
						sb.WriteByte(src[i])
					}
				} else {
					sb.WriteByte(src[i])
				}
				i++
			}
			if i < len(src) {
				i++
			}
			tokens = append(tokens, tok{t: tokStr, v: sb.String()})
			continue
		}

		// template literal (simple, no interpolation in transpiled output)
		if ch == '`' {
			i++
			var sb strings.Builder
			for i < len(src) && src[i] != '`' {
				if src[i] == '\\' && i+1 < len(src) {
					i++
					sb.WriteByte(src[i])
				} else {
					sb.WriteByte(src[i])
				}
				i++
			}
			if i < len(src) {
				i++
			}
			tokens = append(tokens, tok{t: tokStr, v: sb.String()})
			continue
		}

		// number
		if ch >= '0' && ch <= '9' {
			start := i
			for i < len(src) && ((src[i] >= '0' && src[i] <= '9') || src[i] == '.') {
				i++
			}
			n, _ := strconv.ParseFloat(src[start:i], 64)
			tokens = append(tokens, tok{t: tokNum, v: src[start:i], n: n})
			continue
		}

		// identifier
		if isJSIdentStart(ch) {
			start := i
			for i < len(src) && isJSIdentChar(src[i]) {
				i++
			}
			tokens = append(tokens, tok{t: tokIdent, v: src[start:i]})
			continue
		}

		// multi-char operators
		if ch == '.' && i+2 < len(src) && src[i+1] == '.' && src[i+2] == '.' {
			tokens = append(tokens, tok{t: tokSpread})
			i += 3
			continue
		}
		if ch == '=' && i+2 < len(src) && src[i+1] == '=' && src[i+2] == '=' {
			tokens = append(tokens, tok{t: tokEqEqEq})
			i += 3
			continue
		}
		if ch == '!' && i+2 < len(src) && src[i+1] == '=' && src[i+2] == '=' {
			tokens = append(tokens, tok{t: tokNotEqEq})
			i += 3
			continue
		}
		if ch == '=' && i+1 < len(src) && src[i+1] == '>' {
			tokens = append(tokens, tok{t: tokArrow})
			i += 2
			continue
		}
		if ch == '=' && i+1 < len(src) && src[i+1] == '=' {
			tokens = append(tokens, tok{t: tokEqEq})
			i += 2
			continue
		}
		if ch == '!' && i+1 < len(src) && src[i+1] == '=' {
			tokens = append(tokens, tok{t: tokNotEq})
			i += 2
			continue
		}
		if ch == '&' && i+1 < len(src) && src[i+1] == '&' {
			tokens = append(tokens, tok{t: tokAnd})
			i += 2
			continue
		}
		if ch == '|' && i+1 < len(src) && src[i+1] == '|' {
			tokens = append(tokens, tok{t: tokOr})
			i += 2
			continue
		}
		if ch == '>' && i+1 < len(src) && src[i+1] == '=' {
			tokens = append(tokens, tok{t: tokGtEq})
			i += 2
			continue
		}
		if ch == '<' && i+1 < len(src) && src[i+1] == '=' {
			tokens = append(tokens, tok{t: tokLtEq})
			i += 2
			continue
		}
		if ch == '?' && i+1 < len(src) && src[i+1] == '.' {
			tokens = append(tokens, tok{t: tokOptChain})
			i += 2
			continue
		}

		// single-char
		switch ch {
		case '.':
			tokens = append(tokens, tok{t: tokDot})
		case '(':
			tokens = append(tokens, tok{t: tokLParen})
		case ')':
			tokens = append(tokens, tok{t: tokRParen})
		case '[':
			tokens = append(tokens, tok{t: tokLBrack})
		case ']':
			tokens = append(tokens, tok{t: tokRBrack})
		case '{':
			tokens = append(tokens, tok{t: tokLBrace})
		case '}':
			tokens = append(tokens, tok{t: tokRBrace})
		case ',':
			tokens = append(tokens, tok{t: tokComma})
		case ':':
			tokens = append(tokens, tok{t: tokColon})
		case ';':
			tokens = append(tokens, tok{t: tokSemi})
		case '+':
			tokens = append(tokens, tok{t: tokPlus})
		case '-':
			tokens = append(tokens, tok{t: tokMinus})
		case '*':
			tokens = append(tokens, tok{t: tokStar})
		case '/':
			tokens = append(tokens, tok{t: tokSlash})
		case '%':
			tokens = append(tokens, tok{t: tokPercent})
		case '>':
			tokens = append(tokens, tok{t: tokGt})
		case '<':
			tokens = append(tokens, tok{t: tokLt})
		case '!':
			tokens = append(tokens, tok{t: tokNot})
		case '?':
			tokens = append(tokens, tok{t: tokQuestion})
		case '=':
			tokens = append(tokens, tok{t: tokAssign})
		}
		i++
	}
	tokens = append(tokens, tok{t: tokEOF})
	return tokens
}

func isJSIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_' || ch == '$'
}
func isJSIdentChar(ch byte) bool {
	return isJSIdentStart(ch) || (ch >= '0' && ch <= '9')
}

// ─── Evaluator ──────────────────────────────────────────────────

type jsEval struct {
	tokens []tok
	pos    int
	scope  map[string]*jsValue
}

func newJSEval(src string, scope map[string]*jsValue) *jsEval {
	return &jsEval{
		tokens: jsTokenize(src),
		pos:    0,
		scope:  scope,
	}
}

func (e *jsEval) peek() tok {
	if e.pos < len(e.tokens) {
		return e.tokens[e.pos]
	}
	return tok{t: tokEOF}
}

func (e *jsEval) advance() tok {
	t := e.peek()
	if e.pos < len(e.tokens) {
		e.pos++
	}
	return t
}

func (e *jsEval) expect(t tokType) tok {
	tk := e.advance()
	if tk.t != t {
		// best-effort: return what we got
	}
	return tk
}

func (e *jsEval) childScope() *jsEval {
	child := make(map[string]*jsValue, len(e.scope))
	for k, v := range e.scope {
		child[k] = v
	}
	return &jsEval{scope: child}
}

// Eval evaluates the source and returns the result.
func jsEvalExpr(src string, scope map[string]*jsValue) *jsValue {
	ev := newJSEval(src, scope)
	return ev.expr()
}

// ─── Recursive Descent ─────────────────────────────────────────

func (e *jsEval) expr() *jsValue {
	return e.ternary()
}

func (e *jsEval) ternary() *jsValue {
	val := e.logicalOr()
	if e.peek().t == tokQuestion {
		e.advance() // skip ?
		if val.truthy() {
			consequent := e.expr()
			if e.peek().t == tokColon {
				e.advance()
				e.skipExpr() // skip alternate without evaluating
			}
			return consequent
		}
		e.skipExpr() // skip consequent without evaluating
		if e.peek().t == tokColon {
			e.advance()
			return e.expr()
		}
		// Missing ": alternate" (transpiler bug) — treat as null
		return jvNull
	}
	return val
}

// skipExpr skips a complete expression without evaluating it.
// It counts balanced parens/brackets/braces to handle nested expressions.
func (e *jsEval) skipExpr() {
	depth := 0
	for e.pos < len(e.tokens) {
		t := e.tokens[e.pos]
		switch t.t {
		case tokLParen, tokLBrack, tokLBrace:
			depth++
			e.pos++
		case tokRParen, tokRBrack, tokRBrace:
			if depth == 0 {
				return // this closer belongs to parent
			}
			depth--
			e.pos++
		case tokComma:
			if depth == 0 {
				return // comma at top level ends the expression
			}
			e.pos++
		case tokColon:
			if depth == 0 {
				return // colon at top level (ternary else)
			}
			e.pos++
		case tokSemi:
			if depth == 0 {
				return
			}
			e.pos++
		case tokEOF:
			return
		default:
			e.pos++
		}
	}
}

func (e *jsEval) logicalOr() *jsValue {
	left := e.logicalAnd()
	for e.peek().t == tokOr {
		e.advance()
		right := e.logicalAnd()
		if left.truthy() {
			return left
		}
		left = right
	}
	return left
}

func (e *jsEval) logicalAnd() *jsValue {
	left := e.equality()
	for e.peek().t == tokAnd {
		e.advance()
		right := e.equality()
		if !left.truthy() {
			return left
		}
		left = right
	}
	return left
}

func (e *jsEval) equality() *jsValue {
	left := e.comparison()
	for {
		t := e.peek().t
		if t == tokEqEqEq || t == tokEqEq {
			e.advance()
			right := e.comparison()
			left = jvBool(jsStrictEqual(left, right))
		} else if t == tokNotEqEq || t == tokNotEq {
			e.advance()
			right := e.comparison()
			left = jvBool(!jsStrictEqual(left, right))
		} else {
			break
		}
	}
	return left
}

func jsStrictEqual(a, b *jsValue) bool {
	if a.typ != b.typ {
		return false
	}
	switch a.typ {
	case jsTypeUndefined, jsTypeNull:
		return true
	case jsTypeBool:
		return a.bool == b.bool
	case jsTypeNumber:
		return a.num == b.num
	case jsTypeString:
		return a.str == b.str
	}
	return a == b // reference equality for objects/arrays
}

func (e *jsEval) comparison() *jsValue {
	left := e.additive()
	for {
		t := e.peek().t
		switch t {
		case tokGt:
			e.advance()
			right := e.additive()
			left = jvBool(left.toNum() > right.toNum())
		case tokLt:
			e.advance()
			right := e.additive()
			left = jvBool(left.toNum() < right.toNum())
		case tokGtEq:
			e.advance()
			right := e.additive()
			left = jvBool(left.toNum() >= right.toNum())
		case tokLtEq:
			e.advance()
			right := e.additive()
			left = jvBool(left.toNum() <= right.toNum())
		default:
			return left
		}
	}
}

func (e *jsEval) additive() *jsValue {
	left := e.multiplicative()
	for {
		t := e.peek().t
		if t == tokPlus {
			e.advance()
			right := e.multiplicative()
			// string concatenation if either side is string
			if left.typ == jsTypeString || right.typ == jsTypeString {
				left = jvStr(left.toStr() + right.toStr())
			} else {
				left = jvNum(left.toNum() + right.toNum())
			}
		} else if t == tokMinus {
			e.advance()
			right := e.multiplicative()
			left = jvNum(left.toNum() - right.toNum())
		} else {
			break
		}
	}
	return left
}

func (e *jsEval) multiplicative() *jsValue {
	left := e.unary()
	for {
		t := e.peek().t
		if t == tokStar {
			e.advance()
			right := e.unary()
			left = jvNum(left.toNum() * right.toNum())
		} else if t == tokSlash {
			e.advance()
			right := e.unary()
			if right.toNum() != 0 {
				left = jvNum(left.toNum() / right.toNum())
			} else {
				left = jvNum(0)
			}
		} else if t == tokPercent {
			e.advance()
			right := e.unary()
			if right.toNum() != 0 {
				left = jvNum(float64(int64(left.toNum()) % int64(right.toNum())))
			} else {
				left = jvNum(0)
			}
		} else {
			break
		}
	}
	return left
}

func (e *jsEval) unary() *jsValue {
	if e.peek().t == tokNot {
		e.advance()
		val := e.unary()
		return jvBool(!val.truthy())
	}
	if e.peek().t == tokMinus {
		e.advance()
		val := e.unary()
		return jvNum(-val.toNum())
	}
	if e.peek().t == tokIdent && e.peek().v == "typeof" {
		e.advance()
		val := e.unary()
		switch val.typ {
		case jsTypeUndefined:
			return jvStr("undefined")
		case jsTypeNull:
			return jvStr("object")
		case jsTypeBool:
			return jvStr("boolean")
		case jsTypeNumber:
			return jvStr("number")
		case jsTypeString:
			return jvStr("string")
		case jsTypeFunc:
			return jvStr("function")
		default:
			return jvStr("object")
		}
	}
	return e.postfix()
}

func (e *jsEval) postfix() *jsValue {
	val := e.primary()
	for {
		switch e.peek().t {
		case tokDot:
			e.advance()
			prop := e.advance()
			if prop.t == tokIdent {
				val = e.handlePropAccess(val, prop.v)
			}
		case tokOptChain:
			e.advance()
			prop := e.advance()
			if !val.truthy() || val.typ == jsTypeUndefined || val.typ == jsTypeNull {
				val = jvUndefined
				// skip any subsequent call
				if e.peek().t == tokLParen {
					e.skipBalanced(tokLParen, tokRParen)
				}
			} else if prop.t == tokIdent {
				val = e.handlePropAccess(val, prop.v)
			}
		case tokLBrack:
			e.advance()
			idx := e.expr()
			e.expect(tokRBrack)
			val = val.getProp(idx.toStr())
		case tokLParen:
			// Direct function call: val(args...)
			if val.typ == jsTypeFunc {
				val = e.evalFuncCall(val)
			} else {
				// Not a function — this ( belongs to something else
				return val
			}
		default:
			return val
		}
	}
}

// isArrowFunction looks ahead to check if the current ( starts an arrow function.
func (e *jsEval) isArrowFunction() bool {
	// Save position
	saved := e.pos
	defer func() { e.pos = saved }()

	e.pos++ // skip (
	depth := 1
	for e.pos < len(e.tokens) && depth > 0 {
		if e.tokens[e.pos].t == tokLParen {
			depth++
		} else if e.tokens[e.pos].t == tokRParen {
			depth--
		}
		e.pos++
	}
	// After the closing ), check for =>
	return e.pos < len(e.tokens) && e.tokens[e.pos].t == tokArrow
}

// parseArrowFunction parses an arrow function and returns it as a no-op func value.
// For SSR, arrow functions in props (event handlers) are skipped.
func (e *jsEval) parseArrowFunction() *jsValue {
	// Skip params
	e.advance() // skip (
	for e.peek().t != tokRParen && e.peek().t != tokEOF {
		e.advance()
	}
	e.expect(tokRParen)
	e.expect(tokArrow) // skip =>

	// Skip the body
	if e.peek().t == tokLBrace {
		e.skipBalanced(tokLBrace, tokRBrace)
	} else {
		// Expression body — skip one expression
		e.expr()
	}

	return &jsValue{typ: jsTypeFunc, str: "__noop"}
}

func (e *jsEval) evalFuncCall(fn *jsValue) *jsValue {
	e.advance() // skip (

	// Collect args
	var args []*jsValue
	for e.peek().t != tokRParen && e.peek().t != tokEOF {
		// For arrow function args, check if this is an arrow func being passed
		args = append(args, e.expr())
		if e.peek().t == tokComma {
			e.advance()
		}
	}
	e.expect(tokRParen)

	// Handle hook stubs
	if fn.str == "__hook_useState" {
		initial := jvFalse
		if len(args) > 0 {
			initial = args[0]
		}
		// If initial is a function (lazy initializer), we can't call it — use false
		if initial.typ == jsTypeFunc {
			initial = jvFalse
		}
		// Return [value, setter] — setter is a no-op
		return jvArr([]*jsValue{initial, &jsValue{typ: jsTypeFunc, str: "__noop"}})
	}
	if fn.str == "__hook_useEffect" {
		// No-op — skip effect
		return jvUndefined
	}
	if fn.str == "__hook_useRouter" {
		// Return a stub router object
		return jvObj(map[string]*jsValue{
			"pathname": jvStr("/"),
			"query":    jvObj(map[string]*jsValue{}),
		})
	}
	if fn.str == "__hook_useRef" {
		initial := jvNull
		if len(args) > 0 {
			initial = args[0]
		}
		return jvObj(map[string]*jsValue{"current": initial})
	}
	if fn.str == "__hook_useMemo" {
		// Call the memo function
		if len(args) > 0 && args[0].typ == jsTypeFunc {
			// Can't easily call it, return undefined
		}
		return jvUndefined
	}
	if fn.str == "__noop" {
		return jvUndefined
	}

	return jvUndefined
}

func (e *jsEval) handlePropAccess(val *jsValue, prop string) *jsValue {
	// Check for method calls
	if e.peek().t == tokLParen {
		return e.handleMethodCall(val, prop)
	}
	return val.getProp(prop)
}

func (e *jsEval) handleMethodCall(val *jsValue, method string) *jsValue {
	e.advance() // skip (

	switch method {
	case "map":
		return e.evalMapFilter(val, method)
	case "filter":
		return e.evalMapFilter(val, method)
	case "join":
		arg := jvStr(",")
		if e.peek().t != tokRParen {
			arg = e.expr()
		}
		e.expect(tokRParen)
		if val.typ == jsTypeArray {
			var parts []string
			for _, item := range val.array {
				parts = append(parts, item.toStr())
			}
			return jvStr(strings.Join(parts, arg.str))
		}
		return jvStr("")
	case "split":
		arg := jvStr("")
		if e.peek().t != tokRParen {
			arg = e.expr()
		}
		e.expect(tokRParen)
		if val.typ == jsTypeString {
			parts := strings.Split(val.str, arg.str)
			arr := make([]*jsValue, len(parts))
			for i, p := range parts {
				arr[i] = jvStr(p)
			}
			return jvArr(arr)
		}
		return jvArr(nil)
	case "trim":
		e.expect(tokRParen)
		if val.typ == jsTypeString {
			return jvStr(strings.TrimSpace(val.str))
		}
		return val
	case "includes":
		arg := e.expr()
		e.expect(tokRParen)
		if val.typ == jsTypeString {
			return jvBool(strings.Contains(val.str, arg.toStr()))
		}
		if val.typ == jsTypeArray {
			for _, item := range val.array {
				if jsStrictEqual(item, arg) {
					return jvTrue
				}
			}
			return jvFalse
		}
		return jvFalse
	case "slice":
		start := 0
		end := -1
		if e.peek().t != tokRParen {
			start = int(e.expr().toNum())
			if e.peek().t == tokComma {
				e.advance()
				end = int(e.expr().toNum())
			}
		}
		e.expect(tokRParen)
		if val.typ == jsTypeArray {
			arr := val.array
			if start < 0 {
				start = len(arr) + start
			}
			if start < 0 {
				start = 0
			}
			if end < 0 {
				end = len(arr)
			}
			if end > len(arr) {
				end = len(arr)
			}
			if start >= end {
				return jvArr(nil)
			}
			return jvArr(arr[start:end])
		}
		return val
	case "toString":
		e.expect(tokRParen)
		return jvStr(val.toStr())
	case "isArray":
		// Array.isArray(x)
		arg := e.expr()
		e.expect(tokRParen)
		return jvBool(arg.typ == jsTypeArray)
	default:
		// unknown method — skip args and return undefined
		e.skipBalanced(tokLParen, tokRParen)
		return jvUndefined
	}
}

func (e *jsEval) evalMapFilter(val *jsValue, method string) *jsValue {
	// Parse arrow function: param => expr  or  (param) => expr  or  (param, idx) => expr
	params := e.parseArrowParams()
	// skip =>
	e.expect(tokArrow)

	// Capture the body tokens until matching )
	bodyStart := e.pos
	// Check if body is wrapped in parens or braces
	hasBodyParen := e.peek().t == tokLParen
	hasBodyBrace := e.peek().t == tokLBrace
	if hasBodyParen {
		e.skipBalanced(tokLParen, tokRParen)
	} else if hasBodyBrace {
		e.skipBalanced(tokLBrace, tokRBrace)
	} else {
		// expression body — read until the closing ) of .map()
		depth := 1
		for e.pos < len(e.tokens) {
			if e.tokens[e.pos].t == tokLParen {
				depth++
			} else if e.tokens[e.pos].t == tokRParen {
				depth--
				if depth == 0 {
					break
				}
			}
			e.pos++
		}
	}
	bodyEnd := e.pos
	e.expect(tokRParen) // close .map()

	if val.typ != jsTypeArray {
		return jvArr(nil)
	}

	var results []*jsValue
	for i, item := range val.array {
		// Create child scope with callback param bound
		childScope := make(map[string]*jsValue, len(e.scope)+2)
		for k, v := range e.scope {
			childScope[k] = v
		}
		if len(params) > 0 {
			childScope[params[0]] = item
		}
		if len(params) > 1 {
			childScope[params[1]] = jvNum(float64(i))
		}

		// Evaluate body tokens
		bodyTokens := make([]tok, bodyEnd-bodyStart)
		copy(bodyTokens, e.tokens[bodyStart:bodyEnd])

		// If body was wrapped in parens, strip outer parens
		if hasBodyParen && len(bodyTokens) >= 2 && bodyTokens[0].t == tokLParen {
			bodyTokens = bodyTokens[1 : len(bodyTokens)-1]
		}

		// If body was wrapped in braces, extract return expression
		if hasBodyBrace && len(bodyTokens) >= 2 && bodyTokens[0].t == tokLBrace {
			bodyTokens = extractReturnFromBlock(bodyTokens)
		}

		bodyTokens = append(bodyTokens, tok{t: tokEOF})
		childEval := &jsEval{tokens: bodyTokens, pos: 0, scope: childScope}
		result := childEval.expr()

		if method == "filter" {
			if result.truthy() {
				results = append(results, item)
			}
		} else {
			results = append(results, result)
		}
	}

	return jvArr(results)
}

func extractReturnFromBlock(tokens []tok) []tok {
	// Strip outer { }
	if len(tokens) < 2 {
		return tokens
	}
	inner := tokens[1 : len(tokens)-1]
	// Find "return" and take everything after it until ; or end
	for i, t := range inner {
		if t.t == tokIdent && t.v == "return" {
			rest := inner[i+1:]
			// Strip trailing ;
			if len(rest) > 0 && rest[len(rest)-1].t == tokSemi {
				rest = rest[:len(rest)-1]
			}
			return rest
		}
	}
	return inner
}

func (e *jsEval) parseArrowParams() []string {
	var params []string
	if e.peek().t == tokLParen {
		e.advance() // skip (
		for e.peek().t != tokRParen && e.peek().t != tokEOF {
			if e.peek().t == tokIdent {
				params = append(params, e.advance().v)
			}
			// skip default values like = 0
			if e.peek().t == tokAssign {
				e.advance()
				e.expr() // skip default value
			}
			if e.peek().t == tokComma {
				e.advance()
			}
		}
		e.advance() // skip )
	} else if e.peek().t == tokIdent {
		params = append(params, e.advance().v)
	}
	return params
}

func (e *jsEval) skipBalanced(open, close tokType) {
	depth := 1
	e.advance() // skip opening
	for depth > 0 && e.pos < len(e.tokens) {
		t := e.advance()
		if t.t == open {
			depth++
		} else if t.t == close {
			depth--
		}
	}
}

func (e *jsEval) primary() *jsValue {
	t := e.peek()

	switch t.t {
	case tokStr:
		e.advance()
		return jvStr(t.v)

	case tokNum:
		e.advance()
		return jvNum(t.n)

	case tokLParen:
		// Check if this is an arrow function: () => ... or (x) => ... or (x, y) => ...
		if e.isArrowFunction() {
			return e.parseArrowFunction()
		}
		e.advance()
		val := e.expr()
		e.expect(tokRParen)
		return val

	case tokLBrack:
		return e.parseArray()

	case tokLBrace:
		return e.parseObject()

	case tokIdent:
		switch t.v {
		case "true":
			e.advance()
			return jvTrue
		case "false":
			e.advance()
			return jvFalse
		case "null":
			e.advance()
			return jvNull
		case "undefined":
			e.advance()
			return jvUndefined
		case "h":
			e.advance()
			return e.evalHCall()
		case "Array":
			e.advance()
			// Array.isArray(x)
			if e.peek().t == tokDot {
				e.advance()
				method := e.advance()
				if method.v == "isArray" && e.peek().t == tokLParen {
					e.advance() // (
					arg := e.expr()
					e.expect(tokRParen)
					return jvBool(arg.typ == jsTypeArray)
				}
			}
			return jvUndefined
		case "Object":
			e.advance()
			if e.peek().t == tokDot {
				e.advance()
				method := e.advance()
				if method.v == "keys" && e.peek().t == tokLParen {
					e.advance()
					arg := e.expr()
					e.expect(tokRParen)
					if arg.typ == jsTypeObject && arg.object != nil {
						keys := make([]*jsValue, 0, len(arg.object))
						for k := range arg.object {
							keys = append(keys, jvStr(k))
						}
						return jvArr(keys)
					}
					return jvArr(nil)
				}
			}
			return jvUndefined
		case "JSON":
			e.advance()
			if e.peek().t == tokDot {
				e.advance()
				method := e.advance()
				if method.v == "stringify" && e.peek().t == tokLParen {
					e.advance()
					arg := e.expr()
					e.expect(tokRParen)
					b, _ := json.Marshal(jsValueToInterface(arg))
					return jvStr(string(b))
				}
			}
			return jvUndefined
		default:
			e.advance()
			// Look up in scope
			if val, ok := e.scope[t.v]; ok {
				return val
			}
			return jvUndefined
		}

	default:
		e.advance()
		return jvUndefined
	}
}

func (e *jsEval) evalHCall() *jsValue {
	// h(tag, props, ...children)
	e.expect(tokLParen)

	// Tag: string or identifier (component reference)
	var tag string
	var compFunc *jsValue

	if e.peek().t == tokStr {
		tag = e.advance().v
	} else if e.peek().t == tokIdent {
		ident := e.advance()
		// Check if it's a component function in scope
		if fn, ok := e.scope[ident.v]; ok && fn.typ == jsTypeFunc {
			compFunc = fn
		} else if fn, ok := e.scope[ident.v]; ok && fn.typ == jsTypeVNode {
			// component returned a vnode
			tag = "div"
		} else {
			tag = ident.v // unknown component — render as-is
		}
	} else if e.peek().t == tokIdent && e.peek().v == "null" {
		e.advance()
		tag = "" // fragment
	}

	if e.peek().t == tokComma {
		e.advance()
	}

	// Props: null or object
	var props map[string]*jsValue
	if e.peek().t == tokIdent && e.peek().v == "null" {
		e.advance()
	} else if e.peek().t == tokLBrace {
		obj := e.parseObject()
		if obj.typ == jsTypeObject {
			props = obj.object
		}
	}

	// Children
	var children []*ssrNode
	for e.peek().t == tokComma {
		e.advance()
		if e.peek().t == tokRParen {
			break
		}
		child := e.expr()
		children = append(children, jsValueToNodes(child)...)
	}

	e.expect(tokRParen)

	// If this is a component function, call it
	if compFunc != nil && compFunc.typ == jsTypeFunc {
		callProps := make(map[string]*jsValue)
		if props != nil {
			for k, v := range props {
				callProps[k] = v
			}
		}
		// Add children as props.children if present
		if len(children) > 0 {
			childVals := make([]*jsValue, len(children))
			for i, c := range children {
				childVals[i] = jvNode(c)
			}
			callProps["children"] = jvArr(childVals)
		}

		result := e.callFunc(compFunc, callProps)
		if result.typ == jsTypeVNode && result.vnode != nil {
			return result
		}
		return jvUndefined
	}

	node := &ssrNode{
		Tag:      tag,
		Props:    props,
		Children: children,
	}

	return jvNode(node)
}

func (e *jsEval) callFunc(fn *jsValue, props map[string]*jsValue) *jsValue {
	if fn.typ != jsTypeFunc {
		return jvUndefined
	}

	childScope := make(map[string]*jsValue, len(e.scope)+len(props))
	for k, v := range e.scope {
		childScope[k] = v
	}
	// If function has destructured params like { data, params }
	if len(fn.fnParams) == 1 && strings.HasPrefix(fn.fnParams[0], "{") {
		// Destructured parameter — inject props directly
		for k, v := range props {
			childScope[k] = v
		}
	} else if len(fn.fnParams) > 0 {
		// Named parameter
		childScope[fn.fnParams[0]] = jvObj(props)
	}

	bodyTokens := jsTokenize(fn.fnBody)
	childEval := &jsEval{tokens: bodyTokens, pos: 0, scope: childScope}

	// Process statements: const declarations and return
	return childEval.evalStatements()
}

func (e *jsEval) evalStatements() *jsValue {
	for e.peek().t != tokEOF {
		t := e.peek()

		// const/let/var declaration
		if t.t == tokIdent && (t.v == "const" || t.v == "let" || t.v == "var") {
			e.advance()

			// Array destructuring: const [a, b] = expr
			if e.peek().t == tokLBrack {
				e.advance() // skip [
				var names []string
				for e.peek().t != tokRBrack && e.peek().t != tokEOF {
					if e.peek().t == tokIdent {
						names = append(names, e.advance().v)
					} else {
						e.advance() // skip commas etc
					}
				}
				e.expect(tokRBrack)
				e.expect(tokAssign)
				val := e.expr()
				// Destructure array
				if val.typ == jsTypeArray {
					for i, name := range names {
						if i < len(val.array) {
							e.scope[name] = val.array[i]
						} else {
							e.scope[name] = jvUndefined
						}
					}
				} else {
					for _, name := range names {
						e.scope[name] = jvUndefined
					}
				}
				if e.peek().t == tokSemi {
					e.advance()
				}
				continue
			}

			// Object destructuring: const { a, b } = expr
			if e.peek().t == tokLBrace {
				e.advance() // skip {
				var names []string
				for e.peek().t != tokRBrace && e.peek().t != tokEOF {
					if e.peek().t == tokIdent {
						names = append(names, e.advance().v)
					}
					if e.peek().t == tokComma {
						e.advance()
					}
				}
				e.expect(tokRBrace)
				e.expect(tokAssign)
				val := e.expr()
				if val.typ == jsTypeObject && val.object != nil {
					for _, name := range names {
						if v, ok := val.object[name]; ok {
							e.scope[name] = v
						} else {
							e.scope[name] = jvUndefined
						}
					}
				}
				if e.peek().t == tokSemi {
					e.advance()
				}
				continue
			}

			// Simple: const name = expr
			name := e.advance().v
			e.expect(tokAssign)
			val := e.expr()
			e.scope[name] = val
			if e.peek().t == tokSemi {
				e.advance()
			}
			continue
		}

		// return statement
		if t.t == tokIdent && t.v == "return" {
			e.advance()
			return e.expr()
		}

		// if statement
		if t.t == tokIdent && t.v == "if" {
			e.advance() // skip "if"
			e.expect(tokLParen)
			cond := e.expr()
			e.expect(tokRParen)

			if cond.truthy() {
				// Execute the if block
				if e.peek().t == tokLBrace {
					e.advance() // skip {
					result := e.evalBlock()
					if result != nil {
						return result // block had a return
					}
				}
				// Skip else if present
				if e.peek().t == tokIdent && e.peek().v == "else" {
					e.advance()
					if e.peek().t == tokLBrace {
						e.skipBalanced(tokLBrace, tokRBrace)
					} else if e.peek().t == tokIdent && e.peek().v == "if" {
						// else if — skip the entire if chain
						e.skipIfChain()
					}
				}
			} else {
				// Skip the if block
				if e.peek().t == tokLBrace {
					e.skipBalanced(tokLBrace, tokRBrace)
				}
				// Handle else
				if e.peek().t == tokIdent && e.peek().v == "else" {
					e.advance()
					if e.peek().t == tokIdent && e.peek().v == "if" {
						// else if — evaluate as new if
						continue // loop will pick up "if" next iteration
					}
					if e.peek().t == tokLBrace {
						e.advance() // skip {
						result := e.evalBlock()
						if result != nil {
							return result
						}
					}
				}
			}
			continue
		}

		// Bare expression statement (e.g., useEffect(...))
		if t.t == tokIdent {
			if val, ok := e.scope[t.v]; ok && val.typ == jsTypeFunc {
				// Evaluate the expression (will call the function)
				e.expr()
				if e.peek().t == tokSemi {
					e.advance()
				}
				continue
			}
		}

		// skip other tokens
		e.advance()
	}
	return jvUndefined
}

// evalBlock evaluates statements inside { } until the closing }.
// Returns non-nil if a return statement was encountered.
func (e *jsEval) evalBlock() *jsValue {
	for e.peek().t != tokRBrace && e.peek().t != tokEOF {
		t := e.peek()

		if t.t == tokIdent && t.v == "return" {
			e.advance()
			result := e.expr()
			// Skip to closing brace
			for e.peek().t != tokRBrace && e.peek().t != tokEOF {
				e.advance()
			}
			if e.peek().t == tokRBrace {
				e.advance()
			}
			return result
		}

		// Handle nested const, if, etc by delegating to evalStatements logic
		// For simplicity, just skip non-return statements
		if t.t == tokIdent && (t.v == "const" || t.v == "let" || t.v == "var") {
			e.advance()
			name := e.advance().v
			e.expect(tokAssign)
			val := e.expr()
			e.scope[name] = val
			if e.peek().t == tokSemi {
				e.advance()
			}
			continue
		}

		e.advance()
	}
	if e.peek().t == tokRBrace {
		e.advance()
	}
	return nil // no return in block
}

// skipIfChain skips an entire if/else if/else chain without evaluating.
func (e *jsEval) skipIfChain() {
	// Skip "if"
	e.advance()
	// Skip condition (...)
	if e.peek().t == tokLParen {
		e.skipBalanced(tokLParen, tokRParen)
	}
	// Skip block {...}
	if e.peek().t == tokLBrace {
		e.skipBalanced(tokLBrace, tokRBrace)
	}
	// Handle else
	if e.peek().t == tokIdent && e.peek().v == "else" {
		e.advance()
		if e.peek().t == tokIdent && e.peek().v == "if" {
			e.skipIfChain()
		} else if e.peek().t == tokLBrace {
			e.skipBalanced(tokLBrace, tokRBrace)
		}
	}
}

func jsValueToNodes(v *jsValue) []*ssrNode {
	if v == nil {
		return nil
	}
	switch v.typ {
	case jsTypeVNode:
		if v.vnode != nil {
			return []*ssrNode{v.vnode}
		}
		return nil
	case jsTypeString:
		if v.str == "" {
			return nil
		}
		return []*ssrNode{{Text: v.str, IsText: true}}
	case jsTypeNumber:
		return []*ssrNode{{Text: v.toStr(), IsText: true}}
	case jsTypeArray:
		var nodes []*ssrNode
		for _, item := range v.array {
			nodes = append(nodes, jsValueToNodes(item)...)
		}
		return nodes
	case jsTypeNull, jsTypeUndefined, jsTypeBool:
		if v.typ == jsTypeBool && v.bool {
			return []*ssrNode{{Text: "true", IsText: true}}
		}
		return nil
	}
	return nil
}

func (e *jsEval) parseArray() *jsValue {
	e.expect(tokLBrack)
	var items []*jsValue
	for e.peek().t != tokRBrack && e.peek().t != tokEOF {
		items = append(items, e.expr())
		if e.peek().t == tokComma {
			e.advance()
		}
	}
	e.expect(tokRBrack)
	return jvArr(items)
}

func (e *jsEval) parseObject() *jsValue {
	e.expect(tokLBrace)
	obj := make(map[string]*jsValue)
	for e.peek().t != tokRBrace && e.peek().t != tokEOF {
		// spread: ...expr
		if e.peek().t == tokSpread {
			e.advance()
			src := e.expr()
			if src.typ == jsTypeObject && src.object != nil {
				for k, v := range src.object {
					obj[k] = v
				}
			}
			if e.peek().t == tokComma {
				e.advance()
			}
			continue
		}

		// key: can be identifier or string
		var key string
		if e.peek().t == tokStr {
			key = e.advance().v
		} else if e.peek().t == tokIdent {
			key = e.advance().v
		} else {
			e.advance()
			continue
		}

		// Shorthand: { key } (no colon, value same as key name)
		if e.peek().t == tokComma || e.peek().t == tokRBrace {
			if val, ok := e.scope[key]; ok {
				obj[key] = val
			} else {
				obj[key] = jvUndefined
			}
			if e.peek().t == tokComma {
				e.advance()
			}
			continue
		}

		e.expect(tokColon)
		val := e.expr()
		obj[key] = val
		if e.peek().t == tokComma {
			e.advance()
		}
	}
	e.expect(tokRBrace)
	return jvObj(obj)
}

// ─── JSON → JSValue ─────────────────────────────────────────────

func jsonToJSValue(data json.RawMessage) *jsValue {
	if data == nil {
		return jvNull
	}
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return jvNull
	}
	return interfaceToJSValue(raw)
}

func interfaceToJSValue(v interface{}) *jsValue {
	if v == nil {
		return jvNull
	}
	switch val := v.(type) {
	case bool:
		return jvBool(val)
	case float64:
		return jvNum(val)
	case string:
		return jvStr(val)
	case []interface{}:
		arr := make([]*jsValue, len(val))
		for i, item := range val {
			arr[i] = interfaceToJSValue(item)
		}
		return jvArr(arr)
	case map[string]interface{}:
		obj := make(map[string]*jsValue, len(val))
		for k, item := range val {
			obj[k] = interfaceToJSValue(item)
		}
		return jvObj(obj)
	}
	return jvNull
}

func jsValueToInterface(v *jsValue) interface{} {
	switch v.typ {
	case jsTypeNull, jsTypeUndefined:
		return nil
	case jsTypeBool:
		return v.bool
	case jsTypeNumber:
		return v.num
	case jsTypeString:
		return v.str
	case jsTypeArray:
		arr := make([]interface{}, len(v.array))
		for i, item := range v.array {
			arr[i] = jsValueToInterface(item)
		}
		return arr
	case jsTypeObject:
		obj := make(map[string]interface{}, len(v.object))
		for k, item := range v.object {
			obj[k] = jsValueToInterface(item)
		}
		return obj
	}
	return nil
}
