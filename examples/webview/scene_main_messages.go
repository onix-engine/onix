package main

import (
	"github.com/diiyw/onix/module"
)

type SceneMain struct{}

func (m *SceneMain) OnMessage(c *module.Context, message module.Message) error {
	if message.Topic == module.InputTopic {
		if message.From == "" {
			return nil
		}
		return c.Forward(message.From, message)
	}
	return nil
}
