package lungo

import (
	"strings"
	"unicode"
)

// TranspileJSX converts JSX syntax to h() function calls.
// It also runs Next.js/React compatibility transforms (imports, attributes, TypeScript).
// Input:  <div className="foo"><Button onClick={handler}>text</Button></div>
// Output: h("div", {class: "foo"}, h(Button, {onclick: handler}, "text"))
func TranspileJSX(source string) string {
	// Step 1: Next.js / React compatibility transforms
	source = NextCompat(source)
	// Step 2: JSX → h() calls
	t := &jsxTranspiler{src: source, pos: 0}
	return t.transpile()
}

type jsxTranspiler struct {
	src string
	pos int
}

func (t *jsxTranspiler) transpile() string {
	var out strings.Builder
	for t.pos < len(t.src) {
		// Look for JSX opening tag, but not inside strings or template literals
		ch := t.src[t.pos]

		// Skip string literals
		if ch == '"' || ch == '\'' || ch == '`' {
			t.copyString(&out, ch)
			continue
		}

		// Skip line comments
		if ch == '/' && t.pos+1 < len(t.src) && t.src[t.pos+1] == '/' {
			t.copyLineComment(&out)
			continue
		}

		// Skip block comments
		if ch == '/' && t.pos+1 < len(t.src) && t.src[t.pos+1] == '*' {
			t.copyBlockComment(&out)
			continue
		}

		// Detect JSX: < followed by a letter or component name
		// But not comparison operators like < in expressions
		if ch == '<' && t.pos+1 < len(t.src) && (isTagStart(t.src[t.pos+1]) || t.src[t.pos+1] == '>') {
			// Check if this looks like JSX (not a comparison)
			if t.looksLikeJSX() {
				jsx := t.parseJSXElement()
				out.WriteString(jsx)
				continue
			}
		}

		out.WriteByte(ch)
		t.pos++
	}
	return out.String()
}

func (t *jsxTranspiler) looksLikeJSX() bool {
	// Look at what came before: if it's an operator context, it's JSX
	// Simple heuristic: check the last non-space char
	before := strings.TrimRight(t.src[:t.pos], " \t\n\r")
	if len(before) == 0 {
		return true
	}
	last := before[len(before)-1]
	// JSX follows: return, (, =, ,, :, [, {, ?, &&, ||, ;, or start of line
	return last == '(' || last == '=' || last == ',' || last == ':' ||
		last == '[' || last == '{' || last == '?' || last == '&' ||
		last == '|' || last == ';' || last == 'n' || // return
		last == '>' || // =>
		last == '!'
}

func (t *jsxTranspiler) parseJSXElement() string {
	t.pos++ // skip <

	// Fragment: <>...</>
	if t.pos < len(t.src) && t.src[t.pos] == '>' {
		t.pos++ // skip >
		children := t.parseJSXChildren("</>")
		return "h(null, null" + formatChildren(children) + ")"
	}

	// Parse tag name
	tag := t.parseTagName()
	if tag == "" {
		return "<"
	}

	// Parse attributes
	attrs := t.parseAttributes()

	// Self-closing? />
	t.skipWhitespace()
	if t.pos+1 < len(t.src) && t.src[t.pos] == '/' && t.src[t.pos+1] == '>' {
		t.pos += 2
		return formatHCall(tag, attrs, nil)
	}

	// End of opening tag: >
	if t.pos < len(t.src) && t.src[t.pos] == '>' {
		t.pos++
	}

	// Parse children until closing tag
	closeTag := "</" + tag + ">"
	children := t.parseJSXChildren(closeTag)

	return formatHCall(tag, attrs, children)
}

func (t *jsxTranspiler) parseTagName() string {
	start := t.pos
	for t.pos < len(t.src) {
		ch := t.src[t.pos]
		if isTagChar(ch) || ch == '.' {
			t.pos++
		} else {
			break
		}
	}
	return t.src[start:t.pos]
}

func (t *jsxTranspiler) parseAttributes() []jsxAttr {
	var attrs []jsxAttr
	for {
		t.skipWhitespace()
		if t.pos >= len(t.src) {
			break
		}
		// End of attributes
		if t.src[t.pos] == '>' || (t.src[t.pos] == '/' && t.pos+1 < len(t.src) && t.src[t.pos+1] == '>') {
			break
		}

		// Spread: {...obj}
		if t.src[t.pos] == '{' && t.pos+3 < len(t.src) && t.src[t.pos+1] == '.' && t.src[t.pos+2] == '.' && t.src[t.pos+3] == '.' {
			t.pos += 4 // skip {...
			expr := t.parseJSXExpression()
			attrs = append(attrs, jsxAttr{spread: true, value: expr})
			continue
		}

		// Attribute name
		name := t.parseAttrName()
		if name == "" {
			t.pos++
			continue
		}

		t.skipWhitespace()

		// Boolean attribute (no value)
		if t.pos >= len(t.src) || t.src[t.pos] != '=' {
			attrs = append(attrs, jsxAttr{name: name, value: "true"})
			continue
		}

		t.pos++ // skip =
		t.skipWhitespace()

		// Attribute value
		if t.pos >= len(t.src) {
			break
		}

		if t.src[t.pos] == '"' {
			// String value
			t.pos++
			val := t.readUntil('"')
			t.pos++ // skip closing "
			attrs = append(attrs, jsxAttr{name: name, value: `"` + val + `"`})
		} else if t.src[t.pos] == '\'' {
			t.pos++
			val := t.readUntil('\'')
			t.pos++
			attrs = append(attrs, jsxAttr{name: name, value: `"` + val + `"`})
		} else if t.src[t.pos] == '{' {
			// Expression value
			t.pos++ // skip {
			expr := t.parseJSXExpression()
			attrs = append(attrs, jsxAttr{name: name, value: expr})
		}
	}
	return attrs
}

