package main

import (
	"github.com/diiyw/onix/module"
)

type SceneMain struct{}

func (m *SceneMain) OnMessage(c *module.Context, message module.Message) error {
	if message.Topic == module.InputTopic {
		if message.From == "" {
			if err := c.Forward("controls", message); err != nil {
				return err
			}
			return nil
		}
		return c.Forward(message.From, message)
	}
	if message.From == "controls" && message.Topic == "resource" {
		return c.ForwardAs("gallery", "resource", message)
	}
	if message.From == "controls" && message.Topic == "animation" {
		return c.ForwardAs("gallery", "animation", message)
	}
	if message.From == "gallery" && message.Topic == "option" {
		return c.ForwardAs("controls", "option", message)
	}
	if message.From == "gallery" && message.Topic == "selection" {
		return c.ForwardAs("controls", "selection", message)
	}
	return nil
}
