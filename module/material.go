package module

import (
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene/render3d"
)

// InstallMaterial3D replaces the built-in material during this instance's
// installation and registers its concrete renderer without exposing the World.
func (c *Context) InstallMaterial3D[T graphics.Material3D](entity Entity, material *T) error {
	if !c.usable() {
		return ErrDisposed
	}
	if !c.owns(entity) {
		return ErrOwnership
	}
	if !c.installing || material == nil {
		return ErrInvalid
	}
	render3d.WorldFor(c.runtime.world).RegisterMaterial[T]()
	c.runtime.world.Remove[render3d.Material[graphics.StandardMaterial]](entity.entity)
	return c.runtime.world.Insert(entity.entity, render3d.Material[T]{Material: material})
}
