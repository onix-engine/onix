package module

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

const (
	uiRootReady uint8 = 1 << iota
	worldRootReady
	uiChildReady
	uiOwnedReady
	worldChildReady
	worldOwnedReady
)

type transformSpawners struct {
	ready      uint8
	uiRoot     ecs.Spawner6[scene.UITransform, scene.UIGlobalTransform, scene.Anchor, scene.SiblingOrder, scene.Visibility, scene.Activity]
	worldRoot  ecs.Spawner5[scene.Transform, scene.GlobalTransform, scene.SiblingOrder, scene.Visibility, scene.Activity]
	ui         ecs.Spawner7[scene.UITransform, scene.UIGlobalTransform, scene.Anchor, scene.SiblingOrder, scene.Visibility, scene.Activity, scene.Parent]
	uiOwned    ecs.Spawner8[scene.UITransform, scene.UIGlobalTransform, scene.Anchor, scene.SiblingOrder, scene.Visibility, scene.Activity, scene.Parent, entityOwner]
	world      ecs.Spawner6[scene.Transform, scene.GlobalTransform, scene.SiblingOrder, scene.Visibility, scene.Activity, scene.Parent]
	worldOwned ecs.Spawner7[scene.Transform, scene.GlobalTransform, scene.SiblingOrder, scene.Visibility, scene.Activity, scene.Parent, entityOwner]
}

func (r *Runtime) transformFactories() *transformSpawners {
	if r.transforms == nil {
		r.transforms = new(transformSpawners)
	}
	return r.transforms
}
func (r *Runtime) spawnUiRoot(t project.ModTransform, order int, visible, active bool) ecs.Entity {
	s := r.transformFactories()
	if s.ready&uiRootReady == 0 {
		s.uiRoot = r.world.NewSpawner6[scene.UITransform, scene.UIGlobalTransform, scene.Anchor, scene.SiblingOrder, scene.Visibility, scene.Activity]()
		s.ready |= uiRootReady
	}
	return s.uiRoot.Spawn(moduleUITransform(t), scene.UIGlobalTransform{Matrix: mgl32.Ident4()}, scene.AnchorTopLeft, scene.SiblingOrder(order), scene.Visibility{Visible: visible}, scene.Activity{Active: active})
}
func (r *Runtime) spawnWorldRoot(t project.ModTransform, order int, visible, active bool) ecs.Entity {
	s := r.transformFactories()
	if s.ready&worldRootReady == 0 {
		s.worldRoot = r.world.NewSpawner5[scene.Transform, scene.GlobalTransform, scene.SiblingOrder, scene.Visibility, scene.Activity]()
		s.ready |= worldRootReady
	}
	return s.worldRoot.Spawn(moduleTransform(t), scene.GlobalTransform{Matrix: mgl32.Ident4()}, scene.SiblingOrder(order), scene.Visibility{Visible: visible}, scene.Activity{Active: active})
}
func (r *Runtime) spawnUi(t project.ModTransform, parent ecs.Entity, order int, visible, active bool) ecs.Entity {
	s := r.transformFactories()
	if s.ready&uiChildReady == 0 {
		s.ui = r.world.NewSpawner7[scene.UITransform, scene.UIGlobalTransform, scene.Anchor, scene.SiblingOrder, scene.Visibility, scene.Activity, scene.Parent]()
		s.ready |= uiChildReady
	}
	return s.ui.Spawn(moduleUITransform(t), scene.UIGlobalTransform{Matrix: mgl32.Ident4()}, scene.AnchorTopLeft, scene.SiblingOrder(order), scene.Visibility{Visible: visible}, scene.Activity{Active: active}, scene.Parent{Entity: parent})
}
func (r *Runtime) spawnUiOwned(t project.ModTransform, parent ecs.Entity, order int, visible, active bool, c *Context) ecs.Entity {
	s := r.transformFactories()
	if s.ready&uiOwnedReady == 0 {
		s.uiOwned = r.world.NewSpawner8[scene.UITransform, scene.UIGlobalTransform, scene.Anchor, scene.SiblingOrder, scene.Visibility, scene.Activity, scene.Parent, entityOwner]()
		s.ready |= uiOwnedReady
	}
	return s.uiOwned.Spawn(moduleUITransform(t), scene.UIGlobalTransform{Matrix: mgl32.Ident4()}, scene.AnchorTopLeft, scene.SiblingOrder(order), scene.Visibility{Visible: visible}, scene.Activity{Active: active}, scene.Parent{Entity: parent}, entityOwner{context: c})
}
func (r *Runtime) spawnWorld(t project.ModTransform, parent ecs.Entity, order int, visible, active bool) ecs.Entity {
	s := r.transformFactories()
	if s.ready&worldChildReady == 0 {
		s.world = r.world.NewSpawner6[scene.Transform, scene.GlobalTransform, scene.SiblingOrder, scene.Visibility, scene.Activity, scene.Parent]()
		s.ready |= worldChildReady
	}
	return s.world.Spawn(moduleTransform(t), scene.GlobalTransform{Matrix: mgl32.Ident4()}, scene.SiblingOrder(order), scene.Visibility{Visible: visible}, scene.Activity{Active: active}, scene.Parent{Entity: parent})
}
func (r *Runtime) spawnWorldOwned(t project.ModTransform, parent ecs.Entity, order int, visible, active bool, c *Context) ecs.Entity {
	s := r.transformFactories()
	if s.ready&worldOwnedReady == 0 {
		s.worldOwned = r.world.NewSpawner7[scene.Transform, scene.GlobalTransform, scene.SiblingOrder, scene.Visibility, scene.Activity, scene.Parent, entityOwner]()
		s.ready |= worldOwnedReady
	}
	return s.worldOwned.Spawn(moduleTransform(t), scene.GlobalTransform{Matrix: mgl32.Ident4()}, scene.SiblingOrder(order), scene.Visibility{Visible: visible}, scene.Activity{Active: active}, scene.Parent{Entity: parent}, entityOwner{context: c})
}
func (r *Runtime) spawnTransform(t project.ModTransform, ui bool, parent ecs.Entity, order int, visible bool) ecs.Entity {
	if ui {
		if parent == 0 {
			return r.spawnUiRoot(t, order, visible, true)
		}
		return r.spawnUi(t, parent, order, visible, true)
	}
	if parent == 0 {
		return r.spawnWorldRoot(t, order, visible, true)
	}
	return r.spawnWorld(t, parent, order, visible, true)
}
func (r *Runtime) spawnOwnedTransform(c *Context, parent ecs.Entity, order int) ecs.Entity {
	t := project.ModTransform{Scale: [3]float32{1, 1, 1}}
	if c.ui {
		return r.spawnUiOwned(t, parent, order, c.visible, c.enabled, c)
	}
	return r.spawnWorldOwned(t, parent, order, c.visible, c.enabled, c)
}
