package ecs

import "math"

type Entity uint64

// PickingID returns the non-zero entity slot encoded into GPU picking targets.
// The value remains stable for the lifetime of the entity and can be resolved
// without maintaining a per-frame map.
func (e Entity) PickingID() uint32 {
	return uint32(e)
}

// EntitySlot is a cached entity lookup bound to one World.
//
// A slot keeps the entity generation, so resolving it after the entity has
// been despawned or its index has been reused fails instead of returning a
// different entity. Create slots during setup and reuse them in hot paths.
type EntitySlot struct {
	world  *World
	entity Entity
}

// Entity returns the raw entity handle stored in the slot. Use EntityBySlot
// when the slot may have outlived a scene mutation and needs validation.
func (s EntitySlot) Entity() Entity {
	return s.entity
}

// Valid reports whether the slot contains a world-bound entity handle. It
// does not prove that the entity is still alive; resolve it through World.
func (s EntitySlot) Valid() bool {
	return s.world != nil && s.entity != 0
}

const invalidEntityIndex = math.MaxUint32

func makeEntity(index, generation uint32) Entity {
	return Entity(uint64(generation)<<32 | uint64(index+1))
}

// Index decodes the zero-based metadata slot. It does not validate liveness.
func (e Entity) Index() (uint32, bool) {
	low := uint32(e)
	if low == 0 {
		return 0, false
	}
	return low - 1, true
}

func (e Entity) generation() uint32 {
	return uint32(uint64(e) >> 32)
}
