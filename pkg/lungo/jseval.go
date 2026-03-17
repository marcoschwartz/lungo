package lungo

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"
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

// ─── Arrow Function Registry ────────────────────────────────────
// Stores captured arrow functions so they can be called later.

type arrowFunc struct {
	params  []string
	tokens  []tok
	isBlock bool
	scope   map[string]*jsValue
}

var (
	arrowRegistry   = make(map[int]*arrowFunc)
	arrowNextID     int
	arrowRegistryMu sync.Mutex
)

func registerArrow(af *arrowFunc) int {
	arrowRegistryMu.Lock()
	arrowNextID++
	id := arrowNextID
	arrowRegistry[id] = af
	arrowRegistryMu.Unlock()
	return id
}

func callArrow(id int, args []*jsValue, callerScope map[string]*jsValue) *jsValue {
	arrowRegistryMu.Lock()
	af, ok := arrowRegistry[id]
	arrowRegistryMu.Unlock()
	if !ok {
		return jvUndefined
	}

	// Build child scope from captured scope + caller scope + args
	childScope := make(map[string]*jsValue, len(callerScope)+len(af.params))
	for k, v := range af.scope {
		childScope[k] = v
	}
	for k, v := range callerScope {
		childScope[k] = v
	}
	for i, name := range af.params {
		if i < len(args) {
			childScope[name] = args[i]
		} else {
			childScope[name] = jvUndefined
		}
	}

	bodyTokens := make([]tok, len(af.tokens))
	copy(bodyTokens, af.tokens)
	ev := &jsEval{tokens: bodyTokens, pos: 0, scope: childScope}

	if af.isBlock {
		return ev.evalStatements()
	}
	return ev.expr()
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
	tokNullCoalesce // ??
	tokArrow        // =>
	tokAssign
	tokSpread       // ...
	tokTemplatePart // parts of template literals: `text${...}text`
	tokPlusPlus     // ++
	tokMinusMinus   // --
	tokPlusAssign   // +=
	tokMinusAssign  // -=
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
			// Capture entire template literal content as single token
			var sb strings.Builder
			for i < len(src) && src[i] != '`' {
				if src[i] == '\\' && i+1 < len(src) { i++; sb.WriteByte(src[i]) } else { sb.WriteByte(src[i]) }
				i++
			}
			if i < len(src) { i++ }
			raw := sb.String()
			if strings.Contains(raw, "${") {
				tokens = append(tokens, tok{t: tokTemplatePart, v: raw})
			} else {
				tokens = append(tokens, tok{t: tokStr, v: raw})
			}
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
			if i+1 < len(src) && src[i+1] == '+' {
				tokens = append(tokens, tok{t: tokPlusPlus})
				i++
			} else if i+1 < len(src) && src[i+1] == '=' {
				tokens = append(tokens, tok{t: tokPlusAssign})
				i++
			} else {
				tokens = append(tokens, tok{t: tokPlus})
			}
		case '-':
			if i+1 < len(src) && src[i+1] == '-' {
				tokens = append(tokens, tok{t: tokMinusMinus})
				i++
			} else if i+1 < len(src) && src[i+1] == '=' {
				tokens = append(tokens, tok{t: tokMinusAssign})
				i++
			} else {
				tokens = append(tokens, tok{t: tokMinus})
			}
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
			if i+1 < len(src) && src[i+1] == '?' {
				tokens = append(tokens, tok{t: tokNullCoalesce})
				i++
			} else {
				tokens = append(tokens, tok{t: tokQuestion})
			}
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
	val := e.nullishCoalesce()
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

func (e *jsEval) nullishCoalesce() *jsValue {
	val := e.logicalOr()
	for e.peek().t == tokNullCoalesce {
		e.advance()
		right := e.logicalOr()
		if val.typ == jsTypeNull || val.typ == jsTypeUndefined {
			val = right
		}
	}
	return val
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
	// Prefix ++/--
	if e.peek().t == tokPlusPlus {
		e.advance()
		name := e.advance().v
		if v, ok := e.scope[name]; ok {
			newVal := jvNum(v.toNum() + 1)
			e.scope[name] = newVal
			return newVal
		}
		return jvNum(1)
	}
	if e.peek().t == tokMinusMinus {
		e.advance()
		name := e.advance().v
		if v, ok := e.scope[name]; ok {
			newVal := jvNum(v.toNum() - 1)
			e.scope[name] = newVal
			return newVal
		}
		return jvNum(-1)
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
	e.advance() // skip (
	// Collect params
	var params []string
	for e.peek().t != tokRParen && e.peek().t != tokEOF {
		if e.peek().t == tokIdent {
			params = append(params, e.advance().v)
		} else {
			e.advance()
		}
	}
	e.expect(tokRParen)
	e.expect(tokArrow)

	// No-arg expression arrow: () => expr — evaluate immediately
	if len(params) == 0 && e.peek().t != tokLBrace {
		result := e.expr()
		return &jsValue{typ: jsTypeFunc, str: "__resolved", fnParams: nil, object: map[string]*jsValue{"__value": result}}
	}

	// All other arrows (with params, or no-arg block body) — capture for later call
	var bodyToks []tok
	isBlock := false
	if e.peek().t == tokLBrace {
		isBlock = true
		start := e.pos
		e.skipBalanced(tokLBrace, tokRBrace)
		// Copy tokens inside { } (excluding braces)
		if e.pos-start > 2 {
			bodyToks = make([]tok, e.pos-start-2)
			copy(bodyToks, e.tokens[start+1:e.pos-1])
		}
	} else {
		start := e.pos
		e.expr()
		bodyToks = make([]tok, e.pos-start)
		copy(bodyToks, e.tokens[start:e.pos])
	}
	bodyToks = append(bodyToks, tok{t: tokEOF})

	captured := &arrowFunc{params: params, tokens: bodyToks, isBlock: isBlock, scope: e.scope}
	arrowID := registerArrow(captured)
	return &jsValue{typ: jsTypeFunc, str: "__arrow", num: float64(arrowID)}
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
		// Lazy initializer: useState(() => expr) — call to get the value
		if initial.typ == jsTypeFunc && initial.str == "__resolved" && initial.object != nil {
			if v, ok := initial.object["__value"]; ok {
				initial = v
			}
		} else if initial.typ == jsTypeFunc && initial.str == "__arrow" {
			initial = callArrow(int(initial.num), nil, e.scope)
		} else if initial.typ == jsTypeFunc {
			initial = jvFalse
		}
		return jvArr([]*jsValue{initial, &jsValue{typ: jsTypeFunc, str: "__noop"}})
	}
	if fn.str == "__hook_useEffect" {
		return jvUndefined
	}
	if fn.str == "__hook_useRouter" {
		pathname := "/"
		if p, ok := e.scope["__ssr_pathname"]; ok && p.str != "" {
			pathname = p.str
		}
		return jvObj(map[string]*jsValue{
			"pathname": jvStr(pathname),
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
		// useMemo(() => expr, deps) — call the function to get the value
		if len(args) > 0 {
			memo := args[0]
			if memo.typ == jsTypeFunc && memo.str == "__resolved" && memo.object != nil {
				if v, ok := memo.object["__value"]; ok {
					return v
				}
			}
			if memo.typ == jsTypeFunc && memo.str == "__arrow" {
				return callArrow(int(memo.num), nil, e.scope)
			}
		}
		return jvUndefined
	}
	if fn.str == "__noop" || fn.str == "__resolved" {
		return jvUndefined
	}
	if fn.str == "__arrow" {
		return callArrow(int(fn.num), args, e.scope)
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
	case "find":
		return e.evalFind(val)
	case "findIndex":
		return e.evalFindIndex(val)
	case "some":
		return e.evalSomeEvery(val, "some")
	case "every":
		return e.evalSomeEvery(val, "every")
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
	case "padStart":
		targetLen := 0
		padStr := " "
		if e.peek().t != tokRParen {
			targetLen = int(e.expr().toNum())
			if e.peek().t == tokComma {
				e.advance()
				padStr = e.expr().toStr()
			}
		}
		e.expect(tokRParen)
		s := val.toStr()
		for len(s) < targetLen {
			s = padStr + s
		}
		return jvStr(s)
	case "padEnd":
		targetLen := 0
		padStr := " "
		if e.peek().t != tokRParen {
			targetLen = int(e.expr().toNum())
			if e.peek().t == tokComma {
				e.advance()
				padStr = e.expr().toStr()
			}
		}
		e.expect(tokRParen)
		s := val.toStr()
		for len(s) < targetLen {
			s = s + padStr
		}
		return jvStr(s)
	case "toFixed":
		digits := 0
		if e.peek().t != tokRParen {
			digits = int(e.expr().toNum())
		}
		e.expect(tokRParen)
		return jvStr(strconv.FormatFloat(val.toNum(), 'f', digits, 64))
	case "isArray":
		// Array.isArray(x)
		arg := e.expr()
		e.expect(tokRParen)
		return jvBool(arg.typ == jsTypeArray)

	// ── String methods ──────────────────────────────────────
	case "replace":
		search := e.expr()
		e.expect(tokComma)
		replacement := e.expr()
		e.expect(tokRParen)
		if val.typ == jsTypeString {
			return jvStr(strings.Replace(val.str, search.toStr(), replacement.toStr(), 1))
		}
		return val
	case "replaceAll":
		search := e.expr()
		e.expect(tokComma)
		replacement := e.expr()
		e.expect(tokRParen)
		if val.typ == jsTypeString {
			return jvStr(strings.ReplaceAll(val.str, search.toStr(), replacement.toStr()))
		}
		return val
	case "startsWith":
		prefix := e.expr()
		e.expect(tokRParen)
		if val.typ == jsTypeString {
			return jvBool(strings.HasPrefix(val.str, prefix.toStr()))
		}
		return jvFalse
	case "endsWith":
		suffix := e.expr()
		e.expect(tokRParen)
		if val.typ == jsTypeString {
			return jvBool(strings.HasSuffix(val.str, suffix.toStr()))
		}
		return jvFalse
	case "repeat":
		count := 0
		if e.peek().t != tokRParen {
			count = int(e.expr().toNum())
		}
		e.expect(tokRParen)
		if val.typ == jsTypeString && count > 0 {
			return jvStr(strings.Repeat(val.str, count))
		}
		return jvStr("")
	case "toLowerCase":
		e.expect(tokRParen)
		return jvStr(strings.ToLower(val.toStr()))
	case "toUpperCase":
		e.expect(tokRParen)
		return jvStr(strings.ToUpper(val.toStr()))
	case "charAt":
		idx := 0
		if e.peek().t != tokRParen {
			idx = int(e.expr().toNum())
		}
		e.expect(tokRParen)
		s := val.toStr()
		if idx >= 0 && idx < len(s) {
			return jvStr(string(s[idx]))
		}
		return jvStr("")
	case "indexOf":
		search := e.expr()
		e.expect(tokRParen)
		if val.typ == jsTypeString {
			return jvNum(float64(strings.Index(val.str, search.toStr())))
		}
		if val.typ == jsTypeArray {
			for i, item := range val.array {
				if jsStrictEqual(item, search) {
					return jvNum(float64(i))
				}
			}
			return jvNum(-1)
		}
		return jvNum(-1)
	case "lastIndexOf":
		search := e.expr()
		e.expect(tokRParen)
		if val.typ == jsTypeString {
			return jvNum(float64(strings.LastIndex(val.str, search.toStr())))
		}
		return jvNum(-1)
	case "substring":
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
		s := val.toStr()
		if start < 0 {
			start = 0
		}
		if end < 0 {
			end = len(s)
		}
		if start > len(s) {
			start = len(s)
		}
		if end > len(s) {
			end = len(s)
		}
		if start > end {
			start, end = end, start
		}
		return jvStr(s[start:end])
	case "trimStart", "trimLeft":
		e.expect(tokRParen)
		return jvStr(strings.TrimLeft(val.toStr(), " \t\n\r"))
	case "trimEnd", "trimRight":
		e.expect(tokRParen)
		return jvStr(strings.TrimRight(val.toStr(), " \t\n\r"))

	// ── Array methods ───────────────────────────────────────
	case "reduce":
		return e.evalReduce(val)
	case "concat":
		var result []*jsValue
		if val.typ == jsTypeArray {
			result = append(result, val.array...)
		}
		for e.peek().t != tokRParen {
			arg := e.expr()
			if arg.typ == jsTypeArray {
				result = append(result, arg.array...)
			} else {
				result = append(result, arg)
			}
			if e.peek().t == tokComma {
				e.advance()
			}
		}
		e.expect(tokRParen)
		return jvArr(result)
	case "reverse":
		e.expect(tokRParen)
		if val.typ == jsTypeArray {
			n := len(val.array)
			result := make([]*jsValue, n)
			for i, v := range val.array {
				result[n-1-i] = v
			}
			return jvArr(result)
		}
		return val
	case "sort":
		if e.peek().t == tokRParen {
			// No comparator — sort by string representation
			e.expect(tokRParen)
			if val.typ == jsTypeArray {
				result := make([]*jsValue, len(val.array))
				copy(result, val.array)
				sortJSValues(result)
				return jvArr(result)
			}
			return val
		}
		// With comparator callback — skip for now, sort by string
		e.skipBalanced(tokLParen, tokRParen)
		if val.typ == jsTypeArray {
			result := make([]*jsValue, len(val.array))
			copy(result, val.array)
			sortJSValues(result)
			return jvArr(result)
		}
		return val
	case "flat":
		depth := 1
		if e.peek().t != tokRParen {
			depth = int(e.expr().toNum())
		}
		e.expect(tokRParen)
		if val.typ == jsTypeArray {
			return jvArr(flattenArray(val.array, depth))
		}
		return val
	case "flatMap":
		mapped := e.evalMapFilter(val, "map")
		if mapped.typ == jsTypeArray {
			return jvArr(flattenArray(mapped.array, 1))
		}
		return mapped
	case "push":
		// Returns new length (arrays are immutable in SSR)
		count := len(val.array)
		for e.peek().t != tokRParen {
			e.expr()
			count++
			if e.peek().t == tokComma {
				e.advance()
			}
		}
		e.expect(tokRParen)
		return jvNum(float64(count))
	case "pop":
		e.expect(tokRParen)
		if val.typ == jsTypeArray && len(val.array) > 0 {
			return val.array[len(val.array)-1]
		}
		return jvUndefined
	case "shift":
		e.expect(tokRParen)
		if val.typ == jsTypeArray && len(val.array) > 0 {
			return val.array[0]
		}
		return jvUndefined
	case "length":
		// .length() — shouldn't be called as method but handle gracefully
		e.expect(tokRParen)
		if val.typ == jsTypeArray {
			return jvNum(float64(len(val.array)))
		}
		if val.typ == jsTypeString {
			return jvNum(float64(len(val.str)))
		}
		return jvNum(0)
	case "keys":
		// Object.keys() handled elsewhere, but arr.keys() returns indices
		e.expect(tokRParen)
		if val.typ == jsTypeObject && val.object != nil {
			keys := make([]*jsValue, 0, len(val.object))
			for k := range val.object {
				keys = append(keys, jvStr(k))
			}
			return jvArr(keys)
		}
		return jvArr(nil)
	case "values":
		e.expect(tokRParen)
		if val.typ == jsTypeObject && val.object != nil {
			vals := make([]*jsValue, 0, len(val.object))
			for _, v := range val.object {
				vals = append(vals, v)
			}
			return jvArr(vals)
		}
		return jvArr(nil)
	case "entries":
		e.expect(tokRParen)
		if val.typ == jsTypeObject && val.object != nil {
			entries := make([]*jsValue, 0, len(val.object))
			for k, v := range val.object {
				entries = append(entries, jvArr([]*jsValue{jvStr(k), v}))
			}
			return jvArr(entries)
		}
		return jvArr(nil)
	case "assign":
		// Object.assign(target, ...sources)
		target := val
		if target.typ != jsTypeObject {
			target = &jsValue{typ: jsTypeObject, object: make(map[string]*jsValue)}
		}
		for e.peek().t != tokRParen {
			src := e.expr()
			if src.typ == jsTypeObject && src.object != nil {
				for k, v := range src.object {
					target.object[k] = v
				}
			}
			if e.peek().t == tokComma {
				e.advance()
			}
		}
		e.expect(tokRParen)
		return target

	default:
		// Check if method is a callable property on the object
		if val.typ == jsTypeObject && val.object != nil {
			if fn, ok := val.object[method]; ok && fn.typ == jsTypeFunc {
				// ( already consumed by handleMethodCall — collect args and call
				var args []*jsValue
				for e.peek().t != tokRParen && e.peek().t != tokEOF {
					args = append(args, e.expr())
					if e.peek().t == tokComma {
						e.advance()
					}
				}
				e.expect(tokRParen)
				if fn.str == "__arrow" {
					return callArrow(int(fn.num), args, e.scope)
				}
				return jvUndefined
			}
		}
		// Unknown method — skip args and return undefined
		for e.peek().t != tokRParen && e.peek().t != tokEOF {
			e.expr()
			if e.peek().t == tokComma {
				e.advance()
			}
		}
		e.expect(tokRParen)
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

// captureArrowCallback parses an arrow function callback inside a method call
// (e.g. .find(p => ...) ) and returns the param name and prepared body tokens.
// Caller must have already consumed the opening ( of the method call.
func (e *jsEval) captureArrowCallback() (paramName string, bodyTokens []tok) {
	params := e.parseArrowParams()
	paramName = "item"
	if len(params) > 0 {
		paramName = params[0]
	}
	e.expect(tokArrow)

	// Capture body tokens until the closing ) of the method call
	bodyStart := e.pos
	hasBodyParen := e.peek().t == tokLParen
	hasBodyBrace := e.peek().t == tokLBrace
	if hasBodyParen {
		e.skipBalanced(tokLParen, tokRParen)
	} else if hasBodyBrace {
		e.skipBalanced(tokLBrace, tokRBrace)
	} else {
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
	e.expect(tokRParen) // close method call

	bodyTokens = make([]tok, bodyEnd-bodyStart)
	copy(bodyTokens, e.tokens[bodyStart:bodyEnd])

	if hasBodyParen && len(bodyTokens) >= 2 && bodyTokens[0].t == tokLParen {
		bodyTokens = bodyTokens[1 : len(bodyTokens)-1]
	}
	if hasBodyBrace && len(bodyTokens) >= 2 && bodyTokens[0].t == tokLBrace {
		bodyTokens = extractReturnFromBlock(bodyTokens)
	}
	bodyTokens = append(bodyTokens, tok{t: tokEOF})
	return
}

// evalFind handles array.find(item => condition)
func (e *jsEval) evalFind(val *jsValue) *jsValue {
	if val.typ != jsTypeArray {
		e.skipBalanced(tokLParen, tokRParen)
		return jvUndefined
	}

	paramName, bodyTokens := e.captureArrowCallback()

	for _, item := range val.array {
		childScope := getPooledScope(e.scope)
		childScope[paramName] = item
		childEval := &jsEval{tokens: bodyTokens, pos: 0, scope: childScope}
		result := childEval.expr()
		putPooledScope(childScope)
		if result.truthy() {
			return item
		}
	}
	return jvUndefined
}

// evalFindIndex handles array.findIndex(item => condition)
func (e *jsEval) evalFindIndex(val *jsValue) *jsValue {
	if val.typ != jsTypeArray {
		e.skipBalanced(tokLParen, tokRParen)
		return jvNum(-1)
	}

	paramName, bodyTokens := e.captureArrowCallback()

	for i, item := range val.array {
		childScope := getPooledScope(e.scope)
		childScope[paramName] = item
		childEval := &jsEval{tokens: bodyTokens, pos: 0, scope: childScope}
		result := childEval.expr()
		putPooledScope(childScope)
		if result.truthy() {
			return jvNum(float64(i))
		}
	}
	return jvNum(-1)
}

// evalSomeEvery handles array.some/every(item => condition)
func (e *jsEval) evalSomeEvery(val *jsValue, method string) *jsValue {
	if val.typ != jsTypeArray {
		e.skipBalanced(tokLParen, tokRParen)
		if method == "every" {
			return jvTrue
		}
		return jvFalse
	}

	paramName, bodyTokens := e.captureArrowCallback()

	for _, item := range val.array {
		childScope := getPooledScope(e.scope)
		childScope[paramName] = item
		childEval := &jsEval{tokens: bodyTokens, pos: 0, scope: childScope}
		result := childEval.expr()
		putPooledScope(childScope)
		if method == "some" && result.truthy() {
			return jvTrue
		}
		if method == "every" && !result.truthy() {
			return jvFalse
		}
	}
	if method == "some" {
		return jvFalse
	}
	return jvTrue
}

// evalReduce handles array.reduce((acc, item) => expr, initialValue)
func (e *jsEval) evalReduce(val *jsValue) *jsValue {
	if val.typ != jsTypeArray {
		for e.peek().t != tokRParen && e.peek().t != tokEOF {
			e.advance()
		}
		e.expect(tokRParen)
		return jvUndefined
	}

	// Parse arrow params: (acc, item) or (acc, item, index)
	params := e.parseArrowParams()
	e.expect(tokArrow)

	// Capture body tokens — read until comma at depth 0 (before initialValue) or )
	bodyStart := e.pos
	hasBodyParen := e.peek().t == tokLParen
	hasBodyBrace := e.peek().t == tokLBrace
	if hasBodyParen {
		e.skipBalanced(tokLParen, tokRParen)
	} else if hasBodyBrace {
		e.skipBalanced(tokLBrace, tokRBrace)
	} else {
		depth := 0
		for e.pos < len(e.tokens) {
			t := e.tokens[e.pos]
			if t.t == tokLParen {
				depth++
			} else if t.t == tokRParen {
				if depth == 0 {
					break
				}
				depth--
			} else if t.t == tokComma && depth == 0 {
				break
			}
			e.pos++
		}
	}
	bodyEnd := e.pos

	// Parse initial value if present
	var accumulator *jsValue
	if e.peek().t == tokComma {
		e.advance()
		accumulator = e.expr()
	}
	e.expect(tokRParen)

	bodyTokens := make([]tok, bodyEnd-bodyStart)
	copy(bodyTokens, e.tokens[bodyStart:bodyEnd])
	if hasBodyParen && len(bodyTokens) >= 2 && bodyTokens[0].t == tokLParen {
		bodyTokens = bodyTokens[1 : len(bodyTokens)-1]
	}
	if hasBodyBrace && len(bodyTokens) >= 2 && bodyTokens[0].t == tokLBrace {
		bodyTokens = extractReturnFromBlock(bodyTokens)
	}
	bodyTokens = append(bodyTokens, tok{t: tokEOF})

	arr := val.array
	startIdx := 0
	if accumulator == nil {
		if len(arr) == 0 {
			return jvUndefined
		}
		accumulator = arr[0]
		startIdx = 1
	}

	accParam := "acc"
	itemParam := "item"
	if len(params) > 0 {
		accParam = params[0]
	}
	if len(params) > 1 {
		itemParam = params[1]
	}

	for i := startIdx; i < len(arr); i++ {
		childScope := getPooledScope(e.scope)
		childScope[accParam] = accumulator
		childScope[itemParam] = arr[i]
		if len(params) > 2 {
			childScope[params[2]] = jvNum(float64(i))
		}
		childEval := &jsEval{tokens: bodyTokens, pos: 0, scope: childScope}
		accumulator = childEval.expr()
		putPooledScope(childScope)
	}

	return accumulator
}

func sortJSValues(arr []*jsValue) {
	// Simple insertion sort by string representation
	for i := 1; i < len(arr); i++ {
		key := arr[i]
		keyStr := key.toStr()
		j := i - 1
		for j >= 0 && arr[j].toStr() > keyStr {
			arr[j+1] = arr[j]
			j--
		}
		arr[j+1] = key
	}
}

func flattenArray(arr []*jsValue, depth int) []*jsValue {
	if depth <= 0 {
		return arr
	}
	var result []*jsValue
	for _, item := range arr {
		if item.typ == jsTypeArray {
			result = append(result, flattenArray(item.array, depth-1)...)
		} else {
			result = append(result, item)
		}
	}
	return result
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

// evalSingleStatement handles a single statement (like "return expr;") without braces.
func (e *jsEval) evalSingleStatement() *jsValue {
	if e.peek().t == tokIdent && e.peek().v == "return" {
		e.advance() // skip "return"
		val := e.expr()
		// Skip optional semicolon
		if e.peek().t == tokSemi {
			e.advance()
		}
		return val
	}
	// Not a return — evaluate and discard
	e.expr()
	if e.peek().t == tokSemi {
		e.advance()
	}
	return nil
}

// skipSingleStatement skips a single statement without braces (e.g., "return expr;").
func (e *jsEval) skipSingleStatement() {
	depth := 0
	first := true
	for e.peek().t != tokEOF {
		t := e.peek()
		if t.t == tokLParen || t.t == tokLBrace || t.t == tokLBrack {
			depth++
		} else if t.t == tokRParen || t.t == tokRBrace || t.t == tokRBrack {
			depth--
		}
		if depth <= 0 && t.t == tokSemi {
			e.advance() // consume semicolon
			return
		}
		// After the first token, stop at statement keywords (don't consume)
		if !first && depth <= 0 && t.t == tokIdent && (t.v == "return" || t.v == "const" || t.v == "let" || t.v == "var" || t.v == "if" || t.v == "else" || t.v == "for") {
			return
		}
		first = false
		e.advance()
	}
}

func (e *jsEval) primary() *jsValue {
	t := e.peek()

	switch t.t {
	case tokStr:
		e.advance()
		return jvStr(t.v)

	case tokTemplatePart:
		// Template literal with ${} interpolation — raw content stored in token
		raw := e.advance().v
		var sb strings.Builder
		i := 0
		for i < len(raw) {
			if i+1 < len(raw) && raw[i] == '$' && raw[i+1] == '{' {
				i += 2
				// Find matching }
				depth := 1
				start := i
				for i < len(raw) && depth > 0 {
					if raw[i] == '{' { depth++ } else if raw[i] == '}' { depth-- }
					if depth > 0 { i++ }
				}
				exprStr := raw[start:i]
				if i < len(raw) { i++ } // skip }
				// Evaluate the expression
				exprTokens := jsTokenize(exprStr)
				exprEv := &jsEval{tokens: exprTokens, pos: 0, scope: e.scope}
				val := exprEv.expr()
				sb.WriteString(val.toStr())
			} else {
				sb.WriteByte(raw[i])
				i++
			}
		}
		return jvStr(sb.String())

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
		case "new":
			e.advance()
			ctor := e.advance().v
			if e.peek().t == tokLParen {
				e.advance()
				for e.peek().t != tokRParen && e.peek().t != tokEOF {
					e.expr()
					if e.peek().t == tokComma {
						e.advance()
					}
				}
				e.expect(tokRParen)
				if ctor == "Date" {
					noopStr := func(s string) *jsValue {
						id := registerArrow(&arrowFunc{
							tokens: append(jsTokenize(`"`+s+`"`), tok{t: tokEOF}),
							scope:  make(map[string]*jsValue),
						})
						return &jsValue{typ: jsTypeFunc, str: "__arrow", num: float64(id)}
					}
					noopNum := func(n float64) *jsValue {
						id := registerArrow(&arrowFunc{
							tokens: append(jsTokenize(strconv.FormatFloat(n, 'f', -1, 64)), tok{t: tokEOF}),
							scope:  make(map[string]*jsValue),
						})
						return &jsValue{typ: jsTypeFunc, str: "__arrow", num: float64(id)}
					}
					return jvObj(map[string]*jsValue{
						"toLocaleTimeString": noopStr("00:00:00"),
						"toLocaleDateString": noopStr("1/1/2026"),
						"toISOString":        noopStr("2026-01-01T00:00:00.000Z"),
						"toString":           noopStr("Thu Jan 01 2026"),
						"getTime":            noopNum(0),
						"getFullYear":        noopNum(2026),
						"getMonth":           noopNum(0),
						"getDate":            noopNum(1),
						"getHours":           noopNum(0),
						"getMinutes":         noopNum(0),
						"getSeconds":         noopNum(0),
					})
				}
			}
			return jvUndefined
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
			if e.peek().t == tokDot {
				e.advance()
				method := e.advance()
				if e.peek().t == tokLParen {
					e.advance()
					arg := e.expr()
					e.expect(tokRParen)
					switch method.v {
					case "isArray":
						return jvBool(arg.typ == jsTypeArray)
					case "from":
						if arg.typ == jsTypeArray { return arg }
						if arg.typ == jsTypeString {
							arr := make([]*jsValue, len(arg.str))
							for i, ch := range arg.str { arr[i] = jvStr(string(ch)) }
							return jvArr(arr)
						}
						return jvArr(nil)
					}
				}
			}
			return jvUndefined
		case "Object":
			e.advance()
			if e.peek().t == tokDot {
				e.advance()
				method := e.advance()
				if e.peek().t == tokLParen {
					e.advance()
					arg := e.expr()
					// Check for second arg (Object.assign)
					var extraArgs []*jsValue
					for e.peek().t == tokComma {
						e.advance()
						extraArgs = append(extraArgs, e.expr())
					}
					e.expect(tokRParen)
					switch method.v {
					case "keys":
						if arg.typ == jsTypeObject && arg.object != nil {
							keys := make([]*jsValue, 0, len(arg.object))
							for k := range arg.object {
								keys = append(keys, jvStr(k))
							}
							return jvArr(keys)
						}
						return jvArr(nil)
					case "values":
						if arg.typ == jsTypeObject && arg.object != nil {
							vals := make([]*jsValue, 0, len(arg.object))
							for _, v := range arg.object {
								vals = append(vals, v)
							}
							return jvArr(vals)
						}
						return jvArr(nil)
					case "entries":
						if arg.typ == jsTypeObject && arg.object != nil {
							entries := make([]*jsValue, 0, len(arg.object))
							for k, v := range arg.object {
								entries = append(entries, jvArr([]*jsValue{jvStr(k), v}))
							}
							return jvArr(entries)
						}
						return jvArr(nil)
					case "assign":
						target := arg
						if target.typ != jsTypeObject || target.object == nil {
							target = &jsValue{typ: jsTypeObject, object: make(map[string]*jsValue)}
						}
						for _, src := range extraArgs {
							if src.typ == jsTypeObject && src.object != nil {
								for k, v := range src.object {
									target.object[k] = v
								}
							}
						}
						return target
					case "freeze":
						return arg // no-op in SSR
					}
				}
			}
			return jvUndefined
		case "JSON":
			e.advance()
			if e.peek().t == tokDot {
				e.advance()
				method := e.advance()
				if e.peek().t == tokLParen {
					e.advance()
					arg := e.expr()
					e.expect(tokRParen)
					switch method.v {
					case "stringify":
						b, _ := json.Marshal(jsValueToInterface(arg))
						return jvStr(string(b))
					case "parse":
						return jsonToJSValue(json.RawMessage(arg.toStr()))
					}
				}
			}
			return jvUndefined
		case "Number":
			e.advance()
			if e.peek().t == tokLParen {
				e.advance()
				arg := e.expr()
				e.expect(tokRParen)
				return jvNum(arg.toNum())
			}
			return jvUndefined
		case "parseInt":
			e.advance()
			if e.peek().t == tokLParen {
				e.advance()
				arg := e.expr()
				// Optional radix
				if e.peek().t == tokComma { e.advance(); e.expr() }
				e.expect(tokRParen)
				n, err := strconv.ParseInt(strings.TrimSpace(arg.toStr()), 10, 64)
				if err != nil { return jvNum(0) }
				return jvNum(float64(n))
			}
			return jvUndefined
		case "parseFloat":
			e.advance()
			if e.peek().t == tokLParen {
				e.advance()
				arg := e.expr()
				e.expect(tokRParen)
				n, err := strconv.ParseFloat(strings.TrimSpace(arg.toStr()), 64)
				if err != nil { return jvNum(0) }
				return jvNum(n)
			}
			return jvUndefined
		case "Boolean":
			e.advance()
			if e.peek().t == tokLParen {
				e.advance()
				arg := e.expr()
				e.expect(tokRParen)
				return jvBool(arg.truthy())
			}
			return jvUndefined
		case "console":
			e.advance()
			if e.peek().t == tokDot {
				e.advance(); e.advance() // skip .method
				if e.peek().t == tokLParen { e.skipBalanced(tokLParen, tokRParen) }
			}
			return jvUndefined
		case "Math":
			e.advance()
			if e.peek().t == tokDot {
				e.advance()
				method := e.advance().v
				if e.peek().t == tokLParen {
					e.advance()
					arg := e.expr()
					n := arg.toNum()
					switch method {
					case "floor":
						e.expect(tokRParen)
						return jvNum(float64(int64(n)))
					case "ceil":
						e.expect(tokRParen)
						if n == float64(int64(n)) {
							return jvNum(n)
						}
						return jvNum(float64(int64(n) + 1))
					case "round":
						e.expect(tokRParen)
						return jvNum(float64(int64(n + 0.5)))
					case "abs":
						e.expect(tokRParen)
						if n < 0 {
							return jvNum(-n)
						}
						return jvNum(n)
					case "min":
						if e.peek().t == tokComma {
							e.advance()
							b := e.expr().toNum()
							e.expect(tokRParen)
							if n < b {
								return jvNum(n)
							}
							return jvNum(b)
						}
						e.expect(tokRParen)
						return jvNum(n)
					case "max":
						if e.peek().t == tokComma {
							e.advance()
							b := e.expr().toNum()
							e.expect(tokRParen)
							if n > b {
								return jvNum(n)
							}
							return jvNum(b)
						}
						e.expect(tokRParen)
						return jvNum(n)
					case "random":
						e.expect(tokRParen)
						return jvNum(0)
					}
				}
			}
			return jvUndefined
		case "String":
			e.advance()
			if e.peek().t == tokLParen {
				e.advance()
				arg := e.expr()
				e.expect(tokRParen)
				return jvStr(arg.toStr())
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
	// If function has destructured params like { data, params, max = 100 }
	if len(fn.fnParams) == 1 && strings.HasPrefix(fn.fnParams[0], "{") {
		// Inject props
		for k, v := range props {
			childScope[k] = v
		}
		// Parse default values from the param string
		paramStr := fn.fnParams[0]
		paramStr = strings.TrimPrefix(paramStr, "{")
		paramStr = strings.TrimSuffix(strings.TrimSpace(paramStr), "}")
		for _, part := range strings.Split(paramStr, ",") {
			part = strings.TrimSpace(part)
			if eqIdx := strings.Index(part, "="); eqIdx > 0 {
				name := strings.TrimSpace(part[:eqIdx])
				if _, exists := childScope[name]; !exists {
					// Apply default value
					defaultStr := strings.TrimSpace(part[eqIdx+1:])
					defaultVal := jsEvalExpr(defaultStr, childScope)
					childScope[name] = defaultVal
				}
			}
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
				} else if e.peek().t == tokIdent && e.peek().v == "return" {
					// Single-statement if: if (cond) return expr;
					result := e.evalSingleStatement()
					if result != nil {
						return result
					}
				}
				// Skip else if present
				if e.peek().t == tokIdent && e.peek().v == "else" {
					e.advance()
					if e.peek().t == tokLBrace {
						e.skipBalanced(tokLBrace, tokRBrace)
					} else if e.peek().t == tokIdent && e.peek().v == "if" {
						e.skipIfChain()
					} else {
						// else single-statement — skip it
						e.skipSingleStatement()
					}
				}
			} else {
				// Skip the if block
				if e.peek().t == tokLBrace {
					e.skipBalanced(tokLBrace, tokRBrace)
				} else {
					// Skip single-statement if body
					e.skipSingleStatement()
				}
				// Handle else
				if e.peek().t == tokIdent && e.peek().v == "else" {
					e.advance()
					if e.peek().t == tokIdent && e.peek().v == "if" {
						continue // loop will pick up "if" next iteration
					}
					if e.peek().t == tokLBrace {
						e.advance() // skip {
						result := e.evalBlock()
						if result != nil {
							return result
						}
					} else if e.peek().t == tokIdent && e.peek().v == "return" {
						result := e.evalSingleStatement()
						if result != nil {
							return result
						}
					}
				}
			}
			continue
		}

		// for statement
		if t.t == tokIdent && t.v == "for" {
			e.advance() // skip "for"
			e.expect(tokLParen)
			// Check for for...of: for (const x of arr)
			if e.peek().t == tokIdent && (e.peek().v == "const" || e.peek().v == "let" || e.peek().v == "var") {
				e.advance() // skip const/let/var
				varName := e.advance().v // variable name
				if e.peek().t == tokIdent && e.peek().v == "of" {
					e.advance() // skip "of"
					arr := e.expr()
					e.expect(tokRParen)
					// Capture body, then execute for each item
					if e.peek().t == tokLBrace {
						bodyStart := e.pos
						e.skipBalanced(tokLBrace, tokRBrace)
						bodyEnd := e.pos
						if arr.typ == jsTypeArray {
							for _, item := range arr.array {
								e.scope[varName] = item
								bodyTokens := make([]tok, bodyEnd-bodyStart)
								copy(bodyTokens, e.tokens[bodyStart:bodyEnd])
								if len(bodyTokens) >= 2 && bodyTokens[0].t == tokLBrace {
									bodyTokens = bodyTokens[1 : len(bodyTokens)-1]
								}
								bodyTokens = append(bodyTokens, tok{t: tokEOF})
								bodyEv := &jsEval{tokens: bodyTokens, pos: 0, scope: e.scope}
								result := bodyEv.evalStatements()
								if result != nil { return result }
							}
						}
					}
					continue
				}
				// Regular for (let i = 0; ...; ...)
				// Init: already consumed const/let/var and name
				e.expect(tokAssign)
				initVal := e.expr()
				e.scope[varName] = initVal
			} else {
				// for (; ...; ...) or for (expr; ...; ...)
				if e.peek().t != tokSemi {
					e.expr()
				}
			}
			e.expect(tokSemi)
			// Capture condition tokens
			condStart := e.pos
			if e.peek().t != tokSemi {
				e.expr() // evaluate once to skip
			}
			condEnd := e.pos
			e.expect(tokSemi)
			// Capture update tokens
			updateStart := e.pos
			if e.peek().t != tokRParen {
				e.expr() // skip
			}
			updateEnd := e.pos
			e.expect(tokRParen)
			// Capture body
			if e.peek().t == tokLBrace {
				bodyStart := e.pos
				e.skipBalanced(tokLBrace, tokRBrace)
				bodyEnd := e.pos
				// Execute loop (max 10000 iterations for safety)
				for iter := 0; iter < 10000; iter++ {
					// Evaluate condition
					condTokens := make([]tok, condEnd-condStart)
					copy(condTokens, e.tokens[condStart:condEnd])
					condTokens = append(condTokens, tok{t: tokEOF})
					condEv := &jsEval{tokens: condTokens, pos: 0, scope: e.scope}
					if !condEv.expr().truthy() {
						break
					}
					// Execute body
					bodyTokens := make([]tok, bodyEnd-bodyStart)
					copy(bodyTokens, e.tokens[bodyStart:bodyEnd])
					// Strip outer braces
					if len(bodyTokens) >= 2 && bodyTokens[0].t == tokLBrace {
						bodyTokens = bodyTokens[1 : len(bodyTokens)-1]
					}
					bodyTokens = append(bodyTokens, tok{t: tokEOF})
					bodyEv := &jsEval{tokens: bodyTokens, pos: 0, scope: e.scope}
					result := bodyEv.evalStatements()
					if result != nil {
						return result
					}
					// Execute update
					updateTokens := make([]tok, updateEnd-updateStart)
					copy(updateTokens, e.tokens[updateStart:updateEnd])
					updateTokens = append(updateTokens, tok{t: tokEOF})
					updateEv := &jsEval{tokens: updateTokens, pos: 0, scope: e.scope}
					updateEv.expr()
				}
			}
			continue
		}

		// while statement
		if t.t == tokIdent && t.v == "while" {
			e.advance() // skip "while"
			e.expect(tokLParen)
			condStart := e.pos
			e.expr()
			condEnd := e.pos
			e.expect(tokRParen)
			if e.peek().t == tokLBrace {
				bodyStart := e.pos
				e.skipBalanced(tokLBrace, tokRBrace)
				bodyEnd := e.pos
				for iter := 0; iter < 10000; iter++ {
					condTokens := make([]tok, condEnd-condStart)
					copy(condTokens, e.tokens[condStart:condEnd])
					condTokens = append(condTokens, tok{t: tokEOF})
					condEv := &jsEval{tokens: condTokens, pos: 0, scope: e.scope}
					if !condEv.expr().truthy() {
						break
					}
					bodyTokens := make([]tok, bodyEnd-bodyStart)
					copy(bodyTokens, e.tokens[bodyStart:bodyEnd])
					if len(bodyTokens) >= 2 && bodyTokens[0].t == tokLBrace {
						bodyTokens = bodyTokens[1 : len(bodyTokens)-1]
					}
					bodyTokens = append(bodyTokens, tok{t: tokEOF})
					bodyEv := &jsEval{tokens: bodyTokens, pos: 0, scope: e.scope}
					result := bodyEv.evalStatements()
					if result != nil {
						return result
					}
				}
			}
			continue
		}

		// try/catch — execute try block, ignore errors
		if t.t == tokIdent && t.v == "try" {
			e.advance() // skip "try"
			if e.peek().t == tokLBrace {
				e.advance() // skip {
				result := e.evalBlock()
				if result != nil {
					// Skip catch/finally
					if e.peek().t == tokIdent && e.peek().v == "catch" {
						e.advance()
						if e.peek().t == tokLParen { e.skipBalanced(tokLParen, tokRParen) }
						if e.peek().t == tokLBrace { e.skipBalanced(tokLBrace, tokRBrace) }
					}
					if e.peek().t == tokIdent && e.peek().v == "finally" {
						e.advance()
						if e.peek().t == tokLBrace { e.advance(); e.evalBlock() }
					}
					return result
				}
			}
			// Skip catch
			if e.peek().t == tokIdent && e.peek().v == "catch" {
				e.advance()
				if e.peek().t == tokLParen { e.skipBalanced(tokLParen, tokRParen) }
				if e.peek().t == tokLBrace { e.skipBalanced(tokLBrace, tokRBrace) }
			}
			// Execute finally if present
			if e.peek().t == tokIdent && e.peek().v == "finally" {
				e.advance()
				if e.peek().t == tokLBrace { e.advance(); e.evalBlock() }
			}
			continue
		}

		// console.log/warn/error — no-op in SSR
		if t.t == tokIdent && t.v == "console" {
			e.advance()
			if e.peek().t == tokDot {
				e.advance() // skip .
				e.advance() // skip method name
				if e.peek().t == tokLParen {
					// Skip all arguments
					e.skipBalanced(tokLParen, tokRParen)
				}
			}
			if e.peek().t == tokSemi { e.advance() }
			continue
		}

		// Identifier — could be assignment, postfix ++/--, or function call
		if t.t == tokIdent {
			name := t.v
			// Check for postfix ++/--
			if e.pos+1 < len(e.tokens) && e.tokens[e.pos+1].t == tokPlusPlus {
				e.advance(); e.advance() // skip name, ++
				if v, ok := e.scope[name]; ok {
					e.scope[name] = jvNum(v.toNum() + 1)
				}
				if e.peek().t == tokSemi { e.advance() }
				continue
			}
			if e.pos+1 < len(e.tokens) && e.tokens[e.pos+1].t == tokMinusMinus {
				e.advance(); e.advance()
				if v, ok := e.scope[name]; ok {
					e.scope[name] = jvNum(v.toNum() - 1)
				}
				if e.peek().t == tokSemi { e.advance() }
				continue
			}
			// Check for += / -=
			if e.pos+1 < len(e.tokens) && e.tokens[e.pos+1].t == tokPlusAssign {
				e.advance(); e.advance()
				val := e.expr()
				if v, ok := e.scope[name]; ok {
					if v.typ == jsTypeString || val.typ == jsTypeString {
						e.scope[name] = jvStr(v.toStr() + val.toStr())
					} else {
						e.scope[name] = jvNum(v.toNum() + val.toNum())
					}
				}
				if e.peek().t == tokSemi { e.advance() }
				continue
			}
			if e.pos+1 < len(e.tokens) && e.tokens[e.pos+1].t == tokMinusAssign {
				e.advance(); e.advance()
				val := e.expr()
				if v, ok := e.scope[name]; ok {
					e.scope[name] = jvNum(v.toNum() - val.toNum())
				}
				if e.peek().t == tokSemi { e.advance() }
				continue
			}
			// Check for simple reassignment: name = expr
			if e.pos+1 < len(e.tokens) && e.tokens[e.pos+1].t == tokAssign {
				e.advance(); e.advance()
				e.scope[name] = e.expr()
				if e.peek().t == tokSemi { e.advance() }
				continue
			}
			// Function call or other expression
			e.expr()
			if e.peek().t == tokSemi { e.advance() }
			continue
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
		// Empty strings produce no DOM text node, so skip them in SSR.
		// The hydrator handles the child count mismatch gracefully.
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
