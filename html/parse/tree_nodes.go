// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license in LICENSE.go-html.
// Adapted for Onix's slice-based DOM.

package parse

import (
	uinode "github.com/onix-engine/onix/scene/node"
)

const scopeMarkerNode uinode.NodeType = -1

var scopeMarker = uinode.Node{Type: scopeMarkerNode}

type nodeStack []*uinode.Node

func (s *nodeStack) pop() *uinode.Node {
	i := len(*s) - 1
	n := (*s)[i]
	*s = (*s)[:i]
	return n
}

func (s nodeStack) top() *uinode.Node {
	if len(s) == 0 {
		return nil
	}
	return s[len(s)-1]
}

func (s nodeStack) index(n *uinode.Node) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == n {
			return i
		}
	}
	return -1
}

func (s nodeStack) contains(tag string) bool {
	for _, n := range s {
		if n.Data == tag && n.Namespace == uinode.HTML {
			return true
		}
	}
	return false
}

func (s *nodeStack) insert(index int, n *uinode.Node) {
	*s = append(*s, nil)
	copy((*s)[index+1:], (*s)[index:])
	(*s)[index] = n
}

func (s *nodeStack) remove(n *uinode.Node) {
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
