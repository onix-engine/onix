package html

import (
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/layout"
)

func htmlNodeParent(node *dom.Node) *dom.Node   { return node.Parent }
func htmlBoxParent(box *layout.Box) *layout.Box { return box.Parent }

func htmlNearestBox(state *htmlRenderState, node *dom.Node) *layout.Box {
	ancestor := state.NodeAncestors.resolve(node, htmlNodeParent, func(node, inherited *dom.Node) *dom.Node {
		if state.BoxesByNode[node] != nil {
			return node
		}
		return inherited
	})
	return state.BoxesByNode[ancestor]
}

// Resolve shared ancestor paths once per operation. The path and index retain
// their storage between operations, so warmed traversals allocate nothing.
type htmlAncestorCache[N comparable, V any] struct {
	values map[N]V
	path   []N
}

func (cache *htmlAncestorCache[N, V]) clear() {
	clear(cache.values)
}

func (cache *htmlAncestorCache[N, V]) resolve(node N, parent func(N) N, inherit func(N, V) V) V {
	var zero N
	var value V
	if cache.values == nil {
		cache.values = make(map[N]V)
	}
	for node != zero {
		if cached, ok := cache.values[node]; ok {
			value = cached
			break
		}
		cache.path = append(cache.path, node)
		node = parent(node)
	}
	for i := len(cache.path) - 1; i >= 0; i-- {
		node = cache.path[i]
		value = inherit(node, value)
		cache.values[node] = value
		cache.path[i] = zero
	}
	cache.path = cache.path[:0]
	return value
}
