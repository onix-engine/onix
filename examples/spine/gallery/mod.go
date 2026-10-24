package gallery

import (
	"fmt"
	"slices"

	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/examples/spine/contracts"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/spine"
	"github.com/go-gl/mathgl/mgl32"
)

type galleryItem struct {
	entity     module.Entity
	player     *spine.Player
	animations []string
}

type Mod struct {
	items               [8]galleryItem
	resource, animation module.Slot
	option              module.Emitter[contracts.AnimationOption]
	selection           module.Emitter[contracts.GallerySelection]
	selected, playing   int
	animationRevision   module.SlotRevision
}

func (m *Mod) Install(c *module.Context) error {
	var err error
	if m.resource, err = c.Slot("resource"); err != nil {
		return err
	}
	if m.animation, err = c.Slot("animation"); err != nil {
		return err
	}
	if m.option, err = module.BindEmitter[contracts.AnimationOption](c, "option"); err != nil {
		return err
	}
	if m.selection, err = module.BindEmitter[contracts.GallerySelection](c, "selection"); err != nil {
		return err
	}
	for index, name := range [...]string{"coin", "goblins", "raptor", "spineboy", "stretchyman", "stretchyman-ik", "tank", "vine"} {
		entity, err := c.Element(name)
		if err != nil {
			return err
		}
		component, ok := module.Get[scene.Spine2D](c, entity)
		if !ok || component.Player == nil {
			return fmt.Errorf("spine gallery: missing player %q", name)
		}
		m.items[index] = galleryItem{entity, component.Player, component.Player.Resource.Animations()}
	}
	m.selected, m.playing = -1, -1
	if err := module.OnSlotsChange(c, []string{"resource", "animation"}, m.applySelection); err != nil {
		return err
	}
	return m.applySelection(c)
}

func (m *Mod) OnResource(_ *module.Context, index int64) error {
	if index < 0 || index >= int64(len(m.items)) {
		return module.ErrInvalid
	}
	if err := m.resource.Set(project.SlotValue{Type: project.IntValue, Integer: index}); err != nil {
		return err
	}
	return m.animation.Set(project.SlotValue{Type: project.IntValue, Integer: -1})
}

func (m *Mod) OnAnimation(_ *module.Context, index int64) error {
	resource, err := module.ReadSlot[int64](m.resource)
	if err != nil {
		return err
	}
	if resource < 0 || resource >= int64(len(m.items)) || index < 0 || index >= int64(len(m.items[resource].animations)) {
		return module.ErrInvalid
	}
	return m.animation.Set(project.SlotValue{Type: project.IntValue, Integer: index})
}

func (m *Mod) applySelection(c *module.Context) error {
	resource, err := module.ReadSlot[int64](m.resource)
	if err != nil {
		return err
	}
	animation, err := module.ReadSlot[int64](m.animation)
	if err != nil {
		return err
	}
	if resource < 0 || resource >= int64(len(m.items)) {
		return module.ErrInvalid
	}
	item := &m.items[resource]
	changedResource := m.selected != int(resource)
	if changedResource && m.selected >= 0 && m.animation.Revision() == m.animationRevision {
		animation = -1
	}
	if animation == -1 {
		animation = int64(max(0, slices.Index(item.animations, item.player.Animation)))
		if err := m.animation.Set(project.SlotValue{Type: project.IntValue, Integer: animation}); err != nil {
			return err
		}
	}
	if animation < 0 || animation >= int64(len(item.animations)) {
		return module.ErrInvalid
	}
	if !changedResource && m.playing == int(animation) {
		return nil
	}
	if changedResource {
		for index, entry := range m.items {
			if err := module.Set(c, entry.entity, scene.Visibility{Visible: index == int(resource)}); err != nil {
				return err
			}
		}
		for index, name := range item.animations {
			if err := m.option.Emit(contracts.AnimationOption{Index: int64(index), Label: name}); err != nil {
				return err
			}
		}
	}
	if err := item.player.SetAnimation(item.animations[animation], true); err != nil {
		return err
	}
	item.player.Update(0)
	if err := fitPlayer(c, *item); err != nil {
		return err
	}
	m.selected, m.playing = int(resource), int(animation)
	m.animationRevision = m.animation.Revision()
	return m.selection.Emit(contracts.GallerySelection{Resource: resource, Animation: animation, Count: int64(len(item.animations))})
}

func fitPlayer(c *module.Context, item galleryItem) error {
	minX, minY, maxX, maxY, ok := item.player.AnimationBounds()
	if !ok || maxX <= minX || maxY <= minY {
		return fmt.Errorf("spine gallery: animation %q has no drawable bounds", item.player.Animation)
	}
	scale := min(900/(maxX-minX), 600/(maxY-minY), 1.6)
	transform, ok := module.Get[scene.UITransform](c, item.entity)
	if !ok {
		return module.ErrType
	}
	transform.Translation = scene.Px2(-(minX+maxX)*scale*.5, (minY+maxY)*scale*.5)
	transform.Scale = mgl32.Vec2{scale, scale}
	return module.Set(c, item.entity, transform)
}
