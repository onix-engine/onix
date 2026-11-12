package dom

import (
	"fmt"
	"strings"
	"sync/atomic"
)

type NodeType int

const (
	DocumentNode NodeType = iota
	ElementNode
	TextNode
)

type Node struct {
	attributeList
	Parent    *Node
	Data      string
	namespace string
	Children  []*Node
	Type      NodeType
	Order     uint32
	States    StateSet[ElementState]
}

func NewDocument() *Node {
	return &Node{Type: DocumentNode, Data: "#document"}
}

func NewElement(tag string, attrs ...Attribute) *Node {
	node := &Node{Type: ElementNode, Data: tag}
	node.SetAttributes(attrs...)
	return node
}

func NewText(text string) *Node {
	return &Node{Type: TextNode, Data: text}
}

func (n *Node) AppendChild(child *Node) {
	child.Parent = n
	n.Children = append(n.Children, child)
}

// ShareAttributes shares source's storage without copying it. Subsequent edits
// to either node detach its attributes. A nil source clears the attributes.
// Sharing does not extend the lifetime of parser-owned names or values.
// Independent destinations may share a read-only source concurrently.
func (n *Node) ShareAttributes(source *Node) {
	if n == source {
		return
	}
	if source == nil || source.AttrCount() == 0 {
		n.attributeList = attributeList{}
		return
	}
	atomic.StoreUint32(&source.data.shared, 1)
	n.data = source.data
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
		for name, value := range n.SortedAttributes() {
			builder.WriteString(fmt.Sprintf(" %s=%q", name, value))
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
