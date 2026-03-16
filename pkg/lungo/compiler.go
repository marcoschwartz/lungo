package lungo

import (
	"strconv"
	"strings"
)

// compilePageTokens compiles a pre-tokenized function body into a compiledPage.
// Returns nil if compilation fails (caller falls back to interpreted eval).
func compilePageTokens(tokens []tok, localFuncs map[string]*jsValue) *compiledPage {
	c := &compiler{tokens: tokens, pos: 0, funcs: localFuncs}
	defer func() {
		if r := recover(); r != nil {
			// compilation failed — will fall back to interpreted
		}
	}()
	return c.compilePage()
}

type compiler struct {
	tokens []tok
	pos    int
	funcs  map[string]*jsValue // local component functions
}

func (c *compiler) peek() tok {
	if c.pos < len(c.tokens) {
		return c.tokens[c.pos]
	}
	return tok{t: tokEOF}
}

func (c *compiler) peekAt(offset int) tok {
	idx := c.pos + offset
	if idx < len(c.tokens) {
		return c.tokens[idx]
	}
	return tok{t: tokEOF}
}

func (c *compiler) advance() tok {
	t := c.peek()
	if c.pos < len(c.tokens) {
		c.pos++
	}
	return t
}

func (c *compiler) expect(t tokType) {
	c.advance()
}

func (c *compiler) compilePage() *compiledPage {
	page := &compiledPage{}

	for c.peek().t != tokEOF {
		t := c.peek()

		// const/let/var
		if t.t == tokIdent && (t.v == "const" || t.v == "let" || t.v == "var") {
			c.advance()

			// Array destructuring
			if c.peek().t == tokLBrack {
				c.advance()
				var names []string
				for c.peek().t != tokRBrack && c.peek().t != tokEOF {
					if c.peek().t == tokIdent {
						names = append(names, c.advance().v)
					} else {
						c.advance()
					}
				}
				c.expect(tokRBrack)
				c.expect(tokAssign)
				expr := c.compileExpr()
				page.Preamble = append(page.Preamble, compiledStmt{
					IsArrayDestructure: true,
					Names:              names,
					Expr:               expr,
				})
				if c.peek().t == tokSemi {
					c.advance()
				}
				continue
			}

			name := c.advance().v
			c.expect(tokAssign)
			expr := c.compileExpr()
			page.Preamble = append(page.Preamble, compiledStmt{Name: name, Expr: expr})
			if c.peek().t == tokSemi {
				c.advance()
			}
			continue
		}

		// return
		if t.t == tokIdent && t.v == "return" {
			c.advance()
			// Try to compile as a direct node tree first (skip parens if present)
			savedPos := c.pos
			if c.peek().t == tokLParen {
				c.advance()
			}
			if c.peek().t == tokIdent && c.peek().v == "h" {
				node := c.compileHCallAsNode()
				if node != nil {
					if c.peek().t == tokRParen {
						c.advance()
					}
					page.ReturnNode = node
					page.ReturnExpr = nil // not needed
					return page
				}
			}
			// Fallback to expression
			c.pos = savedPos
			page.ReturnExpr = c.compileExpr()
			return page
		}

		// if statement
		if t.t == tokIdent && t.v == "if" {
			c.advance() // skip "if"
			c.expect(tokLParen)
			cond := c.compileExpr()
			c.expect(tokRParen)

			// Compile if body
			var ifBody *compiledPage
			if c.peek().t == tokLBrace {
				ifBody = c.compileBlock()
			}

			// Compile else body
			var elseBody *compiledPage
			if c.peek().t == tokIdent && c.peek().v == "else" {
				c.advance()
				if c.peek().t == tokIdent && c.peek().v == "if" {
					// else if — wrap in a new compiledPage with one if stmt
					// (recursive)
					continue // let the loop pick up "if" next
				}
				if c.peek().t == tokLBrace {
					elseBody = c.compileBlock()
				}
			}

			page.Preamble = append(page.Preamble, compiledStmt{
				IsIf:      true,
				Condition: cond,
				IfBody:    ifBody,
				ElseBody:  elseBody,
			})
			continue
		}

		// Bare function calls (useEffect, etc.) — skip
		if t.t == tokIdent {
			// Try to skip the expression
			c.compileExpr()
			if c.peek().t == tokSemi {
				c.advance()
			}
			continue
		}

		c.advance()
	}
	return nil
}

