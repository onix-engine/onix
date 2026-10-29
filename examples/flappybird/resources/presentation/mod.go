package presentation

import (
	"fmt"

	"github.com/diiyw/onix/examples/flappybird/contracts"
	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/scene"
)

type Mod struct {
	positions     [5]module.Entity
	visibility    [2]module.Entity
	score, status module.Entity
	last          contracts.FlappyFrame
	applied       bool
}

func (m *Mod) Install(c *module.Context) error {
	for index, name := range [...]string{"bird", "pipeTop", "pipeBottom", "groundA", "groundB"} {
		entity, err := c.Element(name)
		if err != nil {
			return err
		}
		m.positions[index] = entity
	}
	for index, name := range [...]string{"getReady", "start"} {
		entity, err := c.Element(name)
		if err != nil {
			return err
		}
		m.visibility[index] = entity
	}
	var err error
	if m.score, err = c.Element("scoreLabel"); err != nil {
		return err
	}
	m.status, err = c.Element("status")
	return err
}

func (m *Mod) OnFrame(c *module.Context, value contracts.FlappyFrame) error {
	if value.Phase < 0 || value.Phase > 2 || value.Score < 0 {
		return module.ErrInvalid
	}
	if m.applied && m.last == value {
		return nil
	}
	const offset = float32((801 + 300) * .5)
	positions := [5][2]float32{
		{value.Bird[0] + 375, 667 - value.Bird[1]},
		{value.Pipes[0] + 375, 667 - value.Pipes[1] - offset},
		{value.Pipes[0] + 375, 667 - value.Pipes[1] + offset},
		{float32(value.Ground) + 375, 1190.5},
		{float32(value.Ground) + 1215, 1190.5},
	}
	for index, position := range positions {
		entity := m.positions[index]
		transform, ok := c.Get[scene.UITransform](entity)
		if !ok {
			return module.ErrType
		}
		if transform.Translation.X.Value != position[0] || transform.Translation.Y.Value != position[1] {
			transform.Translation = scene.Px2(position[0], position[1])
			if err := c.Set(entity, transform); err != nil {
				return err
			}
		}
	}
	if !m.applied || m.last.Phase != value.Phase {
		for index, visible := range [...]bool{value.Phase == 0, value.Phase == 0} {
			if err := c.Set(m.visibility[index], scene.Visibility{Visible: visible}); err != nil {
				return err
			}
		}
		status := "Space / tap to flap"
		if value.Phase == 2 {
			status = "Game over - tap to restart"
		}
		text, ok := c.Get[scene.Text2D](m.status)
		if !ok {
			return module.ErrType
		}
		if text.Value != status {
			text.Value = status
			if err := c.Set(m.status, text); err != nil {
				return err
			}
		}
	}
	if !m.applied || m.last.Score != value.Score {
		text, ok := c.Get[scene.Text2D](m.score)
		if !ok {
			return module.ErrType
		}
		text.Value = fmt.Sprintf("Score: %d", value.Score)
		if err := c.Set(m.score, text); err != nil {
			return err
		}
	}
	m.last, m.applied = value, true
	return nil
}
