package node

import (
	"math/bits"
	"slices"
	"strings"
)

// Arena allocates stable nodes in blocks. A producer can discard the arena
// after building its tree, or reuse it once all published nodes are retired.
// Reset invalidates all previous node pointers.
type Arena struct {
	blocks [][]Node
	used   int
}

func (a *Arena) New(kind NodeType, data string) *Node {
	index := a.used
	block := bits.Len(uint(index/4+1)) - 1
	if block == len(a.blocks) {
		a.blocks = append(a.blocks, make([]Node, 4<<block))
	}
	n := &a.blocks[block][index-4*((1<<block)-1)]
	n.Type, n.Data = kind, data
	a.used++
	return n
}

// Clone copies a subtree into this arena, sharing immutable attributes.
func (a *Arena) Clone(source *Node) *Node {
	if source == nil {
		return nil
	}
	n := a.New(source.Type, source.Data)
	n.Namespace, n.States, n.Order = source.Namespace, source.States, source.Order
	n.ShareAttributes(source)
	if len(source.Children) > 0 {
		n.Children = slices.Grow(n.Children, len(source.Children))[:len(source.Children)]
		for i, child := range source.Children {
			clone := a.Clone(child)
			if clone != nil {
				clone.Parent = n
			}
			n.Children[i] = clone
		}
	}
	return n
}

func (a *Arena) Reset() {
	remaining := a.used
	for _, block := range a.blocks {
		if remaining == 0 {
			break
		}
		count := min(remaining, len(block))
		for i := range count {
			n := &block[i]
			n.AttributeSet.reset()
			clear(n.Children)
			*n = Node{AttributeSet: n.AttributeSet, Children: n.Children[:0]}
		}
		remaining -= count
	}
	a.used = 0
}

// CloneCompact copies the live subtree into a new string backing store.
// Identical strings share storage, and the result can outlive the input tree.
func (a *Arena) CloneCompact(source *Node) *Node {
	if source == nil {
		return nil
	}
	packed := make(map[string]string)
	size := collectCompactStrings(source, packed)
	var text strings.Builder
	text.Grow(size)
	for value := range packed {
		start := text.Len()
		text.WriteString(value)
		packed[value] = text.String()[start:]
	}
	return a.cloneCompact(source, packed)
}

func collectCompactStrings(node *Node, packed map[string]string) int {
	size := addCompactString(node.Data, packed)
	for name, value := range node.Attributes() {
		size += addCompactString(name, packed) + addCompactString(value, packed)
	}
	for _, child := range node.Children {
		if child != nil {
			size += collectCompactStrings(child, packed)
		}
	}
	return size
}

func addCompactString(value string, packed map[string]string) int {
	if value != "" {
		if _, exists := packed[value]; !exists {
			packed[value] = ""
			return len(value)
		}
	}
	return 0
}

func (a *Arena) cloneCompact(source *Node, packed map[string]string) *Node {
	n := a.New(source.Type, packed[source.Data])
	n.Namespace, n.States, n.Order = source.Namespace, source.States, source.Order
	if count := source.AttrCount(); count != 0 {
		var inline [smallAttributeLimit]Attribute
		if count <= len(inline) {
			attrs := inline[:0]
			for name, value := range source.Attributes() {
				attrs = append(attrs, Attribute{packed[name], packed[value]})
			}
			n.SetAttributes(attrs...)
		} else {
			storage := n.AttributeSet.writable(0)
			if storage.large == nil {
				storage.promote(count)
			}
			for name, value := range source.Attributes() {
				storage.large[packed[name]] = packed[value]
			}
		}
	}
	n.Children = slices.Grow(n.Children[:0], len(source.Children))
	for _, child := range source.Children {
		if child != nil {
			n.AppendChild(a.cloneCompact(child, packed))
		}
	}
	return n
}