// compileBlock compiles a { ... } block into a compiledPage (handles const, return, if)
func (c *compiler) compileBlock() *compiledPage {
	c.expect(tokLBrace)
	block := &compiledPage{}

	for c.peek().t != tokRBrace && c.peek().t != tokEOF {
		t := c.peek()

		// return in block
		if t.t == tokIdent && t.v == "return" {
			c.advance()
			block.ReturnExpr = c.compileExpr()
			if c.peek().t == tokSemi {
				c.advance()
			}
			// Skip rest of block
			for c.peek().t != tokRBrace && c.peek().t != tokEOF {
				c.advance()
			}
			break
		}

		// const/let/var
		if t.t == tokIdent && (t.v == "const" || t.v == "let" || t.v == "var") {
			c.advance()
			name := c.advance().v
			c.expect(tokAssign)
			expr := c.compileExpr()
			block.Preamble = append(block.Preamble, compiledStmt{Name: name, Expr: expr})
			if c.peek().t == tokSemi {
				c.advance()
			}
			continue
		}

		// Skip other tokens
		c.advance()
	}
	c.expect(tokRBrace)
	return block
}

// ─── Expression Compiler ────────────────────────────────────────

func (c *compiler) compileExpr() compiledExpr {
	return c.compileTernary()
}

func (c *compiler) compileTernary() compiledExpr {
	left := c.compileLogicalOr()
	if c.peek().t == tokQuestion {
		c.advance()
		consequent := c.compileExpr()
		if c.peek().t == tokColon {
			c.advance()
			alternate := c.compileExpr()
			return func(scope map[string]*jsValue) *jsValue {
				if left(scope).truthy() {
					return consequent(scope)
				}
				return alternate(scope)
			}
		}
		// Missing colon (transpiler bug workaround)
		return func(scope map[string]*jsValue) *jsValue {
			if left(scope).truthy() {
				return consequent(scope)
			}
			return jvNull
		}
	}
	return left
}

func (c *compiler) compileLogicalOr() compiledExpr {
	left := c.compileLogicalAnd()
	for c.peek().t == tokOr {
		c.advance()
		right := c.compileLogicalAnd()
		prev := left
		left = func(scope map[string]*jsValue) *jsValue {
			l := prev(scope)
			if l.truthy() {
				return l
			}
			return right(scope)
		}
	}
	return left
}

func (c *compiler) compileLogicalAnd() compiledExpr {
	left := c.compileEquality()
	for c.peek().t == tokAnd {
		c.advance()
		right := c.compileEquality()
		prev := left
		left = func(scope map[string]*jsValue) *jsValue {
			l := prev(scope)
			if !l.truthy() {
				return l
			}
			return right(scope)
		}
	}
	return left
}

func (c *compiler) compileEquality() compiledExpr {
	left := c.compileComparison()
	for {
		t := c.peek().t
		if t == tokEqEqEq || t == tokEqEq {
			c.advance()
			right := c.compileComparison()
			prev := left
			left = func(scope map[string]*jsValue) *jsValue {
				return jvBool(jsStrictEqual(prev(scope), right(scope)))
			}
		} else if t == tokNotEqEq || t == tokNotEq {
			c.advance()
			right := c.compileComparison()
			prev := left
			left = func(scope map[string]*jsValue) *jsValue {
				return jvBool(!jsStrictEqual(prev(scope), right(scope)))
			}
		} else {
			break
		}
	}
	return left
}

func (c *compiler) compileComparison() compiledExpr {
	left := c.compileAdditive()
	for {
		t := c.peek().t
		switch t {
		case tokGt:
			c.advance()
			right := c.compileAdditive()
			prev := left
			left = func(scope map[string]*jsValue) *jsValue {
				return jvBool(prev(scope).toNum() > right(scope).toNum())
			}
		case tokLt:
			c.advance()
			right := c.compileAdditive()
			prev := left
			left = func(scope map[string]*jsValue) *jsValue {
				return jvBool(prev(scope).toNum() < right(scope).toNum())
			}
		case tokGtEq:
			c.advance()
			right := c.compileAdditive()
			prev := left
			left = func(scope map[string]*jsValue) *jsValue {
				return jvBool(prev(scope).toNum() >= right(scope).toNum())
			}
		case tokLtEq:
			c.advance()
			right := c.compileAdditive()
			prev := left
			left = func(scope map[string]*jsValue) *jsValue {
				return jvBool(prev(scope).toNum() <= right(scope).toNum())
			}
		default:
			return left
		}
	}
}

func (c *compiler) compileAdditive() compiledExpr {
	left := c.compileUnary()
	for {
		t := c.peek().t
		if t == tokPlus {
			c.advance()
			right := c.compileUnary()
			prev := left
			left = func(scope map[string]*jsValue) *jsValue {
				l, r := prev(scope), right(scope)
				if l.typ == jsTypeString || r.typ == jsTypeString {
					return jvStr(l.toStr() + r.toStr())
				}
				return jvNum(l.toNum() + r.toNum())
			}
		} else if t == tokMinus {
			c.advance()
			right := c.compileUnary()
			prev := left
			left = func(scope map[string]*jsValue) *jsValue {
				return jvNum(prev(scope).toNum() - right(scope).toNum())
			}
		} else {
			break
		}
	}
	return left
}