func (t *jsxTranspiler) parseAttrName() string {
	start := t.pos
	for t.pos < len(t.src) {
		ch := t.src[t.pos]
		if isTagChar(ch) || ch == '-' {
			t.pos++
		} else {
			break
		}
	}
	return t.src[start:t.pos]
}

func (t *jsxTranspiler) parseJSXChildren(closeTag string) []string {
	var children []string

	for t.pos < len(t.src) {
		// Check for closing tag
		if strings.HasPrefix(t.src[t.pos:], closeTag) {
			t.pos += len(closeTag)
			break
		}

		// Check for </> (fragment close or generic close)
		if strings.HasPrefix(t.src[t.pos:], "</>") {
			t.pos += 3
			break
		}

		ch := t.src[t.pos]

		// Expression child: {expr}
		if ch == '{' {
			t.pos++
			expr := t.parseJSXExpression()
			if strings.TrimSpace(expr) != "" {
				children = append(children, strings.TrimSpace(expr))
			}
			continue
		}

		// Nested JSX element
		if ch == '<' && t.pos+1 < len(t.src) && (isTagStart(t.src[t.pos+1]) || t.src[t.pos+1] == '>') {
			child := t.parseJSXElement()
			children = append(children, child)
			continue
		}

		// Text content
		text := t.parseTextContent()
		if strings.TrimSpace(text) != "" {
			children = append(children, `"`+escapeJSString(strings.TrimSpace(text))+`"`)
		}
	}

	return children
}

func (t *jsxTranspiler) parseTextContent() string {
	start := t.pos
	for t.pos < len(t.src) {
		if t.src[t.pos] == '<' || t.src[t.pos] == '{' {
			break
		}
		t.pos++
	}
	return t.src[start:t.pos]
}

func (t *jsxTranspiler) parseJSXExpression() string {
	// Parse a JS expression inside {}, handling nested braces, strings, template literals
	depth := 1
	var out strings.Builder

	for t.pos < len(t.src) && depth > 0 {
		ch := t.src[t.pos]

		if ch == '{' {
			depth++
			out.WriteByte(ch)
			t.pos++
		} else if ch == '}' {
			depth--
			if depth > 0 {
				out.WriteByte(ch)
			}
			t.pos++
		} else if ch == '"' || ch == '\'' || ch == '`' {
			out.WriteString(t.readJSString(ch))
		} else if ch == '(' {
			out.WriteByte(ch)
			t.pos++
			// Read until matching paren, handling JSX inside
			parenExpr := t.readBalancedWithJSX('(', ')')
			out.WriteString(parenExpr)
			out.WriteByte(')')
		} else if ch == '[' {
			out.WriteByte(ch)
			t.pos++
			inner := t.readBalancedWithJSX('[', ']')
			out.WriteString(inner)
			out.WriteByte(']')
		} else if ch == '<' && t.pos+1 < len(t.src) && (isTagStart(t.src[t.pos+1]) || t.src[t.pos+1] == '>') {
			// Nested JSX inside expression
			jsx := t.parseJSXElement()
			out.WriteString(jsx)
		} else {
			out.WriteByte(ch)
			t.pos++
		}
	}

	return out.String()
}

// readBalancedWithJSX reads balanced parens/brackets while also transpiling any JSX inside.
func (t *jsxTranspiler) readBalancedWithJSX(open, close byte) string {
	depth := 1
	var out strings.Builder
	for t.pos < len(t.src) && depth > 0 {
		ch := t.src[t.pos]
		if ch == open {
			depth++
			out.WriteByte(ch)
			t.pos++
		} else if ch == close {
			depth--
			if depth == 0 {
				t.pos++
				return out.String()
			}
			out.WriteByte(ch)
			t.pos++
		} else if ch == '"' || ch == '\'' || ch == '`' {
			out.WriteString(t.readJSString(ch))
		} else if ch == '<' && t.pos+1 < len(t.src) && (isTagStart(t.src[t.pos+1]) || t.src[t.pos+1] == '>') {
			// JSX inside parens
			jsx := t.parseJSXElement()
			out.WriteString(jsx)
		} else if ch == '(' {
			out.WriteByte(ch)
			t.pos++
			inner := t.readBalancedWithJSX('(', ')')
			out.WriteString(inner)
			out.WriteByte(')')
		} else {
			out.WriteByte(ch)
			t.pos++
		}
	}
	return out.String()
}

