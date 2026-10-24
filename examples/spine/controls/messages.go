package controls

import (
	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/examples/spine/contracts"
)

func (m *Mod) OnMessage(c *module.Context, message module.Message) error {
	switch message.Topic {
	case module.InputTopic:
		value, ok := module.Value[module.Input](message)
		if !ok {
			return module.ErrType
		}
		return m.Input(c, value)
	case "option":
		value, ok := module.Value[contracts.AnimationOption](message)
		if !ok {
			return module.ErrType
		}
		return m.OnOption(c, value)
	case "selection":
		value, ok := module.Value[contracts.GallerySelection](message)
		if !ok {
			return module.ErrType
		}
		return m.OnSelection(c, value)
	}
	return nil
}
