package ecs

import (
	"iter"
	"reflect"
	"slices"
)

// QueryScope confines ordinary queries to a borrowed entity set. Direct
// component access remains available for loading and lifetime management.
// Ordinary queries use table order; explicit Query.In lists preserve order.
// Membership is compiled at structural boundaries; warm iteration does not allocate.
type QueryScope struct {
	entities  []Entity
	version   uint64
	values    map[reflect.Type]any
	world     *World
	compiled  uint64
	structure uint64
	tables    []scopeTable
}

type scopeTable struct {
	rows   []uint64
	blocks []uint32
	count  int
}

func (s *QueryScope) bind(world *World) {
	if s.world == world && s.compiled == s.version && s.structure == world.structureVersion {
		return
	}
	for i := range s.tables {
		for _, block := range s.tables[i].blocks {
			s.tables[i].rows[block] = 0
		}
		s.tables[i].blocks = s.tables[i].blocks[:0]
		s.tables[i].count = 0
	}
	if len(s.tables) < len(world.archetypes) {
		s.tables = append(s.tables, make([]scopeTable, len(world.archetypes)-len(s.tables))...)
	}
	for _, entity := range s.entities {
		_, meta, alive := world.entityMeta(entity)
		if !alive {
			continue
		}
		table := &s.tables[meta.archetypeID]
		word := int(meta.row) / 64
		if len(table.rows) <= word {
			words := (len(world.archetypes[meta.archetypeID].entities) + 63) / 64
			table.rows = append(table.rows, make([]uint64, words-len(table.rows))...)
		}
		bit := uint64(1) << (meta.row % 64)
		if table.rows[word] == 0 {
			table.blocks = append(table.blocks, uint32(word))
		}
		if table.rows[word]&bit == 0 {
			table.rows[word] |= bit
			table.count++
		}
	}
	for i := range s.tables {
		slices.Sort(s.tables[i].blocks)
	}
	s.world, s.compiled, s.structure = world, s.version, world.structureVersion
}

// ScopeValue keeps long-lived system caches separate for execution and
// rendering scopes. Switching scopes does not rebuild the other scope's cache.
func (world *World) ScopeValue[Tag, T any]() *T {
	scope := world.queryScope
	if scope == nil {
		return world.FrameValue[Tag, T]()
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
