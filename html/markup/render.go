package markup

import (
	"strings"

	"github.com/onix-engine/onix/scene/node"
)

// HTML serializes a native document or subtree for transport.
func HTML(n *node.Node) string {
	return HTMLWithSizeHint(n, 0)
}

// HTMLWithSizeHint reserves space for a known source length before encoding.
func HTMLWithSizeHint(n *node.Node, size int) string {
	var e encoder
	e.out.Grow(size)
	e.write(n)
	return e.out.String()
}

type encoder struct {
	fragments  map[*node.Node]string
	subtree    *node.Node
	out        strings.Builder
	start, end int
}

func (e *encoder) write(n *node.Node) {
	if n == nil {
		return
	}
	out := &e.out
	if n == e.subtree {
		e.start = out.Len()
		defer func() { e.end = out.Len() }()
	}
	switch n.Type {
	case node.DocumentNode:
		if fragment, ok := e.fragments[n]; ok {
			out.WriteString(fragment)
			return
		}
	case node.TextNode:
		if n.Parent != nil && n.Parent.Namespace == node.HTML {
			switch strings.ToLower(n.Parent.Data) {
			case "script", "style", "xmp", "iframe", "noembed", "noframes", "plaintext":
				out.WriteString(n.Data)
				return
			}
		}
		escape(out, n.Data, false)
		return
	case node.ElementNode:
		writeStartTag(out, n)
		if n.Namespace == node.HTML {
			switch strings.ToLower(n.Data) {
			case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr":
				return
			case "pre", "listing", "textarea":
				if len(n.Children) > 0 && n.Children[0] != nil && n.Children[0].Type == node.TextNode && strings.HasPrefix(n.Children[0].Data, "\n") {
					out.WriteByte('\n')
				}
			}
		}
	}
	for _, child := range n.Children {
		e.write(child)
	}
	if n.Type == node.ElementNode {
		out.WriteString("</")
		out.WriteString(n.Data)
		out.WriteByte('>')
	}
}

func escape(out *strings.Builder, value string, attribute bool) {
	start := 0
	for i := 0; i < len(value); i++ {
		entity := escapeEntity(value[i], attribute)
		if entity == "" {
			continue
		}
		out.WriteString(value[start:i])
		out.WriteString(entity)
		start = i + 1
	}
	out.WriteString(value[start:])
}

func escapeEntity(c byte, attribute bool) string {
	switch c {
	case '&':
		return "&amp;"
	case '<':
		return "&lt;"
	case '>':
		return "&gt;"
	case '"':
		if attribute {
			return "&quot;"
		}
	case 0:
		return "\uFFFD"
	case '\r':
		return "&#13;"
	}
	return ""
}

func escapedSize(value string, attribute bool) int {
	size := len(value)
	for i := 0; i < len(value); i++ {
		if entity := escapeEntity(value[i], attribute); entity != "" {
			size += len(entity) - 1
		}
	}
	return size
}

func writeStartTag(out *strings.Builder, n *node.Node) {
	out.WriteByte('<')
	out.WriteString(n.Data)
	for name, value := range n.SortedAttributes() {
		out.WriteByte(' ')
		out.WriteString(name)
		out.WriteString("=\"")
		escape(out, value, true)
		out.WriteByte('"')
	}
	out.WriteByte('>')
}

// StartTag serializes an opening element for streaming HTML renderer callbacks.
func StartTag(n *node.Node) string {
	var out strings.Builder
	writeStartTag(&out, n)
	return out.String()
}

// EndTag closes a non-void element for streaming HTML renderer callbacks.
func EndTag(tag string) string { return "</" + tag + ">" }
