package module

import (
	"fmt"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func transformBundle(t project.ModTransform, ui bool, parent ecs.Entity, order int, visible, active bool, anchor *scene.Anchor) ecs.Bundle {
	b := ecs.NewBundle(7)
	if ui {
		b.Put(moduleUITransform(t))
		b.Put(scene.UIGlobalTransform{Matrix: mgl32.Ident4()})
	} else {
		b.Put(moduleTransform(t))
		b.Put(scene.GlobalTransform{Matrix: mgl32.Ident4()})
	}
	if anchor != nil {
		b.Put(*anchor)
	} else if ui {
		b.Put(scene.AnchorTopLeft)
	}
	if parent != 0 {
		b.Put(scene.Parent{Entity: parent})
	}
	b.Put(scene.SiblingOrder(order))
	b.Put(scene.Visibility{Visible: visible})
	b.Put(scene.Activity{Active: active})
	return b
}

func moduleUITransform(t project.ModTransform) scene.UITransform {
	local := scene.NewUITransform()
	local.Translation = scene.Px2(t.Position[0], t.Position[1])
	local.Scale = mgl32.Vec2{t.Scale[0], t.Scale[1]}
	local.Rotation = t.Rotation[2]
	return local
}

func moduleTransform(t project.ModTransform) scene.Transform {
	rotation := mgl32.QuatRotate(t.Rotation[2], mgl32.Vec3{0, 0, 1}).Mul(mgl32.QuatRotate(t.Rotation[1], mgl32.Vec3{0, 1, 0})).Mul(mgl32.QuatRotate(t.Rotation[0], mgl32.Vec3{1, 0, 0}))
	return scene.Transform{Translation: mgl32.Vec3(t.Position), Rotation: rotation, Scale: mgl32.Vec3(t.Scale)}
}

func installAlternateTransform(w *ecs.World, e ecs.Entity, t project.ModTransform, ui bool) {
	if ui && !w.Has[scene.UITransform](e) {
		_ = w.Insert2(e, moduleUITransform(t), scene.UIGlobalTransform{Matrix: mgl32.Ident4()})
	} else if !ui && !w.Has[scene.Transform](e) {
		_ = w.Insert2(e, moduleTransform(t), scene.GlobalTransform{Matrix: mgl32.Ident4()})
	}
}

type privateResources struct {
	store *project.Resources
	refs  int
}

func (r *Runtime) spawnContent(c *Context, t project.ModTransform, spec project.ModComponents, parent ecs.Entity, order int, ui bool) (ecs.Entity, error) {
	if c.resources == nil {
		shared := r.contentResources[c.definition.ID]
		if shared == nil {
			store, err := project.OpenModResources(r.options.Source, c.definition, r.options.Resources)
			if err != nil {
				return 0, err
			}
			shared = &privateResources{store: store}
			r.contentResources[c.definition.ID] = shared
		}
		shared.refs++
		c.resources = shared
	}
	b := transformBundle(t, ui, parent, order, c.visible, c.enabled, spec.Anchor)
	prepared := false
	if p := r.preparedContent; p != nil && p.index < len(p.items) {
		item := &p.items[p.index]
		p.index++
		if item.store == c.resources {
			b.Extend(item.bundle)
			prepared = true
		}
	}
	if !prepared {
		if err := r.content.appendTo(c.resources.store, &b, spec); err != nil {
			return 0, err
		}
	}

	return r.world.Spawn(b), nil
}

func (r *Runtime) installContent(c *Context) error {
	if err := r.bindContent(c, c.root, "", c.definition.Spec.Components); err != nil {
		return err
	}
	for _, element := range c.definition.Elements {
		if err := r.bindContent(c, c.elements[element.Spec.ID], element.Spec.ID, element.Spec.Components); err != nil {
			return fmt.Errorf("element %s: %w", element.Spec.ID, err)
		}
	}
	return nil
}

func (r *Runtime) bindContent(c *Context, entity ecs.Entity, name string, components project.ModComponents) error {
	if components.Collision != nil {
		if err := r.bindCollision(c, entity, name, components.Collision.Emit, components.RigidBody3D != nil); err != nil {
			return err
		}
	}
	if components.Material != nil {
		scene.RenderWorldFor(r.world).RegisterMaterial2D[project.ModMaterial2D]()
	}
	if spec := components.Input; spec != nil && spec.Pointer {
		width, height := float32(0), float32(0)
		if len(components.Size) == 2 {
			width, height = components.Size[0], components.Size[1]
		}
		if sprite, ok := r.world.Get[scene.Sprite2D](entity); ok {
			width, height = sprite.Width, sprite.Height
		}
		if err := r.world.Insert(entity, scene.InputTarget2D{Width: width, Height: height, Disabled: !c.enabled}); err != nil {
			return err
		}

	}
	if components.Animation != nil {
		a := r.world.GetMut[scene.Animator](entity)
		if a == nil || a.Clip == nil {
			return fmt.Errorf("animation component is missing")
		}
		floats, err := c.bindPrivateTracks(entity, a.Clip.FloatTracks)
		if err != nil {
			return err
		}
		textures, err := c.bindPrivateTracks(entity, a.Clip.TextureTracks)
		if err != nil {
			return err
		}
		bools, err := c.bindPrivateTracks(entity, a.Clip.BoolTracks)
		if err != nil {
			return err
		}
		a.BindTargets(entity, floats, textures, bools)
	}
	return nil
}

func (c *Context) bindPrivateTracks[T any](owner ecs.Entity, tracks []scene.AnimationTrack[T]) ([]ecs.Entity, error) {
	result := make([]ecs.Entity, len(tracks))
	for i, track := range tracks {
		e := owner
		if track.Binding.Path != "" {
			e = c.elements[track.Binding.Path]
			if e == 0 {
				return nil, fmt.Errorf("animation target %q is not private content", track.Binding.Path)
			}
		}
		switch track.Binding.Target {
		case scene.AnimationTargetSpriteTexture, scene.AnimationTargetSpriteAlpha:
			if !c.runtime.world.Has[scene.Sprite2D](e) {
				return nil, fmt.Errorf("animation target %q has no sprite", track.Binding.Path)
			}
		case scene.AnimationTargetVisibility:
			if !c.runtime.world.Has[scene.Visibility](e) {
				return nil, ErrType
			}
		default:
			if !c.runtime.world.Has[scene.UITransform](e) && !c.runtime.world.Has[scene.Transform](e) {
				return nil, ErrType
			}
		}
		result[i] = e
	}
	return result, nil
}
