package ecs

import (
	"iter"
	"math/bits"
	"reflect"
	"slices"
)

// QueryScope confines ordinary queries to a borrowed entity set. Direct
// component access remains available for loading and lifetime management.
// Ordinary queries use table order; explicit Query.In lists preserve order.
// Membership is compiled at structural boundaries; warm iteration does not allocate.
type QueryScope struct {
	values     map[reflect.Type]any
	world      *World
	entities   []Entity
	tables     []scopeTable
	version    uint64
	compiled   uint64
	structure  uint64
	firstTable uint32 // linked nonempty scope tables, using table ID + 1
	pending    bool   // a currently non-live handle may become live after binding
}

type scopeTable struct {
	rows   []uint64
	blocks []uint32
	count  uint32
	next   uint32
}

func (s *QueryScope) bind(world *World) {
	if s.world == world && s.compiled == s.version {
		if s.structure == world.structureVersion {
			return
		}
		if !s.pending && s.rowsUnchanged(world) {
			s.structure = world.structureVersion
			return
		}
	}
	for link := s.firstTable; link != 0; {
		table := &s.tables[link-1]
		for _, block := range table.blocks {
			table.rows[block] = 0
		}
		link = table.next
		table.blocks = table.blocks[:0]
		table.count, table.next = 0, 0
	}
	s.firstTable = 0
	if len(s.tables) < len(world.archetypes) {
		s.tables = append(s.tables, make([]scopeTable, len(world.archetypes)-len(s.tables))...)
	}
	s.pending = false
	for _, entity := range s.entities {
		_, meta, alive := world.entityMeta(entity)
		if !alive {
			// Retired generations cannot rejoin; reserved or not-yet-issued
			// handles still require a full check after structural changes.
			if index, valid := entity.Index(); valid && (int(index) >= len(world.entities) || world.entities[index].generation == entity.generation()) {
				s.pending = true
			}
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
			if table.count == 0 {
				table.next, s.firstTable = s.firstTable, meta.archetypeID+1
			}
			table.blocks = append(table.blocks, uint32(word))
		}
		if table.rows[word]&bit == 0 {
			table.rows[word] |= bit
			table.count++
		}
	}
	for link := s.firstTable; link != 0; link = s.tables[link-1].next {
		slices.Sort(s.tables[link-1].blocks)
	}
	s.world, s.compiled, s.structure = world, s.version, world.structureVersion
}

// Appending outsiders preserves existing row identities. Only removals from
// a table containing members can invalidate this scope's compiled row bits.
func (s *QueryScope) rowsUnchanged(world *World) bool {
	for link := s.firstTable; link != 0; link = s.tables[link-1].next {
		if world.archetypes[link-1].rowVersion > s.structure {
			return false
		}
	}
	return true
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

// InQueryScope reports whether a live entity belongs to the current query
// scope. It lets hierarchy traversals honor scopes without scanning them.
func (w *World) InQueryScope(entity Entity) bool {
	_, meta, alive := w.entityMeta(entity)
	if !alive || w.queryScope == nil {
		return alive
	}
	scope := w.queryScope
	scope.bind(w)
	if int(meta.archetypeID) >= len(scope.tables) {
		return false
	}
	table := &scope.tables[meta.archetypeID]
	word := int(meta.row) / 64
	return word < len(table.rows) && table.rows[word]&(uint64(1)<<(meta.row%64)) != 0
}

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
		for block, word := range w.occupiedArchetypes {
			for word != 0 {
				table := w.archetypes[block*64+bits.TrailingZeros64(word)]
				word &= word - 1
				for _, entity := range table.entities {
					if !yield(entity) {
						return
					}
				}
			}
		}
	}
}
