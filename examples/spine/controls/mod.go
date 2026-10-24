package controls

import (
	"fmt"
	"strconv"

	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/examples/spine/contracts"
	"github.com/diiyw/onix/scene"
	htmlui "github.com/diiyw/onix/ui/html"
)

type Mod struct {
	resource, animation                        module.Emitter[int64]
	resourceNode, animationNode, template      module.HTMLNode
	options                                    []module.HTMLNode
	selectedResource, selectedAnimation, count int
}

func (m *Mod) Install(c *module.Context) error {
	var err error
	if m.resource, err = module.BindEmitter[int64](c, "resource"); err != nil {
		return err
	}
	if m.animation, err = module.BindEmitter[int64](c, "animation"); err != nil {
		return err
	}
	root, err := module.HTML(c, c.Root())
	if err != nil {
		return err
	}
	var ok bool
	if m.resourceNode, ok = root.FindByID("resource"); !ok {
		return module.ErrInvalid
	}
	if m.animationNode, ok = root.FindByID("animation"); !ok {
		return module.ErrInvalid
	}
	if m.template, ok = root.FindByID("template"); !ok {
		return module.ErrInvalid
	}
	m.template.RemoveAttribute("id")
	if !m.template.Remove() {
		return module.ErrInvalid
	}
	m.options = make([]module.HTMLNode, 0, 32)
	return nil
}

func (m *Mod) OnOption(_ *module.Context, value contracts.AnimationOption) error {
	if value.Index < 0 || value.Index > int64(len(m.options)) {
		return module.ErrInvalid
	}
	if value.Index == int64(len(m.options)) {
		option := m.template.Clone()
		option.SetAttribute("value", strconv.FormatInt(value.Index, 10))
		m.options = append(m.options, option)
	}
	if !m.options[value.Index].SetText(value.Label) {
		return module.ErrDisposed
	}
	return nil
}

func (m *Mod) OnSelection(_ *module.Context, value contracts.GallerySelection) error {
	resource, animation, count := int(value.Resource), int(value.Animation), int(value.Count)
	if resource < 0 || resource >= 8 || count < 1 || count > len(m.options) || animation < 0 || animation >= count {
		return module.ErrInvalid
	}
	for index, option := range m.options {
		if index >= count && index < m.count {
			if !option.Remove() {
				return module.ErrInvalid
			}
		} else if index < count && index >= m.count {
			if !m.animationNode.Append(option) {
				return module.ErrInvalid
			}
		}
	}
	if !m.resourceNode.SetSelectedIndex(resource) || !m.animationNode.SetSelectedIndex(animation) {
		return fmt.Errorf("spine selectors: selection unavailable")
	}
	m.selectedResource, m.selectedAnimation, m.count = resource, animation, count
	return nil
}

func (m *Mod) Input(_ *module.Context, event module.Input) error {
	if event.HTMLKind == htmlui.EventChange {
		index, err := strconv.ParseInt(event.Value, 10, 64)
		if err != nil {
			return err
		}
		switch event.HTMLID {
		case "resource":
			if index < 0 || index >= 8 {
				return module.ErrInvalid
			}
			return m.resource.Emit(index)
		case "animation":
			if index < 0 || index >= int64(m.count) {
				return module.ErrInvalid
			}
			return m.animation.Emit(index)
		}
	}
	if event.Event.Kind != scene.InputEventKeyDown {
		return nil
	}
	switch event.Event.Key {
	case input.KeyUp:
		return m.resource.Emit(int64((m.selectedResource + 7) % 8))
	case input.KeyDown:
		return m.resource.Emit(int64((m.selectedResource + 1) % 8))
	case input.KeyLeft:
		if m.count > 0 {
			return m.animation.Emit(int64((m.selectedAnimation + m.count - 1) % m.count))
		}
	case input.KeyRight:
		if m.count > 0 {
			return m.animation.Emit(int64((m.selectedAnimation + 1) % m.count))
		}
	}
	return nil
}
