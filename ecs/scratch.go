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

func FrameValue[Tag, T any](world *World) *T {
	requireWorld(world)
	return cachedValue[Tag, T](world.frameScratch)
}

func cachedValue[Tag, T any](values map[reflect.Type]any) *T {
	key := reflect.TypeFor[struct {
		Tag Tag
		Val *T
	}]()
	if value, ok := values[key]; ok {
		return value.(*T)
	}
	value := new(T)
	values[key] = value
	return value
}

func FrameScratch[Tag, T any](world *World) *Scratch[T] {
	requireWorld(world)
	key := reflect.TypeFor[struct {
		Tag Tag
		Val T
	}]()
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
func FrameScratchMap[Tag any, K comparable, V any](world *World) map[K]V {
	requireWorld(world)
	key := reflect.TypeFor[struct {
		Tag Tag
		Key K
		Val V
	}]()
	if value, ok := world.frameScratch[key]; ok {
		scratch := value.(map[K]V)
		clear(scratch)
		return scratch
	}
	scratch := make(map[K]V)
	world.frameScratch[key] = scratch
	return scratch
}
