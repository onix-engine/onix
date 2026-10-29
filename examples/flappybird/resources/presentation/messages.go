package presentation

import (
	"github.com/diiyw/onix/examples/flappybird/contracts"
	"github.com/diiyw/onix/module"
)

func (m *Mod) OnMessage(c *module.Context, message module.Message) error {
	switch message.Topic {
	case "frame":
		value, ok := message.Value[contracts.FlappyFrame]()
		if !ok {
			return module.ErrType
		}
		return m.OnFrame(c, value)
	}
	return nil
}
