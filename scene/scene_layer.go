package scene

import "github.com/diiyw/onix/ecs"

// SceneLayer separates camera content and orders complete scenes before local
// world/UI order. Descendants inherit the scene root's layer.
type SceneLayer struct {
	ID, Order uint64
	Overlay   bool
}

func LayerForEntity(world *ecs.World, entity ecs.Entity) SceneLayer {
	for remaining := world.EntityCount() + 1; entity != 0 && remaining > 0; remaining-- {
		if layer, ok := ecs.Get[SceneLayer](world, entity); ok {
			return layer
		}
		parent, ok := ecs.Get[Parent](world, entity)
		if !ok {
			break
		}
		entity = parent.Entity
	}
	return SceneLayer{}
}

func sceneCameraVisible(world *ecs.World, entity ecs.Entity) bool {
	for remaining := world.EntityCount() + 1; entity != 0 && remaining > 0; remaining-- {
		if v, ok := ecs.Get[Visibility](world, entity); ok && !v.Visible {
			return false
		}
		// Scene roots pause simulation while retained backgrounds stay visible.
		// Module-local activity still disables that module's authored cameras.
		if a, ok := ecs.Get[Activity](world, entity); ok && !a.Active && !ecs.Has[SceneLayer](world, entity) {
			return false
		}
		parent, ok := ecs.Get[Parent](world, entity)
		if !ok {
			return true
		}
		entity = parent.Entity
	}
	return entity == 0
}
