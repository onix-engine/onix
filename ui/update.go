package ui

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

// SetText changes an existing text visual without replacing its layout caches.
// Equal values and missing components return false and do not mark Changed.
func SetText(world *ecs.World, entity ecs.Entity, value string) bool {
	text := ecs.GetRef[scene.Text2D](world, entity)
	if text == nil || text.Value == value {
		return false
	}
	ecs.MustMut[scene.Text2D](world, entity).Value = value
	return true
}

// SetPosition changes only a UI node's translation, preserving its other transforms.
func SetPosition(world *ecs.World, entity ecs.Entity, position scene.Val2) bool {
	transform, ok := ecs.Get[scene.UITransform](world, entity)
	if !ok || transform.Translation == position {
		return false
	}
	transform.Translation = position
	return ecs.SetIfChanged(world, entity, transform)
}

// SetSize changes the node's measured Size2D. Use SetControlBounds when input
// target bounds must also change.
func SetSize(world *ecs.World, entity ecs.Entity, width, height float32) bool {
	return ecs.SetIfChanged(world, entity, scene.Size2D{Width: width, Height: height})
}
