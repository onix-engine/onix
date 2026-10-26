package input

import (
	engineinput "github.com/diiyw/onix/input"
	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/scene"
)

type Mod struct {
	action       module.Emitter[module.Signal]
	pointer, key bool
}

func (m *Mod) Enter(c *module.Context, _ module.SceneStartup) error {
	if state, ok := module.ContentResource[*engineinput.State](c.Content()); ok {
		m.pointer, m.key = state.Pointer().Down, state.KeyHeld(engineinput.KeySpace)
	} else {
		m.pointer, m.key = false, false
	}
	return nil
}
func (m *Mod) Resume(c *module.Context, v module.SceneStartup) error { return m.Enter(c, v) }

func (m *Mod) Install(c *module.Context) (err error) {
	m.action, err = module.BindEmitter[module.Signal](c, "action")
	return err
}

func (m *Mod) Input(_ *module.Context, value module.Input) error {
	switch value.Event.Kind {
	case scene.InputEventPress:
		if !m.pointer {
			m.pointer = true
			return m.action.Emit(module.Signal{})
		}
	case scene.InputEventRelease, scene.InputEventCancel:
		m.pointer = false
	case scene.InputEventKeyDown:
		if value.Event.Key == engineinput.KeySpace && !m.key {
			m.key = true
			return m.action.Emit(module.Signal{})
		}
	case scene.InputEventKeyUp:
		if value.Event.Key == engineinput.KeySpace {
			m.key = false
		}
	}
	return nil
}
