package ecs

import "reflect"

type resourceSlot[T any] struct {
	value   T
	added   Tick
	changed Tick
}

func InsertResource[T any](world *World, resource T) {
	requireWorld(world)
	world.mutationVersion++
	typ := reflect.TypeFor[T]()
	tick := world.mutationTick()
	if value, ok := world.resources[typ]; ok {
		slot, ok := value.(*resourceSlot[T])
		if !ok {
			panic("ecs: resource storage type mismatch")
		}
		slot.value = resource
		slot.changed = tick
		return
	}
	world.resources[typ] = &resourceSlot[T]{value: resource, added: tick, changed: tick}
}

func GetResource[T any](world *World) (T, bool) {
	var zero T
	requireWorld(world)
	slot, ok := resourceSlotFor[T](world)
	if !ok {
		return zero, false
	}
	return slot.value, true
}

// ResourceChangeTick returns the last write tick, or zero for a missing resource.
func ResourceChangeTick[T any](world *World) Tick {
	requireWorld(world)
	if slot, ok := resourceSlotFor[T](world); ok {
		return slot.changed
	}
	return 0
}

func GetResourceMut[T any](world *World) (*T, bool) {
	requireWorld(world)
	slot, ok := resourceSlotFor[T](world)
	if !ok {
		return nil, false
	}
	slot.changed = world.mutationTick()
	world.mutationVersion++
	return &slot.value, true
}

func RemoveResource[T any](world *World) (T, bool) {
	var zero T
	requireWorld(world)
	typ := reflect.TypeFor[T]()
	value, ok := world.resources[typ]
	if !ok {
		return zero, false
	}
	delete(world.resources, typ)
	world.mutationVersion++
	slot, ok := value.(*resourceSlot[T])
	if !ok {
		panic("ecs: resource storage type mismatch")
	}
	return slot.value, true
}

func ResourceAdded[T any](context *SystemContext) bool {
	slot, ok := contextResourceSlot[T](context)
	return ok && tickInRange(slot.added, context.lastRun, context.thisRun)
}

func ResourceChanged[T any](context *SystemContext) bool {
	slot, ok := contextResourceSlot[T](context)
	return ok && tickInRange(slot.changed, context.lastRun, context.thisRun)
}

func resourceSlotFor[T any](world *World) (*resourceSlot[T], bool) {
	value, ok := world.resources[reflect.TypeFor[T]()]
	if !ok {
		return nil, false
	}
	slot, ok := value.(*resourceSlot[T])
	if !ok {
		panic("ecs: resource storage type mismatch")
	}
	return slot, true
}

func contextResourceSlot[T any](context *SystemContext) (*resourceSlot[T], bool) {
	if context == nil || context.World == nil {
		return nil, false
	}
	return resourceSlotFor[T](context.World)
}
