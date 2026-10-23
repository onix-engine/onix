package html

import (
	"fmt"
	"sort"
	"strings"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/internal/dom"
)

// Serialize returns the current retained HTML document for page.
//
// The returned string is HTML markup, rather than the diagnostic tree emitted
// by dom.Node.String. The retained DOM is serialized after all mutations that
// have already been applied to the page, so callers should run Sync after
// changing component state when the serialized output must include that
// change.
func Serialize(world *ecs.World, page ecs.Entity) (string, error) {
	root, err := Root(world, page)
	if err != nil {
		return "", err
	}
	return serializeHTMLNode(root.node), nil
}

// Serialize serializes this retained node and all of its descendants as HTML
// markup. Root is the usual entry point when the whole page is needed.
func (n Node) Serialize() (string, error) {
	if !n.valid() {
		return "", fmt.Errorf("ecs: html node is invalid")
	}
	return serializeHTMLNode(n.node), nil
}

func serializeHTMLNode(node *dom.Node) string {
	if node == nil {
		return ""
	}
	var builder strings.Builder
	writeHTMLNode(&builder, node)
	return builder.String()
}

func writeHTMLNode(builder *strings.Builder, node *dom.Node) {
	if node == nil {
		return
	}
	switch node.Type {
	case dom.DocumentNode:
		for _, child := range node.Children {
			writeHTMLNode(builder, child)
		}
	case dom.TextNode:
		writeEscapedHTMLText(builder, node.Data)
	case dom.ElementNode:
		builder.WriteByte('<')
		builder.WriteString(node.Data)
		writeHTMLAttributes(builder, node.Attributes)
		builder.WriteByte('>')
		if htmlComponentVoidTag(node.Data) {
			return
		}
		for _, child := range node.Children {
			writeHTMLNode(builder, child)
		}
		builder.WriteString("</")
		builder.WriteString(node.Data)
		builder.WriteByte('>')
	}
}

func writeHTMLAttributes(builder *strings.Builder, attributes map[string]string) {
	if len(attributes) == 0 {
		return
	}
	names := make([]string, 0, len(attributes))
	for name := range attributes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		builder.WriteByte(' ')
		builder.WriteString(name)
		builder.WriteString("=\"")
		writeEscapedHTMLAttribute(builder, attributes[name])
		builder.WriteByte('"')
	}
}

func writeEscapedHTMLText(builder *strings.Builder, value string) {
	start := 0
	for index := 0; index < len(value); index++ {
		var replacement string
		switch value[index] {
		case '&':
			replacement = "&amp;"
		case '<':
			replacement = "&lt;"
		case '>':
			replacement = "&gt;"
		case 0:
			replacement = "\xef\xbf\xbd"
		default:
			continue
		}
		if start < index {
			builder.WriteString(value[start:index])
		}
		builder.WriteString(replacement)
		start = index + 1
	}
	if start < len(value) {
		builder.WriteString(value[start:])
	}
}

func writeEscapedHTMLAttribute(builder *strings.Builder, value string) {
	start := 0
	for index := 0; index < len(value); index++ {
		var replacement string
		switch value[index] {
		case '&':
			replacement = "&amp;"
		case '<':
			replacement = "&lt;"
		case '>':
			replacement = "&gt;"
		case '"':
			replacement = "&quot;"
		case 0:
			replacement = "\xef\xbf\xbd"
		default:
			continue
		}
		if start < index {
			builder.WriteString(value[start:index])
		}
		builder.WriteString(replacement)
		start = index + 1
	}
	if start < len(value) {
		builder.WriteString(value[start:])
	}
}