func (t *jsxTranspiler) looksLikeJSXInExpr() bool {
	// Inside an expression {}, < is more likely JSX than comparison
	// Check if followed by tag-like chars
	if t.pos+1 >= len(t.src) {
		return false
	}
	return isTagStart(t.src[t.pos+1])
}

func (t *jsxTranspiler) readBalanced(open, close byte) string {
	depth := 1
	var out strings.Builder
	for t.pos < len(t.src) && depth > 0 {
		ch := t.src[t.pos]
		if ch == open {
			depth++
		} else if ch == close {
			depth--
			if depth == 0 {
				t.pos++
				return out.String()
			}
		}
		if ch == '"' || ch == '\'' || ch == '`' {
			out.WriteString(t.readJSString(ch))
		} else {
			out.WriteByte(ch)
			t.pos++
		}
	}
	return out.String()
}

func (t *jsxTranspiler) readJSString(quote byte) string {
	var out strings.Builder
	out.WriteByte(quote)
	t.pos++
	for t.pos < len(t.src) {
		ch := t.src[t.pos]
		out.WriteByte(ch)
		t.pos++
		if ch == quote && (t.pos < 2 || t.src[t.pos-2] != '\\') {
			break
		}
		if quote == '`' && ch == '$' && t.pos < len(t.src) && t.src[t.pos] == '{' {
			// Template literal expression
			out.WriteByte('{')
			t.pos++
			inner := t.readBalanced('{', '}')
			out.WriteString(inner)
			out.WriteByte('}')
		}
	}
	return out.String()
}

func (t *jsxTranspiler) copyString(out *strings.Builder, quote byte) {
	out.WriteByte(quote)
	t.pos++
	for t.pos < len(t.src) {
		ch := t.src[t.pos]
		out.WriteByte(ch)
		t.pos++
		if ch == quote && t.src[t.pos-2] != '\\' {
			break
		}
		if quote == '`' && ch == '$' && t.pos < len(t.src) && t.src[t.pos] == '{' {
			out.WriteByte('{')
			t.pos++
			for t.pos < len(t.src) {
				c := t.src[t.pos]
				out.WriteByte(c)
				t.pos++
				if c == '}' {
					break
				}
			}
		}
	}
}

func (t *jsxTranspiler) copyLineComment(out *strings.Builder) {
	for t.pos < len(t.src) && t.src[t.pos] != '\n' {
		out.WriteByte(t.src[t.pos])
		t.pos++
	}
}

func (t *jsxTranspiler) copyBlockComment(out *strings.Builder) {
	out.WriteByte(t.src[t.pos])
	t.pos++
	out.WriteByte(t.src[t.pos])
	t.pos++
	for t.pos+1 < len(t.src) {
		if t.src[t.pos] == '*' && t.src[t.pos+1] == '/' {
			out.WriteByte('*')
			out.WriteByte('/')
			t.pos += 2
			return
		}
		out.WriteByte(t.src[t.pos])
		t.pos++
	}
}

func (t *jsxTranspiler) skipWhitespace() {
	for t.pos < len(t.src) && (t.src[t.pos] == ' ' || t.src[t.pos] == '\t' || t.src[t.pos] == '\n' || t.src[t.pos] == '\r') {
		t.pos++
	}
}

func (t *jsxTranspiler) readUntil(ch byte) string {
	start := t.pos
	for t.pos < len(t.src) && t.src[t.pos] != ch {
		if t.src[t.pos] == '\\' {
			t.pos++ // skip escape
		}
		t.pos++
	}
	return t.src[start:t.pos]
}

type jsxAttr struct {
	name   string
	value  string
	spread bool
}

func formatHCall(tag string, attrs []jsxAttr, children []string) string {
	var out strings.Builder
	out.WriteString("h(")

	// Tag — quoted for HTML elements, unquoted for components
	if len(tag) > 0 && unicode.IsUpper(rune(tag[0])) {
		out.WriteString(tag)
	} else {
		out.WriteByte('"')
		out.WriteString(tag)
		out.WriteByte('"')
	}

	// Props
	if len(attrs) == 0 {
		out.WriteString(", null")
	} else {
		out.WriteString(", {")
		for i, a := range attrs {
			if i > 0 {
				out.WriteString(", ")
			}
			if a.spread {
				out.WriteString("..." + a.value)
			} else {
				// Quote attribute names that contain hyphens (e.g., stroke-width)
				if strings.Contains(a.name, "-") {
					out.WriteByte('"')
					out.WriteString(a.name)
					out.WriteByte('"')
				} else {
					out.WriteString(a.name)
				}
				out.WriteString(": ")
				out.WriteString(a.value)
			}
		}
		out.WriteByte('}')
	}

	// Children
	out.WriteString(formatChildren(children))

	out.WriteByte(')')
	return out.String()
}

func formatChildren(children []string) string {
	if len(children) == 0 {
		return ""
	}
	var out strings.Builder
	for _, c := range children {
		out.WriteString(", ")
		out.WriteString(c)
	}
	return out.String()
}

func isTagStart(ch byte) bool {
	return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || ch == '_'
}

func isTagChar(ch byte) bool {
	return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_'
}

func escapeJSString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}
