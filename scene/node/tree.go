// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license in
// ../../html/parse/LICENSE.go-html.

package node

import "slices"

func (n *Node) LastChild() *Node {
	if len(n.Children) == 0 {
		return nil
	}
	return n.Children[len(n.Children)-1]
}

func (n *Node) PreviousSibling() *Node {
	if n.Parent != nil {
		if index := slices.Index(n.Parent.Children, n); index > 0 {
			return n.Parent.Children[index-1]
		}
	}
	return nil
}

// InsertBefore inserts a detached child before a sibling. A nil sibling appends.
func (n *Node) InsertBefore(child, before *Node) {
	index := len(n.Children)
	if before != nil {
		index = slices.Index(n.Children, before)
		if index < 0 {
			panic("scene/node: insertion reference is not a child")
		}
	}
	n.Children = append(n.Children, nil)
	copy(n.Children[index+1:], n.Children[index:])
	n.Children[index] = child
	child.Parent = n
}

// RemoveChild detaches a child and reports whether it belonged to n.
func (n *Node) RemoveChild(child *Node) bool {
	index := slices.Index(n.Children, child)
	if index < 0 {
		return false
	}
	copy(n.Children[index:], n.Children[index+1:])
	n.Children[len(n.Children)-1] = nil
	n.Children = n.Children[:len(n.Children)-1]
	child.Parent = nil
	return true
}

// AdoptChildren moves all of src's children to dst in their existing order.
func (dst *Node) AdoptChildren(src *Node) {
	for _, child := range src.Children {
		dst.AppendChild(child)
	}
	clear(src.Children)
	src.Children = src.Children[:0]
}

// KeepChildren removes children rejected by keep, preserving relative order.
func (n *Node) KeepChildren(keep func(*Node) bool) {
	children := n.Children[:0]
	for _, child := range n.Children {
		if child != nil && keep(child) {
			children = append(children, child)
		} else if child != nil {
			child.Parent = nil
		}
	}
	clear(n.Children[len(children):])
	n.Children = children
}

// ClearChildren detaches all direct children while retaining list capacity.
func (n *Node) ClearChildren() {
	n.KeepChildren(func(*Node) bool { return false })
}

// Within reports whether n is root or a descendant of root.
func (n *Node) Within(root *Node) bool {
	for ; n != nil; n = n.Parent {
		if n == root {
			return true
		}
	}
	return false
}
