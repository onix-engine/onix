package ecs

import "reflect"

type Scratch[T any] struct {
	Values      []T
	previousLen int
}

// ReleaseUnused clears references in the portion used by the previous frame
// that was not overwritten in the current frame.
func (s *Scratch[T]) ReleaseUnused() {
	if s == nil || s.previousLen <= len(s.Values) {
		return
	}
	clear(s.Values[len(s.Values):min(s.previousLen, cap(s.Values))])
	s.previousLen = len(s.Values)
}

// Reserve truncates the buffer to empty and grows it to hold want values.
// Growth is geometric so a slowly rising entity count does not reallocate each frame.
func (s *Scratch[T]) Reserve(want int) []T {
	if cap(s.Values) < want {
		s.Values = make([]T, 0, max(want, cap(s.Values)*2))
	}
	return s.Values[:0]
}

// Distinct key types keep persistent values, slices and maps independent even
// when they share tags and their element types contain pointers.
type valueCacheKey[Tag, T any] struct{}
type sliceCacheKey[Tag, T any] struct{}
type mapCacheKey[Tag any, K comparable, V any] struct{}

func (world *World) FrameValue[Tag, T any]() *T {
	requireWorld(world)
	return cachedValue[Tag, T](world.frameScratch)
}

func cachedValue[Tag, T any](values map[reflect.Type]any) *T {
	key := reflect.TypeFor[valueCacheKey[Tag, T]]()
	if value, ok := values[key]; ok {
		return value.(*T)
	}
	value := new(T)
	values[key] = value
	return value
}

func (world *World) FrameScratch[Tag, T any]() *Scratch[T] {
	requireWorld(world)
	key := reflect.TypeFor[sliceCacheKey[Tag, T]]()
	if value, ok := world.frameScratch[key]; ok {
		scratch := value.(*Scratch[T])
		scratch.previousLen = max(scratch.previousLen, len(scratch.Values))
		scratch.Values = scratch.Values[:0]
		return scratch
	}
	scratch := &Scratch[T]{}
	world.frameScratch[key] = scratch
	return scratch
}

// FrameScratchMap returns a per-frame map that is cleared on every call. Maps
// are reference values, so the stored map is the one callers mutate.
func (world *World) FrameScratchMap[Tag any, K comparable, V any]() map[K]V {
	requireWorld(world)
	key := reflect.TypeFor[mapCacheKey[Tag, K, V]]()
	if value, ok := world.frameScratch[key]; ok {
		scratch := value.(map[K]V)
		clear(scratch)
		return scratch
	}
	scratch := make(map[K]V)
	world.frameScratch[key] = scratch
	return scratch
}
