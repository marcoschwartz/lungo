package lungo

import (
	"strings"
)

// RenderSSRHTML converts an SSR vnode tree into an HTML string.
func RenderSSRHTML(node *ssrNode) string {
	if node == nil {
		return ""
	}
	var sb strings.Builder
	renderNode(node, &sb)
	return sb.String()
}

func renderNode(node *ssrNode, sb *strings.Builder) {
	if node == nil {
		return
	}

	// Text node
	if node.IsText {
		sb.WriteString(htmlEscape(node.Text))
		return
	}

	// Fragment (empty tag)
	if node.Tag == "" {
		for _, child := range node.Children {
			renderNode(child, sb)
		}
		return
	}

	// Opening tag
	sb.WriteByte('<')
	sb.WriteString(node.Tag)

	// Render attributes
	if node.Props != nil {
		renderAttrs(node.Props, sb)
	}

	// Void elements (self-closing)
	if isVoidElement(node.Tag) {
		sb.WriteString(" />")
		return
	}

	sb.WriteByte('>')

	// Children
	for _, child := range node.Children {
		renderNode(child, sb)
	}

	// Closing tag
	sb.WriteString("</")
	sb.WriteString(node.Tag)
	sb.WriteByte('>')
}

func renderAttrs(props map[string]*jsValue, sb *strings.Builder) {
	for key, val := range props {
		// Skip event handlers (client-only)
		if strings.HasPrefix(key, "on") && len(key) > 2 {
			continue
		}
		// Skip ref (client-only)
		if key == "ref" || key == "key" || key == "children" || key == "dangerouslySetInnerHTML" {
			continue
		}

		// Style object → inline style string
		if key == "style" && val.typ == jsTypeObject {
			sb.WriteString(` style="`)
			renderStyleObject(val, sb)
			sb.WriteByte('"')
			continue
		}

		// Boolean attributes
		if val.typ == jsTypeBool {
			if val.bool {
				sb.WriteByte(' ')
				sb.WriteString(key)
			}
			continue
		}

		// Skip undefined/null
		if val.typ == jsTypeUndefined || val.typ == jsTypeNull {
			continue
		}

		// Regular attribute
		sb.WriteByte(' ')
		sb.WriteString(key)
		sb.WriteString(`="`)
		sb.WriteString(htmlEscapeAttr(val.toStr()))
		sb.WriteByte('"')
	}
}

func renderStyleObject(val *jsValue, sb *strings.Builder) {
	if val.object == nil {
		return
	}
	first := true
	for key, v := range val.object {
		if v.typ == jsTypeUndefined || v.typ == jsTypeNull {
			continue
		}
		if !first {
			sb.WriteByte(';')
		}
		first = false
		// Convert camelCase to kebab-case
		sb.WriteString(camelToKebab(key))
		sb.WriteByte(':')
		sb.WriteString(v.toStr())
	}
}

func camelToKebab(s string) string {
	var sb strings.Builder
	for i, ch := range s {
		if ch >= 'A' && ch <= 'Z' {
			if i > 0 {
				sb.WriteByte('-')
			}
			sb.WriteByte(byte(ch + 32)) // lowercase
		} else {
			sb.WriteRune(ch)
		}
	}
	return sb.String()
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func htmlEscapeAttr(s string) string {
	s = htmlEscape(s)
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true,
	"embed": true, "hr": true, "img": true, "input": true,
	"link": true, "meta": true, "source": true, "track": true,
	"wbr": true,
}

func isVoidElement(tag string) bool {
	return voidElements[tag]
}
