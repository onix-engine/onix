package ecs

import (
	"iter"
	"reflect"
)

// QueryScope confines ordinary queries to a borrowed entity list. Direct
// component access remains available for loading and lifetime management.
// Rebuild only at structural boundaries; iteration does not allocate.
type QueryScope struct {
	entities []Entity
	version  uint64
	values   map[reflect.Type]any
}

// ScopeValue keeps long-lived system caches separate for execution and
// rendering scopes. Switching scopes does not rebuild the other scope's cache.
func ScopeValue[Tag, T any](world *World) *T {
	scope := world.queryScope
	if scope == nil {
		return FrameValue[Tag, T](world)
	}
	if scope.values == nil {
		scope.values = make(map[reflect.Type]any)
	}
	return cachedValue[Tag, T](scope.values)
}

func (s *QueryScope) Reset(entities []Entity) {
	s.entities = entities
	s.version++
}

func (w *World) QueryScope() *QueryScope { return w.queryScope }

// SetQueryScope returns the previous scope. Nil restores unrestricted queries.
// A scope cannot change while a query iterator is live.
func (w *World) SetQueryScope(scope *QueryScope) *QueryScope {
	if w.activeQueryIterators != 0 {
		panic("ecs: cannot change query scope during iteration")
	}
	previous := w.queryScope
	w.queryScope = scope
	return previous
}

// QueryScopeKey identifies both the selected scope and its structural revision.
func (w *World) QueryScopeKey() (*QueryScope, uint64) {
	if w.queryScope == nil {
		return nil, 0
	}
	return w.queryScope, w.queryScope.version
}

// Entities traverses every live entity independently of the current scope.
// It is intended for rebuilding scopes, not normal system updates.
func (w *World) Entities() iter.Seq[Entity] {
	return func(yield func(Entity) bool) {
		w.activeQueryIterators++
		defer func() { w.activeQueryIterators-- }()
		for _, table := range w.archetypes {
			for _, entity := range table.entities {
				if !yield(entity) {
					return
				}
			}
		}
	}
}
