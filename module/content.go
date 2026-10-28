package module

import (
	"fmt"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func transformBundle(t project.ModTransform, ui bool, parent ecs.Entity, order int, visible, active bool) ecs.Bundle {
	b := ecs.NewBundle(7)
	if ui {
		local := scene.NewUITransform()
		local.Translation = scene.Px2(t.Position[0], t.Position[1])
		local.Scale = mgl32.Vec2{t.Scale[0], t.Scale[1]}
		local.Rotation = t.Rotation[2]
		ecs.Put(&b, local)
		ecs.Put(&b, scene.UIGlobalTransform{Matrix: mgl32.Ident4()})
		ecs.Put(&b, scene.AnchorTopLeft)
	} else {
		rotation := mgl32.QuatRotate(t.Rotation[2], mgl32.Vec3{0, 0, 1}).Mul(mgl32.QuatRotate(t.Rotation[1], mgl32.Vec3{0, 1, 0})).Mul(mgl32.QuatRotate(t.Rotation[0], mgl32.Vec3{1, 0, 0}))
		ecs.Put(&b, scene.Transform{Translation: mgl32.Vec3(t.Position), Rotation: rotation, Scale: mgl32.Vec3(t.Scale)})
		ecs.Put(&b, scene.GlobalTransform{Matrix: mgl32.Ident4()})
	}
	if parent != 0 {
		ecs.Put(&b, scene.Parent{Entity: parent})
	}
	ecs.Put(&b, scene.SiblingOrder(order))
	ecs.Put(&b, scene.Visibility{Visible: visible})
	ecs.Put(&b, scene.Activity{Active: active})
	return b
}

func (r *Runtime) spawnTransform(t project.ModTransform, ui bool, parent ecs.Entity, order int, visible bool) ecs.Entity {
	return r.world.Spawn(transformBundle(t, ui, parent, order, visible, true))
}

func installAlternateTransform(w *ecs.World, e ecs.Entity, t project.ModTransform, ui bool) {
	if ui && !ecs.Has[scene.UITransform](w, e) {
		local := scene.NewUITransform()
		local.Translation = scene.Px2(t.Position[0], t.Position[1])
		local.Scale = mgl32.Vec2{t.Scale[0], t.Scale[1]}
		local.Rotation = t.Rotation[2]
		_ = ecs.Insert(w, e, local)
		_ = ecs.Insert(w, e, scene.UIGlobalTransform{Matrix: mgl32.Ident4()})
	} else if !ui && !ecs.Has[scene.Transform](w, e) {
		rotation := mgl32.QuatRotate(t.Rotation[2], mgl32.Vec3{0, 0, 1}).Mul(mgl32.QuatRotate(t.Rotation[1], mgl32.Vec3{0, 1, 0})).Mul(mgl32.QuatRotate(t.Rotation[0], mgl32.Vec3{1, 0, 0}))
		_ = ecs.Insert(w, e, scene.Transform{Translation: mgl32.Vec3(t.Position), Rotation: rotation, Scale: mgl32.Vec3(t.Scale)})
		_ = ecs.Insert(w, e, scene.GlobalTransform{Matrix: mgl32.Ident4()})
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
	b := transformBundle(t, ui, parent, order, c.visible, c.enabled)
	if r.options.Content == nil {
		var content ecs.Bundle
		prepared := false
		if p := r.preparedContent; p != nil && p.index < len(p.items) {
			item := &p.items[p.index]
			p.index++
			if item.store == c.resources {
				content, prepared = item.bundle, true
			}
		}
		if !prepared {
			var err error
			content, err = c.resources.store.ModContent(spec)
			if err != nil {
				return 0, err
			}
		}
		b.Extend(content)
	}
	entity := r.world.Spawn(b)
	if spec.Anchor != nil {
		_ = ecs.Insert(r.world, entity, *spec.Anchor)
	}
	return entity, nil
}

func (r *Runtime) installContent(c *Context) error {
	if r.options.Content != nil {
		if err := r.options.Content(c, c.definition.Spec.Components); err != nil {
			return err
		}
	}
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
		scene.RegisterMaterial2D[project.ModMaterial2D](r.world)
	}
	if spec := components.Input; spec != nil && spec.Pointer {
		width, height := float32(0), float32(0)
		if len(components.Size) == 2 {
			width, height = components.Size[0], components.Size[1]
		}
		if sprite, ok := ecs.Get[scene.Sprite2D](r.world, entity); ok {
			width, height = sprite.Width, sprite.Height
		}
		if err := ecs.Insert(r.world, entity, scene.InputTarget2D{Width: width, Height: height, Disabled: !c.enabled}); err != nil {
			return err
		}

	}
	if components.Animation != nil {
		a := ecs.GetMut[scene.Animator](r.world, entity)
		if a == nil || a.Clip == nil {
			return fmt.Errorf("animation component is missing")
		}
		floats, err := bindPrivateTracks(c, entity, a.Clip.FloatTracks)
		if err != nil {
			return err
		}
		textures, err := bindPrivateTracks(c, entity, a.Clip.TextureTracks)
		if err != nil {
			return err
		}
		bools, err := bindPrivateTracks(c, entity, a.Clip.BoolTracks)
		if err != nil {
			return err
		}
		a.BindTargets(entity, floats, textures, bools)
	}
	return nil
}

func bindPrivateTracks[T any](c *Context, owner ecs.Entity, tracks []scene.AnimationTrack[T]) ([]ecs.Entity, error) {
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
			if !ecs.Has[scene.Sprite2D](c.runtime.world, e) {
				return nil, fmt.Errorf("animation target %q has no sprite", track.Binding.Path)
			}
		case scene.AnimationTargetVisibility:
			if !ecs.Has[scene.Visibility](c.runtime.world, e) {
				return nil, ErrType
			}
		default:
			if !ecs.Has[scene.UITransform](c.runtime.world, e) && !ecs.Has[scene.Transform](c.runtime.world, e) {
				return nil, ErrType
			}
		}
		result[i] = e
	}
	return result, nil
}
