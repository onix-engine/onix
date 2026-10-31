// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license in LICENSE.go-html.
// Adapted for Onix's slice-based DOM.

package dom

import "slices"

const scopeMarkerNode NodeType = -1

var scopeMarker = Node{Type: scopeMarkerNode}

func lastChild(n *Node) *Node {
	if len(n.Children) == 0 {
		return nil
	}
	return n.Children[len(n.Children)-1]
}

func previousSibling(n *Node) *Node {
	if n.Parent != nil {
		if index := slices.Index(n.Parent.Children, n); index > 0 {
			return n.Parent.Children[index-1]
		}
	}
	return nil
}

func (n *Node) InsertBefore(child, before *Node) {
	index := len(n.Children)
	if before != nil {
		index = slices.Index(n.Children, before)
		if index < 0 {
			panic("dom: insertion reference is not a child")
		}
	}
	n.Children = append(n.Children, nil)
	copy(n.Children[index+1:], n.Children[index:])
	n.Children[index] = child
	child.Parent = n
}

func (n *Node) RemoveChild(child *Node) {
	index := slices.Index(n.Children, child)
	if index < 0 {
		panic("dom: removal target is not a child")
	}
	copy(n.Children[index:], n.Children[index+1:])
	n.Children[len(n.Children)-1] = nil
	n.Children = n.Children[:len(n.Children)-1]
	child.Parent = nil
}

func reparentChildren(dst, src *Node) {
	for _, child := range src.Children {
		dst.AppendChild(child)
	}
	clear(src.Children)
	src.Children = src.Children[:0]
}

type nodeStack []*Node

func (s *nodeStack) pop() *Node {
	i := len(*s) - 1
	n := (*s)[i]
	*s = (*s)[:i]
	return n
}

func (s nodeStack) top() *Node {
	if len(s) == 0 {
		return nil
	}
	return s[len(s)-1]
}

func (s nodeStack) index(n *Node) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == n {
			return i
		}
	}
	return -1
}

func (s nodeStack) contains(tag string) bool {
	for _, n := range s {
		if n.Data == tag && n.namespace == "" {
			return true
		}
	}
	return false
}

func (s *nodeStack) insert(index int, n *Node) {
	*s = append(*s, nil)
	copy((*s)[index+1:], (*s)[index:])
	(*s)[index] = n
}

func (s *nodeStack) remove(n *Node) {
	if index := s.index(n); index >= 0 {
		copy((*s)[index:], (*s)[index+1:])
		(*s)[len(*s)-1] = nil
		*s = (*s)[:len(*s)-1]
	}
}

type insertionModeStack []insertionMode

func (s *insertionModeStack) pop() insertionMode {
	index := len(*s) - 1
	mode := (*s)[index]
	*s = (*s)[:index]
	return mode
}

func (s insertionModeStack) top() insertionMode {
	if len(s) == 0 {
		return nil
	}
	return s[len(s)-1]
}
