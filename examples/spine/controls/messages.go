package controls

import (
	"github.com/diiyw/onix/examples/spine/contracts"
	"github.com/diiyw/onix/module"
)

func (m *Mod) OnMessage(c *module.Context, message module.Message) error {
	switch message.Topic {
	case module.InputTopic:
		value, ok := message.Value[module.Input]()
		if !ok {
			return module.ErrType
		}
		return m.Input(c, value)
	case "option":
		value, ok := message.Value[contracts.AnimationOption]()
		if !ok {
			return module.ErrType
		}
		return m.OnOption(c, value)
	case "selection":
		value, ok := message.Value[contracts.GallerySelection]()
		if !ok {
			return module.ErrType
		}
		return m.OnSelection(c, value)
	}
	return nil
}
