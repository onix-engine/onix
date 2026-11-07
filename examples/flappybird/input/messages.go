package input

import (
	"github.com/onix-engine/onix/module"
)

func (m *Mod) OnMessage(c *module.Context, message module.Message) error {
	switch message.Topic {
	case module.InputTopic:
		value, ok := message.Value[module.Input]()
		if !ok {
			return module.ErrType
		}
		return m.Input(c, value)

	}
	return nil
}
