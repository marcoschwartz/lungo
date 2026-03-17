package lungo

import (
	"strings"

	"github.com/marcoschwartz/lungo/pkg/espresso"
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

// RenderSSRHTMLPooled uses a pooled string builder for performance.
func RenderSSRHTMLPooled(node *ssrNode) string {
	return RenderSSRHTML(node)
}

func renderNode(node *ssrNode, sb *strings.Builder) {
	if node == nil {
		return
	}

	if node.IsText {
		sb.WriteString(htmlEscape(node.Text))
		return
	}

	if node.Tag == "" {
		for _, child := range node.Children {
			renderNode(child, sb)
		}
		return
	}

	sb.WriteByte('<')
	sb.WriteString(node.Tag)

	if node.Props != nil {
		renderAttrs(node.Props, sb)
	}

	if isVoidElement(node.Tag) {
		sb.WriteString(" />")
		return
	}

	sb.WriteByte('>')

	for _, child := range node.Children {
		renderNode(child, sb)
	}

	sb.WriteString("</")
	sb.WriteString(node.Tag)
	sb.WriteByte('>')
}

func renderAttrs(props map[string]*espresso.Value, sb *strings.Builder) {
	for key, val := range props {
		if strings.HasPrefix(key, "on") && len(key) > 2 {
			continue
		}
		if key == "ref" || key == "key" || key == "children" || key == "dangerouslySetInnerHTML" {
			continue
		}

		// Style object → inline style string
		if key == "style" && val.Type() == espresso.TypeObject && val.Object() != nil {
			sb.WriteString(` style="`)
			renderStyleObject(val, sb)
			sb.WriteByte('"')
			continue
		}

		// Boolean attributes
		if val.Type() == espresso.TypeBool {
			if val.Truthy() {
				sb.WriteByte(' ')
				sb.WriteString(key)
			}
			continue
		}

		if val.IsUndefined() || val.IsNull() {
			continue
		}

		sb.WriteByte(' ')
		sb.WriteString(key)
		sb.WriteString(`="`)
		sb.WriteString(htmlEscapeAttr(val.String()))
		sb.WriteByte('"')
	}
}

func renderStyleObject(val *espresso.Value, sb *strings.Builder) {
	if val.Object() == nil {
		return
	}
	first := true
	for key, v := range val.Object() {
		if v.IsUndefined() || v.IsNull() {
			continue
		}
		if !first {
			sb.WriteByte(';')
		}
		first = false
		sb.WriteString(camelToKebab(key))
		sb.WriteByte(':')
		sb.WriteString(v.String())
	}
}

func camelToKebab(s string) string {
	var sb strings.Builder
	for i, ch := range s {
		if ch >= 'A' && ch <= 'Z' {
			if i > 0 {
				sb.WriteByte('-')
			}
			sb.WriteByte(byte(ch + 32))
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
