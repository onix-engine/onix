package main

import "github.com/onix-engine/onix/module"

type SceneMain struct{}

func (m *SceneMain) OnMessage(c *module.Context, message module.Message) error {
	if message.Topic == module.InputTopic {
		return c.Forward("viewer", message)
	}
	return nil
}
