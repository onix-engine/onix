package simulation

import (
	"github.com/diiyw/onix/examples/flappybird/contracts"
	"github.com/diiyw/onix/module"
)

type Mod struct {
	state     *flappyState
	start     module.Slot
	frame     module.Emitter[contracts.FlappyFrame]
	last      contracts.FlappyFrame
	published bool
}

func (m *Mod) Install(c *module.Context) error {
	var err error
	if m.start, err = c.Slot("birdStart"); err != nil {
		return err
	}
	if m.frame, err = c.BindEmitter[contracts.FlappyFrame]("frame"); err != nil {
		return err
	}
	if err := module.OnSlotsChange(c, []string{"birdStart"}, m.reset); err != nil {
		return err
	}
	return m.reset(c)
}

func (m *Mod) reset(_ *module.Context) error {
	start, err := m.start.Read[[2]float32]()
	if err != nil {
		return err
	}
	position := flappyWorldPosition(start[0], start[1])
	if m.state == nil {
		m.state = newFlappyState(position)
	} else {
		m.state.birdStart = position
		m.state.reset()
	}
	return m.publish()
}

func (m *Mod) OnAction(_ *module.Context, _ module.Signal) error {
	m.state.action()
	return m.publish()
}

func (m *Mod) Update(_ *module.Context, tick module.Tick) error {
	m.state.update(float64(tick.Delta))
	return m.publish()
}

func (m *Mod) publish() error {
	state := m.state.snapshot()
	frame := contracts.FlappyFrame{
		Bird:   [2]float32{state.birdX, state.birdY},
		Pipes:  [2]float32{state.pipeX, state.gapCenter},
		Ground: float64(state.groundX), Score: int64(state.score), Phase: int64(state.phase),
	}
	if m.published && m.last == frame {
		return nil
	}
	if err := m.frame.Emit(frame); err != nil {
		return err
	}
	m.last, m.published = frame, true
	return nil
}

func (m *Mod) Destroy(_ *module.Context) error {
	m.state = nil
	return nil
}
