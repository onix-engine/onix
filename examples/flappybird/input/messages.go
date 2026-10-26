package input

import (
	"github.com/diiyw/onix/module"
)

func (m *Mod) OnMessage(c *module.Context, message module.Message) error {
	switch message.Topic {
	case module.InputTopic:
		value, ok := module.Value[module.Input](message)
		if !ok {
			return module.ErrType
		}
		return m.Input(c, value)

	}
	return nil
}
