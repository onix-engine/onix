package presentation

import (
	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/examples/flappybird/contracts"
)

func (m *Mod) OnMessage(c *module.Context, message module.Message) error {
	switch message.Topic {
	case "frame":
		value, ok := module.Value[contracts.FlappyFrame](message)
		if !ok {
			return module.ErrType
		}
		return m.OnFrame(c, value)
	}
	return nil
}
