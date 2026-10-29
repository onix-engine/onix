package main

import (
	"github.com/diiyw/onix/examples/flappybird/contracts"
	"github.com/diiyw/onix/module"
)

type SceneMain struct{}

func (m *SceneMain) Resume(c *module.Context, _ module.SceneStartup) error {
	return c.Send("simulation", "action", module.Signal{})
}

func (m *SceneMain) OnMessage(c *module.Context, message module.Message) error {
	if message.Topic == "scene.switch.result" {
		result, _ := message.Value[module.SceneSwitchResult]()
		return result.Err
	}
	if message.Topic == module.InputTopic {
		if message.From == "" {
			return c.Forward("input", message)
		}
		return c.Forward(message.From, message)
	}
	if message.From == "input" && message.Topic == "action" {
		return c.ForwardAs("simulation", "action", message)
	}
	if message.From == "simulation" && message.Topic == "frame" {
		if err := c.ForwardAs("presentation", "frame", message); err != nil {
			return err
		}
		frame, ok := message.Value[contracts.FlappyFrame]()
		if !ok {
			return module.ErrType
		}
		if frame.Phase == 2 {
			return c.Switch(module.SceneSwitchRequest{Scene: "result.json", Overlay: true})
		}
	}
	return nil
}
