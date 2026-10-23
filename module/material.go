package module

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene/render3d"
)

// InstallMaterial3D replaces the built-in material during this instance's
// installation and registers its concrete renderer without exposing the World.
func InstallMaterial3D[T graphics.Material3D](c *Context, entity Entity, material *T) error {
	if !c.usable() {
		return ErrDisposed
	}
	if !c.owns(entity) {
		return ErrOwnership
	}
	if !c.installing || material == nil {
		return ErrInvalid
	}
	render3d.RegisterMaterial[T](c.runtime.world)
	ecs.Remove[render3d.Material[graphics.StandardMaterial]](c.runtime.world, entity.entity)
	return ecs.Insert(c.runtime.world, entity.entity, render3d.Material[T]{Material: material})
}
