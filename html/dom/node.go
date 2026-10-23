package dom

import (
	"fmt"
	"sort"
	"strings"
)

type NodeType int

const (
	DocumentNode NodeType = iota
	ElementNode
	TextNode
)

type Node struct {
	Type       NodeType
	Data       string
	Attributes map[string]string
	Parent     *Node
	Children   []*Node
	States     StateSet[ElementState]
	Order      uint32
}

func NewDocument() *Node {
	return &Node{Type: DocumentNode, Data: "#document"}
}

func NewElement(tag string, attrs map[string]string) *Node {
	return &Node{Type: ElementNode, Data: tag, Attributes: attrs}
}

func NewText(text string) *Node {
	return &Node{Type: TextNode, Data: text}
}

func (n *Node) AppendChild(child *Node) {
	child.Parent = n
	n.Children = append(n.Children, child)
}

func (n *Node) Attr(name string) (string, bool) {
	if n.Attributes == nil {
		return "", false
	}

	value, ok := n.Attributes[name]
	return value, ok
}

func HasInputType(node *Node, inputType string) bool {
	if node == nil || node.Type != ElementNode || node.Data != "input" {
		return false
	}
	typeValue, ok := node.Attr("type")
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(typeValue), inputType)
}

func (n *Node) String() string {
	var builder strings.Builder
	n.writeTo(&builder, 0)
	return strings.TrimRight(builder.String(), "\n")
}

func (n *Node) writeTo(builder *strings.Builder, depth int) {
	builder.WriteString(strings.Repeat("  ", depth))

	switch n.Type {
	case DocumentNode:
		builder.WriteString(n.Data)
	case ElementNode:
		builder.WriteString("<")
		builder.WriteString(n.Data)
		for _, name := range sortedAttributeNames(n.Attributes) {
			builder.WriteString(fmt.Sprintf(" %s=%q", name, n.Attributes[name]))
		}
		builder.WriteString(">")
	case TextNode:
		builder.WriteString(fmt.Sprintf("%q", n.Data))
	}

	builder.WriteByte('\n')

	for _, child := range n.Children {
		child.writeTo(builder, depth+1)
	}
}

func sortedAttributeNames(attrs map[string]string) []string {
	if len(attrs) == 0 {
		return nil
	}

	names := make([]string, 0, len(attrs))
	for name := range attrs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
