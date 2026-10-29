// Package entitycache stores large system records densely behind a small entity index.
package entitycache

import (
	"slices"

	"github.com/diiyw/onix/ecs"
)

type entry[T any] struct {
	entity ecs.Entity
	value  T
}

// Store validates full entity generations. Record pointers are borrowed until
// the next Ensure, Delete or Reset; no pointers into ECS columns are retained.
type Store[T any] struct {
	indices []uint32 // one-based dense positions
	entries []entry[T]
}

func (s *Store[T]) Get(entity ecs.Entity) *T {
	index, ok := entity.Index()
	if !ok || int(index) >= len(s.indices) {
		return nil
	}
	position := s.indices[index]
	if position == 0 || s.entries[position-1].entity != entity {
		return nil
	}
	return &s.entries[position-1].value
}

func (s *Store[T]) Ensure(entity ecs.Entity) *T {
	index, ok := entity.Index()
	if !ok {
		panic("entitycache: invalid entity")
	}
	if int(index) >= len(s.indices) {
		s.indices = slices.Grow(s.indices, int(index)+1-len(s.indices))[:int(index)+1]
	}
	if position := s.indices[index]; position != 0 {
		e := &s.entries[position-1]
		if e.entity != entity {
			*e = entry[T]{entity: entity}
		}
		return &e.value
	}
	s.entries = append(s.entries, entry[T]{entity: entity})
	s.indices[index] = uint32(len(s.entries))
	return &s.entries[len(s.entries)-1].value
}

func (s *Store[T]) Delete(entity ecs.Entity) {
	index, ok := entity.Index()
	if !ok || int(index) >= len(s.indices) {
		return
	}
	position := s.indices[index]
	if position == 0 || s.entries[position-1].entity != entity {
		return
	}
	last := len(s.entries) - 1
	if int(position)-1 != last {
		s.entries[position-1] = s.entries[last]
		moved, _ := s.entries[position-1].entity.Index()
		s.indices[moved] = position
	}
	s.entries[last] = entry[T]{}
	s.entries = s.entries[:last]
	s.indices[index] = 0
}

func (s *Store[T]) Reset() {
	for i := range s.entries {
		index, _ := s.entries[i].entity.Index()
		s.indices[index] = 0
	}
	clear(s.entries)
	s.entries = s.entries[:0]
}

func (s *Store[T]) Len() int { return len(s.entries) }
