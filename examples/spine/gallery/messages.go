package gallery

import (
	"github.com/diiyw/onix/module"
)

func (m *Mod) OnMessage(c *module.Context, message module.Message) error {
	switch message.Topic {
	case "resource":
		value, ok := message.Value[int64]()
		if !ok {
			return module.ErrType
		}
		return m.OnResource(c, value)
	case "animation":
		value, ok := message.Value[int64]()
		if !ok {
			return module.ErrType
		}
		return m.OnAnimation(c, value)
	}
	return nil
}
