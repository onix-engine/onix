package html

import (
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/layout"
	"github.com/diiyw/onix/internal/reuse"
)

func htmlNodeParent(node *dom.Node) *dom.Node   { return node.Parent }
func htmlBoxParent(box *layout.Box) *layout.Box { return box.Parent }

func htmlNearestBox(state *htmlRenderState, node *dom.Node) *layout.Box {
	ancestor := state.NodeAncestors.resolve(node, htmlNodeParent, func(node, inherited *dom.Node) *dom.Node {
		if state.boxForNode(node) != nil {
			return node
		}
		return inherited
	})
	return state.boxForNode(ancestor)
}

// Resolve shared ancestor paths once per operation. The path and index retain
// their storage between operations, so warmed traversals allocate nothing.
type htmlAncestorCache[N comparable, V any] struct {
	values    map[N]V
	retention *htmlAncestorRetention
	path      []N
}

type htmlAncestorRetention struct {
	mapRetention  reuse.MapRetention
	pathRetention reuse.Retention
}

func (cache *htmlAncestorCache[N, V]) clear() {
	if cache.retention == nil && len(cache.values) > 64 {
		cache.retention = &htmlAncestorRetention{}
	}
	if cache.retention != nil {
		cache.values = cache.retention.mapRetention.Trim(cache.values, 64)
		cache.path = cache.retention.pathRetention.Trim(cache.path, 32)
	}
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
	if cache.retention == nil && len(cache.path) > 32 {
		cache.retention = &htmlAncestorRetention{}
	}
	if cache.retention != nil {
		cache.retention.pathRetention.Record(len(cache.path))
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
