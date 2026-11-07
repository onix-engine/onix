package presentation

import (
	"github.com/onix-engine/onix/examples/flappybird/contracts"
	"github.com/onix-engine/onix/module"
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
