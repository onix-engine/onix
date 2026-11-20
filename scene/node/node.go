package node

import (
	"fmt"
	"strings"
	"sync/atomic"
)

type NodeType int8

const (
	DocumentNode NodeType = iota
	ElementNode
	TextNode
)

type Namespace uint8

const (
	HTML Namespace = iota
	MathML
	SVG
)

// Node is the ordered logical node shared by native UI content producers.
// Its address stays stable while attached, detached, or moved to another parent.
type Node struct {
	AttributeSet
	Parent    *Node
	Data      string
	Children  []*Node
	Order     uint32
	States    StateSet[uint16]
	Type      NodeType
	Namespace Namespace
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
		n.AttributeSet = AttributeSet{}
		return
	}
	atomic.StoreUint32(&source.data.shared, 1)
	n.data = source.data
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

// CopyAttributes copies into independently writable storage.
func (n *Node) CopyAttributes(source *Node) {
	n.AttributeSet.copyFrom(&source.AttributeSet)
}

// EqualAttributes compares authored attributes without exposing their storage.
func (n *Node) EqualAttributes(other *Node) bool {
	return n.AttributeSet.equal(&other.AttributeSet)
}
