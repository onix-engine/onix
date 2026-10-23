package project

import (
	"fmt"
	"reflect"

	"github.com/diiyw/onix/ecs"
)

// Component resolves a named scene node and acquires write access. Use
// RequireComponent once at installation and slot access for repeated updates.
func Component[T any](scene *SceneInstance, id string) (*T, error) {
	if scene == nil || scene.World == nil {
		return nil, fmt.Errorf("scene instance is nil")
	}
	entity, err := scene.Entity(id)
	if err != nil {
		return nil, err
	}
	value := ecs.GetMut[T](scene.World, entity)
	if value == nil {
		return nil, fmt.Errorf("scene node %q has no component %s", id, reflect.TypeFor[T]())
	}
	return value, nil
}

// RequireComponent validates a named node without marking its component changed.
// The returned slot rejects retired entities and slots used in another world.
func RequireComponent[T any](scene *SceneInstance, id string) (ecs.EntitySlot, error) {
	if scene == nil || scene.World == nil {
		return ecs.EntitySlot{}, fmt.Errorf("scene instance is nil")
	}
	entity, err := scene.Entity(id)
	if err != nil {
		return ecs.EntitySlot{}, err
	}
	if !ecs.Has[T](scene.World, entity) {
		return ecs.EntitySlot{}, fmt.Errorf("scene node %q has no component %s", id, reflect.TypeFor[T]())
	}
	return scene.World.Slot(entity), nil
}
