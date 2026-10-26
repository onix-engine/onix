package main

import "github.com/diiyw/onix/module"

type SceneResult struct{}

func (m *SceneResult) OnMessage(c *module.Context, message module.Message) error {
	if message.Topic == "scene.switch.result" {
		result, _ := module.Value[module.SceneSwitchResult](message)
		return result.Err
	}
	if message.Topic == module.InputTopic {
		return c.Forward("input", message)
	}
	if message.Topic == "action" && message.From == "input" {
		return c.Switch(module.SceneSwitchRequest{Scene: "main.json", Retain: true})
	}
	return nil
}