func (c *compiler) compileUnary() compiledExpr {
	if c.peek().t == tokNot {
		c.advance()
		val := c.compileUnary()
		return func(scope map[string]*jsValue) *jsValue {
			return jvBool(!val(scope).truthy())
		}
	}
	if c.peek().t == tokMinus {
		c.advance()
		val := c.compileUnary()
		return func(scope map[string]*jsValue) *jsValue {
			return jvNum(-val(scope).toNum())
		}
	}
	if c.peek().t == tokIdent && c.peek().v == "typeof" {
		c.advance()
		val := c.compileUnary()
		return func(scope map[string]*jsValue) *jsValue {
			v := val(scope)
			switch v.typ {
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
	}
	if c.peek().t == tokIdent && c.peek().v == "new" {
		c.advance()
		ctor := c.advance().v
		if c.peek().t == tokLParen {
			c.advance()
			var ctorArgs []compiledExpr
			for c.peek().t != tokRParen && c.peek().t != tokEOF {
				ctorArgs = append(ctorArgs, c.compileExpr())
				if c.peek().t == tokComma {
					c.advance()
				}
			}
			c.expect(tokRParen)
			_ = ctorArgs
			if ctor == "Date" {
				return func(scope map[string]*jsValue) *jsValue {
					return jvObj(map[string]*jsValue{
						"toLocaleTimeString": &jsValue{typ: jsTypeFunc, str: "__noop"},
						"toLocaleDateString": &jsValue{typ: jsTypeFunc, str: "__noop"},
						"getTime":            &jsValue{typ: jsTypeFunc, str: "__noop"},
					})
				}
			}
		}
		return func(scope map[string]*jsValue) *jsValue { return jvUndefined }
	}
	return c.compilePostfix()
}

func (c *compiler) compilePostfix() compiledExpr {
	val := c.compilePrimary()
	for {
		switch c.peek().t {
		case tokDot:
			c.advance()
			prop := c.advance().v
			if c.peek().t == tokLParen {
				// Method call
				val = c.compileMethodCall(val, prop)
			} else {
				prev := val
				val = func(scope map[string]*jsValue) *jsValue {
					return prev(scope).getProp(prop)
				}
			}
		case tokOptChain:
			c.advance()
			prop := c.advance().v
			prev := val
			if c.peek().t == tokLParen {
				// Skip the call for optional chain on null
				mc := c.compileMethodCall(prev, prop)
				val = func(scope map[string]*jsValue) *jsValue {
					v := prev(scope)
					if v.typ == jsTypeUndefined || v.typ == jsTypeNull {
						return jvUndefined
					}
					return mc(scope)
				}
			} else {
				val = func(scope map[string]*jsValue) *jsValue {
					v := prev(scope)
					if v.typ == jsTypeUndefined || v.typ == jsTypeNull {
						return jvUndefined
					}
					return v.getProp(prop)
				}
			}
		case tokLBrack:
			c.advance()
			idx := c.compileExpr()
			c.expect(tokRBrack)
			prev := val
			val = func(scope map[string]*jsValue) *jsValue {
				return prev(scope).getProp(idx(scope).toStr())
			}
		case tokLParen:
			// Function call — skip args (hooks, event handlers)
			val = c.compileFuncCall(val)
		default:
			return val
		}
	}
}

func (c *compiler) compileFuncCall(fn compiledExpr) compiledExpr {
	c.advance() // skip (
	var args []compiledExpr
	for c.peek().t != tokRParen && c.peek().t != tokEOF {
		args = append(args, c.compileExpr())
		if c.peek().t == tokComma {
			c.advance()
		}
	}
	c.expect(tokRParen)

	return func(scope map[string]*jsValue) *jsValue {
		fnVal := fn(scope)
		if fnVal.typ == jsTypeFunc {
			// Hook stubs
			switch fnVal.str {
			case "__hook_useState":
				initial := jvFalse
				if len(args) > 0 {
					initial = args[0](scope)
				}
				if initial.typ == jsTypeFunc && initial.str == "__resolved" && initial.object != nil {
					if v, ok := initial.object["__value"]; ok {
						initial = v
					}
				} else if initial.typ == jsTypeFunc && initial.str == "__arrow" {
					initial = callArrow(int(initial.num), nil, scope)
				} else if initial.typ == jsTypeFunc {
					initial = jvFalse
				}
				return jvArr([]*jsValue{initial, &jsValue{typ: jsTypeFunc, str: "__noop"}})
			case "__hook_useEffect":
				return jvUndefined
			case "__hook_useRouter":
				pathname := "/"
				if p, ok := scope["__ssr_pathname"]; ok && p.str != "" {
					pathname = p.str
				}
				return jvObj(map[string]*jsValue{
					"pathname": jvStr(pathname),
					"query":    jvObj(map[string]*jsValue{}),
				})
			case "__hook_useRef":
				initial := jvNull
				if len(args) > 0 {
					initial = args[0](scope)
				}
				return jvObj(map[string]*jsValue{"current": initial})
			case "__hook_useMemo":
				if len(args) > 0 {
					val := args[0](scope)
					if val.typ == jsTypeFunc && val.str == "__resolved" && val.object != nil {
						if v, ok := val.object["__value"]; ok {
							return v
						}
					}
					if val.typ == jsTypeFunc && val.str == "__arrow" {
						return callArrow(int(val.num), nil, scope)
					}
				}
				return jvUndefined
			case "__noop", "__resolved":
				return jvUndefined
			}
		}
		return jvUndefined
	}
}

func (c *compiler) compileMethodCall(obj compiledExpr, method string) compiledExpr {
	c.advance() // skip (

	switch method {
	case "map":
		return c.compileMapCall(obj)
	case "filter":
		return c.compileFilterCall(obj)
	case "join":
		arg := c.compileExpr()
		c.expect(tokRParen)
		return func(scope map[string]*jsValue) *jsValue {
			arr := obj(scope)
			if arr.typ != jsTypeArray {
				return jvStr("")
			}
			sep := arg(scope).str
			var parts []string
			for _, item := range arr.array {
				parts = append(parts, item.toStr())
			}
			return jvStr(strings.Join(parts, sep))
		}
	case "split":
		arg := c.compileExpr()
		c.expect(tokRParen)
		return func(scope map[string]*jsValue) *jsValue {
			s := obj(scope)
			if s.typ != jsTypeString {
				return jvArr(nil)
			}
			parts := strings.Split(s.str, arg(scope).str)
			arr := make([]*jsValue, len(parts))
			for i, p := range parts {
				arr[i] = jvStr(p)
			}
			return jvArr(arr)
		}
	case "isArray":
		arg := c.compileExpr()
		c.expect(tokRParen)
		return func(scope map[string]*jsValue) *jsValue {
			return jvBool(arg(scope).typ == jsTypeArray)
		}
	case "padStart":
		targetLen := c.compileExpr()
		var padStr compiledExpr
		if c.peek().t == tokComma {
			c.advance()
			padStr = c.compileExpr()
		}
		c.expect(tokRParen)
		return func(scope map[string]*jsValue) *jsValue {
			s := obj(scope).toStr()
			tl := int(targetLen(scope).toNum())
			ps := " "
			if padStr != nil {
				ps = padStr(scope).toStr()
			}
			for len(s) < tl {
				s = ps + s
			}
			return jvStr(s)
		}
	case "toFixed":
		digits := c.compileExpr()
		c.expect(tokRParen)
		return func(scope map[string]*jsValue) *jsValue {
			return jvStr(strconv.FormatFloat(obj(scope).toNum(), 'f', int(digits(scope).toNum()), 64))
		}
	case "toString":
		c.expect(tokRParen)
		return func(scope map[string]*jsValue) *jsValue {
			return jvStr(obj(scope).toStr())
		}
	case "find":
		return c.compileFindCall(obj)
	case "findIndex":
		return c.compileFindIndexCall(obj)
	case "some":
		return c.compileSomeCall(obj)
	case "every":
		return c.compileEveryCall(obj)
	default:
		// Unknown method — skip args
		for c.peek().t != tokRParen && c.peek().t != tokEOF {
			c.compileExpr()
			if c.peek().t == tokComma {
				c.advance()
			}
		}
		c.expect(tokRParen)
		return func(scope map[string]*jsValue) *jsValue { return jvUndefined }
	}
}

func (c *compiler) compileMapCall(arr compiledExpr) compiledExpr {
	params := c.parseCompilerArrowParams()
	c.expect(tokArrow)

	// Try to compile the body as a compiled node (h() call)
	bodyStart := c.pos
	hasParen := c.peek().t == tokLParen
	if hasParen {
		c.advance()
	}

	// Check if body starts with h(
	if c.peek().t == tokIdent && c.peek().v == "h" {
		body := c.compileHCallAsNode()
		if body != nil {
			if hasParen {
				c.expect(tokRParen)
			}
			c.expect(tokRParen) // close .map()

			paramName := ""
			indexName := ""
			if len(params) > 0 {
				paramName = params[0]
			}
			if len(params) > 1 {
				indexName = params[1]
			}

			cm := &compiledMap{
				ArrayExpr: arr,
				ParamName: paramName,
				IndexName: indexName,
				Body:      body,
			}
			return func(scope map[string]*jsValue) *jsValue {
				nodes := cm.execute(scope)
				vals := make([]*jsValue, len(nodes))
				for i, n := range nodes {
					vals[i] = jvNode(n)
				}
				return jvArr(vals)
			}
		}
	}

	// Fallback: compile as generic expression
	c.pos = bodyStart
	if hasParen {
		c.pos-- // back before (
	}
	return c.compileMapFallback(arr, params)
}

func (c *compiler) compileMapFallback(arr compiledExpr, params []string) compiledExpr {
	// Skip to closing ) of .map()
	bodyStart := c.pos
	hasParen := c.peek().t == tokLParen
	if hasParen {
		c.advance()
	}
	bodyExpr := c.compileExpr()
	if hasParen {
		c.expect(tokRParen)
	}
	c.expect(tokRParen) // close .map()
	_ = bodyStart

	return func(scope map[string]*jsValue) *jsValue {
		arrVal := arr(scope)
		if arrVal.typ != jsTypeArray {
			return jvArr(nil)
		}
		results := make([]*jsValue, 0, len(arrVal.array))
		for i, item := range arrVal.array {
			childScope := getPooledScope(scope)
			if len(params) > 0 {
				childScope[params[0]] = item
			}
			if len(params) > 1 {
				childScope[params[1]] = jvNum(float64(i))
			}
			results = append(results, bodyExpr(childScope))
			putPooledScope(childScope)
		}
		return jvArr(results)
	}
}

func (c *compiler) compileFilterCall(arr compiledExpr) compiledExpr {
	params := c.parseCompilerArrowParams()
	c.expect(tokArrow)
	body := c.compileExpr()
	c.expect(tokRParen)

	return func(scope map[string]*jsValue) *jsValue {
		arrVal := arr(scope)
		if arrVal.typ != jsTypeArray {
			return jvArr(nil)
		}
		var results []*jsValue
		for i, item := range arrVal.array {
			childScope := getPooledScope(scope)
			if len(params) > 0 {
				childScope[params[0]] = item
			}
			if len(params) > 1 {
				childScope[params[1]] = jvNum(float64(i))
			}
			if body(childScope).truthy() {
				results = append(results, item)
			}
			putPooledScope(childScope)
		}
		return jvArr(results)
	}
}

func (c *compiler) compileFindCall(arr compiledExpr) compiledExpr {
	params := c.parseCompilerArrowParams()
	c.expect(tokArrow)
	body := c.compileExpr()
	c.expect(tokRParen)

	return func(scope map[string]*jsValue) *jsValue {
		arrVal := arr(scope)
		if arrVal.typ != jsTypeArray {
			return jvUndefined
		}
		for i, item := range arrVal.array {
			childScope := getPooledScope(scope)
			if len(params) > 0 {
				childScope[params[0]] = item
			}
			if len(params) > 1 {
				childScope[params[1]] = jvNum(float64(i))
			}
			found := body(childScope).truthy()
			putPooledScope(childScope)
			if found {
				return item
			}
		}
		return jvUndefined
	}
}

func (c *compiler) compileFindIndexCall(arr compiledExpr) compiledExpr {
	params := c.parseCompilerArrowParams()
	c.expect(tokArrow)
	body := c.compileExpr()
	c.expect(tokRParen)

	return func(scope map[string]*jsValue) *jsValue {
		arrVal := arr(scope)
		if arrVal.typ != jsTypeArray {
			return jvNum(-1)
		}
		for i, item := range arrVal.array {
			childScope := getPooledScope(scope)
			if len(params) > 0 {
				childScope[params[0]] = item
			}
			if len(params) > 1 {
				childScope[params[1]] = jvNum(float64(i))
			}
			found := body(childScope).truthy()
			putPooledScope(childScope)
			if found {
				return jvNum(float64(i))
			}
		}
		return jvNum(-1)
	}
}

func (c *compiler) compileSomeCall(arr compiledExpr) compiledExpr {
	params := c.parseCompilerArrowParams()
	c.expect(tokArrow)
	body := c.compileExpr()
	c.expect(tokRParen)

	return func(scope map[string]*jsValue) *jsValue {
		arrVal := arr(scope)
		if arrVal.typ != jsTypeArray {
			return jvFalse
		}
		for i, item := range arrVal.array {
			childScope := getPooledScope(scope)
			if len(params) > 0 {
				childScope[params[0]] = item
			}
			if len(params) > 1 {
				childScope[params[1]] = jvNum(float64(i))
			}
			match := body(childScope).truthy()
			putPooledScope(childScope)
			if match {
				return jvTrue
			}
		}
		return jvFalse
	}
}

func (c *compiler) compileEveryCall(arr compiledExpr) compiledExpr {
	params := c.parseCompilerArrowParams()
	c.expect(tokArrow)
	body := c.compileExpr()
	c.expect(tokRParen)

	return func(scope map[string]*jsValue) *jsValue {
		arrVal := arr(scope)
		if arrVal.typ != jsTypeArray {
			return jvTrue
		}
		for i, item := range arrVal.array {
			childScope := getPooledScope(scope)
			if len(params) > 0 {
				childScope[params[0]] = item
			}
			if len(params) > 1 {
				childScope[params[1]] = jvNum(float64(i))
			}
			match := body(childScope).truthy()
			putPooledScope(childScope)
			if !match {
				return jvFalse
			}
		}
		return jvTrue
	}
}

func (c *compiler) parseCompilerArrowParams() []string {
	var params []string
	if c.peek().t == tokLParen {
		c.advance()
		for c.peek().t != tokRParen && c.peek().t != tokEOF {
			if c.peek().t == tokIdent {
				params = append(params, c.advance().v)
			}
			if c.peek().t == tokComma {
				c.advance()
			}
		}
		c.advance() // skip )
	} else if c.peek().t == tokIdent {
		params = append(params, c.advance().v)
	}
	return params
}

// ─── Compile h() calls into compiledNode ────────────────────────

func (c *compiler) compileHCallAsNode() *compiledNode {
	if c.peek().t != tokIdent || c.peek().v != "h" {
		return nil
	}
	c.advance() // skip h
	c.expect(tokLParen)

	// Tag
	var tag string
	var isComponent bool
	var compName string
	if c.peek().t == tokStr {
		tag = c.advance().v
	} else if c.peek().t == tokIdent {
		ident := c.advance().v
		if _, ok := c.funcs[ident]; ok {
			isComponent = true
			compName = ident
			tag = "div" // placeholder
		} else {
			tag = ident
		}
	}

	if c.peek().t == tokComma {
		c.advance()
	}

	// Props
	var staticProps map[string]string
	var dynamicProps map[string]compiledExpr
	if c.peek().t == tokIdent && c.peek().v == "null" {
		c.advance()
	} else if c.peek().t == tokLBrace {
		staticProps, dynamicProps = c.compileProps()
	}

	// Children
	var children []compiledChild
	for c.peek().t == tokComma {
		c.advance()
		if c.peek().t == tokRParen {
			break
		}
		child := c.compileChildExpr()
		children = append(children, child)
	}

	c.expect(tokRParen)

	// Component: compile as dynamic expression that calls the component function
	if isComponent {
		fn := c.funcs[compName]
		props := staticProps
		dynProps := dynamicProps
		childNodes := children
		// Return nil node — the component call is handled as a dynamic child expression
		// The parent's compiledChild.Expr will call this
		compExpr := func(scope map[string]*jsValue) *jsValue {
			callProps := make(map[string]*jsValue)
			for k, v := range props {
				callProps[k] = jvStr(v)
			}
			for k, expr := range dynProps {
				callProps[k] = expr(scope)
			}
			if len(childNodes) > 0 {
				var childVals []*jsValue
				for _, ch := range childNodes {
					if ch.Node != nil {
						childVals = append(childVals, jvNode(ch.Node.execute(scope)))
					} else if ch.MapExpr != nil {
						nodes := ch.MapExpr.execute(scope)
						for _, n := range nodes {
							childVals = append(childVals, jvNode(n))
						}
					} else if ch.Expr != nil {
						childVals = append(childVals, ch.Expr(scope))
					}
				}
				callProps["children"] = jvArr(childVals)
			}
			ev := &jsEval{scope: scope}
			return ev.callFunc(fn, callProps)
		}
		// Wrap in a transparent node that just renders the component result
		return &compiledNode{
			IsText:      false,
			Tag:         "", // empty tag = fragment, renders children only
			Children:    []compiledChild{{Expr: compExpr}},
		}
	}

	return &compiledNode{
		Tag:          tag,
		StaticProps:  staticProps,
		DynamicProps: dynamicProps,
		Children:     children,
	}
}

func (c *compiler) compileProps() (map[string]string, map[string]compiledExpr) {
	c.expect(tokLBrace)
	static := make(map[string]string)
	dynamic := make(map[string]compiledExpr)

	for c.peek().t != tokRBrace && c.peek().t != tokEOF {
		// Spread
		if c.peek().t == tokSpread {
			c.advance()
			expr := c.compileExpr()
			// Can't easily handle spread in compiled mode — use dynamic
			dynamic["__spread__"] = expr
			if c.peek().t == tokComma {
				c.advance()
			}
			continue
		}

		var key string
		if c.peek().t == tokStr {
			key = c.advance().v
		} else if c.peek().t == tokIdent {
			key = c.advance().v
		} else {
			c.advance()
			continue
		}

		c.expect(tokColon)

		// Check if value is a simple static literal (no operators after it)
		if c.peek().t == tokStr && c.peekAt(1).t != tokPlus {
			static[key] = c.advance().v
		} else if c.peek().t == tokNum && c.peekAt(1).t != tokPlus {
			static[key] = c.advance().v
		} else {
			// Dynamic value (expression, concat, ternary, etc.)
			dynamic[key] = c.compileExpr()
		}

		if c.peek().t == tokComma {
			c.advance()
		}
	}
	c.expect(tokRBrace)
	return static, dynamic
}

func (c *compiler) compileChildExpr() compiledChild {
	// Check if it's an h() call — compile as node
	if c.peek().t == tokIdent && c.peek().v == "h" {
		node := c.compileHCallAsNode()
		if node != nil {
			return compiledChild{Node: node}
		}
	}

	// Check for static string
	if c.peek().t == tokStr {
		text := c.advance().v
		return compiledChild{Node: &compiledNode{IsText: true, StaticText: text}}
	}

	// Dynamic expression
	expr := c.compileExpr()
	return compiledChild{Expr: expr}
}

func (c *compiler) compilePrimary() compiledExpr {
	t := c.peek()

	switch t.t {
	case tokStr:
		c.advance()
		s := t.v
		return func(scope map[string]*jsValue) *jsValue { return jvStr(s) }

	case tokNum:
		c.advance()
		n := t.n
		return func(scope map[string]*jsValue) *jsValue { return jvNum(n) }

	case tokLParen:
		// Check for arrow function
		if c.isCompilerArrowFunction() {
			return c.compileArrowFunction()
		}
		c.advance()
		val := c.compileExpr()
		c.expect(tokRParen)
		return val

	case tokLBrack:
		return c.compileArray()

	case tokLBrace:
		return c.compileObject()

	case tokIdent:
		switch t.v {
		case "true":
			c.advance()
			return func(scope map[string]*jsValue) *jsValue { return jvTrue }
		case "false":
			c.advance()
			return func(scope map[string]*jsValue) *jsValue { return jvFalse }
		case "null":
			c.advance()
			return func(scope map[string]*jsValue) *jsValue { return jvNull }
		case "undefined":
			c.advance()
			return func(scope map[string]*jsValue) *jsValue { return jvUndefined }
		case "h":
			c.advance()
			return c.compileHCallExpr()
		case "Array":
			c.advance()
			if c.peek().t == tokDot {
				c.advance()
				method := c.advance()
				if method.v == "isArray" && c.peek().t == tokLParen {
					c.advance()
					arg := c.compileExpr()
					c.expect(tokRParen)
					return func(scope map[string]*jsValue) *jsValue {
						return jvBool(arg(scope).typ == jsTypeArray)
					}
				}
			}
			return func(scope map[string]*jsValue) *jsValue { return jvUndefined }
		case "Math":
			c.advance()
			if c.peek().t == tokDot {
				c.advance()
				method := c.advance().v
				if c.peek().t == tokLParen {
					c.advance()
					arg := c.compileExpr()
					// Check for second arg (min/max)
					var arg2 compiledExpr
					if c.peek().t == tokComma {
						c.advance()
						arg2 = c.compileExpr()
					}
					c.expect(tokRParen)
					return func(scope map[string]*jsValue) *jsValue {
						n := arg(scope).toNum()
						switch method {
						case "floor":
							return jvNum(float64(int64(n)))
						case "ceil":
							if n == float64(int64(n)) {
								return jvNum(n)
							}
							return jvNum(float64(int64(n) + 1))
						case "round":
							return jvNum(float64(int64(n + 0.5)))
						case "abs":
							if n < 0 {
								return jvNum(-n)
							}
							return jvNum(n)
						case "min":
							if arg2 != nil {
								b := arg2(scope).toNum()
								if b < n {
									return jvNum(b)
								}
							}
							return jvNum(n)
						case "max":
							if arg2 != nil {
								b := arg2(scope).toNum()
								if b > n {
									return jvNum(b)
								}
							}
							return jvNum(n)
						case "random":
							return jvNum(0)
						}
						return jvNum(n)
					}
				}
			}
			return func(scope map[string]*jsValue) *jsValue { return jvUndefined }
		case "String":
			c.advance()
			if c.peek().t == tokLParen {
				c.advance()
				arg := c.compileExpr()
				c.expect(tokRParen)
				return func(scope map[string]*jsValue) *jsValue {
					return jvStr(arg(scope).toStr())
				}
			}
			return func(scope map[string]*jsValue) *jsValue { return jvUndefined }
		default:
			c.advance()
			name := t.v
			return func(scope map[string]*jsValue) *jsValue {
				if val, ok := scope[name]; ok {
					return val
				}
				return jvUndefined
			}
		}

	default:
		c.advance()
		return func(scope map[string]*jsValue) *jsValue { return jvUndefined }
	}
}

func (c *compiler) compileHCallExpr() compiledExpr {
	// h( already consumed the "h", now expect (
	c.expect(tokLParen)

	// Tag
	var tag string
	var compName string
	if c.peek().t == tokStr {
		tag = c.advance().v
	} else if c.peek().t == tokIdent {
		compName = c.advance().v
		tag = compName
	}

	if c.peek().t == tokComma {
		c.advance()
	}

	// Props
	var propsExpr compiledExpr
	if c.peek().t == tokIdent && c.peek().v == "null" {
		c.advance()
	} else if c.peek().t == tokLBrace {
		propsExpr = c.compileObject()
	}

	// Children
	var childExprs []compiledExpr
	for c.peek().t == tokComma {
		c.advance()
		if c.peek().t == tokRParen {
			break
		}
		childExprs = append(childExprs, c.compileExpr())
	}
	c.expect(tokRParen)

	fn := c.funcs[compName]

	return func(scope map[string]*jsValue) *jsValue {
		// Component call
		if fn != nil && fn.typ == jsTypeFunc {
			callProps := make(map[string]*jsValue)
			if propsExpr != nil {
				p := propsExpr(scope)
				if p.typ == jsTypeObject && p.object != nil {
					callProps = p.object
				}
			}
			ev := &jsEval{scope: scope}
			return ev.callFunc(fn, callProps)
		}

		// Regular element
		var props map[string]*jsValue
		if propsExpr != nil {
			p := propsExpr(scope)
			if p.typ == jsTypeObject {
				props = p.object
			}
		}
		var children []*ssrNode
		for _, ce := range childExprs {
			val := ce(scope)
			children = append(children, jsValueToNodes(val)...)
		}
		return jvNode(&ssrNode{Tag: tag, Props: props, Children: children})
	}
}

func (c *compiler) compileArray() compiledExpr {
	c.expect(tokLBrack)
	var items []compiledExpr
	for c.peek().t != tokRBrack && c.peek().t != tokEOF {
		items = append(items, c.compileExpr())
		if c.peek().t == tokComma {
			c.advance()
		}
	}
	c.expect(tokRBrack)

	if len(items) == 0 {
		return func(scope map[string]*jsValue) *jsValue { return jvArr(nil) }
	}
	return func(scope map[string]*jsValue) *jsValue {
		arr := make([]*jsValue, len(items))
		for i, item := range items {
			arr[i] = item(scope)
		}
		return jvArr(arr)
	}
}

func (c *compiler) compileObject() compiledExpr {
	c.expect(tokLBrace)
	type kv struct {
		key  string
		expr compiledExpr
	}
	var pairs []kv

	for c.peek().t != tokRBrace && c.peek().t != tokEOF {
		var key string
		if c.peek().t == tokStr {
			key = c.advance().v
		} else if c.peek().t == tokIdent {
			key = c.advance().v
		} else {
			c.advance()
			continue
		}

		if c.peek().t == tokComma || c.peek().t == tokRBrace {
			// Shorthand
			k := key
			pairs = append(pairs, kv{key: k, expr: func(scope map[string]*jsValue) *jsValue {
				if v, ok := scope[k]; ok {
					return v
				}
				return jvUndefined
			}})
			if c.peek().t == tokComma {
				c.advance()
			}
			continue
		}

		c.expect(tokColon)
		val := c.compileExpr()
		pairs = append(pairs, kv{key: key, expr: val})
		if c.peek().t == tokComma {
			c.advance()
		}
	}
	c.expect(tokRBrace)

	return func(scope map[string]*jsValue) *jsValue {
		obj := make(map[string]*jsValue, len(pairs))
		for _, p := range pairs {
			obj[p.key] = p.expr(scope)
		}
		return jvObj(obj)
	}
}

func (c *compiler) isCompilerArrowFunction() bool {
	saved := c.pos
	defer func() { c.pos = saved }()
	c.pos++ // skip (
	depth := 1
	for c.pos < len(c.tokens) && depth > 0 {
		if c.tokens[c.pos].t == tokLParen {
			depth++
		} else if c.tokens[c.pos].t == tokRParen {
			depth--
		}
		c.pos++
	}
	return c.pos < len(c.tokens) && c.tokens[c.pos].t == tokArrow
}

func (c *compiler) compileArrowFunction() compiledExpr {
	c.advance() // skip (
	for c.peek().t != tokRParen && c.peek().t != tokEOF {
		c.advance()
	}
	c.expect(tokRParen)
	c.expect(tokArrow)
	// Skip body
	if c.peek().t == tokLBrace {
		c.skipCompilerBalanced(tokLBrace, tokRBrace)
	} else {
		c.compileExpr()
	}
	return func(scope map[string]*jsValue) *jsValue {
		return &jsValue{typ: jsTypeFunc, str: "__noop"}
	}
}

func (c *compiler) skipCompilerBalanced(open, close tokType) {
	depth := 1
	c.advance()
	for depth > 0 && c.pos < len(c.tokens) {
		t := c.advance()
		if t.t == open {
			depth++
		} else if t.t == close {
			depth--
		}
	}
}
