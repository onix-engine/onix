package ui

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

// SetText changes an existing text visual without replacing its layout caches.
// Equal values and missing components return false and do not mark Changed.
func SetText(world *ecs.World, entity ecs.Entity, value string) bool {
	text := world.GetRef[scene.Text2D](entity)
	if text == nil || text.Value == value {
		return false
	}
	world.MustMut[scene.Text2D](entity).Value = value
	return true
}

// SetPosition changes only a UI node's translation, preserving its other transforms.
func SetPosition(world *ecs.World, entity ecs.Entity, position scene.Val2) bool {
	transform, ok := world.Get[scene.UITransform](entity)
	if !ok || transform.Translation == position {
		return false
	}
	transform.Translation = position
	return world.SetIfChanged(entity, transform)
}

// SetSize changes the node's measured Size2D. Use SetControlBounds when input
// target bounds must also change.
func SetSize(world *ecs.World, entity ecs.Entity, width, height float32) bool {
	return world.SetIfChanged(entity, scene.Size2D{Width: width, Height: height})
}
