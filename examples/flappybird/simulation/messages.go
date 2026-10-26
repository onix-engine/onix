package simulation

import (
	"github.com/diiyw/onix/module"
)

func (m *Mod) OnMessage(c *module.Context, message module.Message) error {
	switch message.Topic {
	case "action":
		value, ok := module.Value[module.Signal](message)
		if !ok {
			return module.ErrType
		}
		return m.OnAction(c, value)
	}
	return nil
}
