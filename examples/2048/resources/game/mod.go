package game

import (
	"fmt"
	"math"
	"math/bits"
	"math/rand/v2"
	"strconv"

	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/scene"
	htmlui "github.com/diiyw/onix/ui/html"
)

var tileViews = func() [32]struct{ text, class, spawned string } {
	var views [32]struct{ text, class, spawned string }
	views[0].text, views[0].class = "0", "tile empty"
	for exponent := 1; exponent < len(views); exponent++ {
		text := strconv.FormatUint(uint64(1)<<exponent, 10)
		class := "tile v" + strconv.Itoa(min(exponent, 12))
		if exponent >= 10 {
			class += " small"
		}
		if exponent >= 14 {
			class += " tiny"
		}
		views[exponent].text = text
		views[exponent].class = class
		views[exponent].spawned = class
	}
	return views
}()

// Mod keeps the board and DOM handles for one independent game instance.
// Input drives updates; there is no per-frame polling or board rebuilding.
type Mod struct {
	game
	tiles, tileParents                        [16]module.HTMLNode
	score, best, moves, undoButton            module.HTMLNode
	overlay, title, description, keep, status module.HTMLNode
	pressX, pressY                            float32
	dragging                                  bool
}

func (m *Mod) Install(c *module.Context) error {
	document, err := module.HTML(c, c.Root())
	if err != nil {
		return err
	}
	for i := range m.tiles {
		id := "cell-" + strconv.Itoa(i)
		parent, ok := document.FindByID(id)
		if !ok {
			return fmt.Errorf("2048: missing HTML node %q", id)
		}
		value, ok := document.FindByID("value-" + strconv.Itoa(i))
		if !ok {
			return fmt.Errorf("2048: missing HTML node %q", "value-"+strconv.Itoa(i))
		}
		m.tileParents[i], m.tiles[i] = parent, value
	}
	for _, binding := range []struct {
		id   string
		node *module.HTMLNode
	}{
		{"score", &m.score}, {"best", &m.best}, {"moves", &m.moves},
		{"undo", &m.undoButton}, {"overlay", &m.overlay}, {"result-title", &m.title},
		{"result-description", &m.description}, {"continue", &m.keep}, {"status", &m.status},
	} {
		node, ok := document.FindByID(binding.id)
		if !ok {
			return fmt.Errorf("2048: missing HTML node %q", binding.id)
		}
		*binding.node = node
	}
	m.random = *rand.NewPCG(rand.Uint64(), rand.Uint64())
	m.reset()
	m.render()
	return nil
}

func (m *Mod) OnMessage(_ *module.Context, message module.Message) error {
	if message.Topic != module.InputTopic {
		return nil
	}
	event, ok := module.Value[module.Input](message)
	if !ok {
		return module.ErrType
	}
	m.handleInput(event)
	return nil
}

func (m *Mod) handleInput(event module.Input) {
	if event.HTMLKind == htmlui.EventClick {
		switch event.HTMLID {
		case "restart", "retry":
			m.reset()
		case "undo":
			m.undo()
		case "continue":
			m.KeepPlaying = true
		case "left":
			m.move(left)
		case "right":
			m.move(right)
		case "up":
			m.move(up)
		case "down":
			m.move(down)
		default:
			return
		}
		m.render()
		return
	}
	e := event.Event
	switch e.Kind {
	case scene.InputEventKeyDown:
		switch e.Key {
		case input.KeyLeft, input.KeyA:
			m.move(left)
		case input.KeyRight, input.KeyD:
			m.move(right)
		case input.KeyUp, input.KeyW:
			m.move(up)
		case input.KeyDown, input.KeyS:
			m.move(down)
		case input.KeyR:
			m.reset()
		case input.KeyZ:
			m.undo()
		case input.KeyEnter:
			if m.Won && !m.Lost {
				m.KeepPlaying = true
			}
		default:
			return
		}
		m.render()
	case scene.InputEventPress:
		// The board's decorative nodes pass pointer input to the module root,
		// so both ends of a swipe use the same logical viewport coordinates.
		m.dragging = event.HTMLID == "" && e.X >= 38 && e.X <= 502 && e.Y >= 222 && e.Y <= 686
		m.pressX, m.pressY = e.X, e.Y
	case scene.InputEventCancel:
		m.dragging = false
	case scene.InputEventRelease:
		if !m.dragging {
			return
		}
		m.dragging = false
		dx, dy := e.X-m.pressX, e.Y-m.pressY
		if max(math.Abs(float64(dx)), math.Abs(float64(dy))) < 24 {
			return
		}
		if math.Abs(float64(dx)) > math.Abs(float64(dy)) {
			if dx < 0 {
				m.move(left)
			} else {
				m.move(right)
			}
		} else if dy < 0 {
			m.move(up)
		} else {
			m.move(down)
		}
		m.render()
	}
}

func (m *Mod) render() {
	for i, value := range m.Board {
		exponent := 0
		if value != 0 {
			exponent = bits.TrailingZeros32(value)
		}
		view := &tileViews[exponent]
		class := view.class
		if i == m.Spawned && value != 0 {
			class = view.spawned
		}
		m.tiles[i].SetText(view.text)
		m.tileParents[i].SetAttribute("class", class)
	}
	m.score.SetText(strconv.FormatUint(m.Score, 10))
	m.best.SetText(strconv.FormatUint(m.Best, 10))
	m.moves.SetText(strconv.FormatUint(m.Moves, 10))
	if m.CanUndo {
		m.undoButton.RemoveAttribute("disabled")
	} else {
		m.undoButton.SetAttribute("disabled", "")
	}
	showWin := m.Won && !m.KeepPlaying
	m.overlay.SetHidden(!m.Lost && !showWin)
	m.keep.SetHidden(m.Lost || !showWin)
	if m.Lost {
		m.title.SetText("没有空位了")
		m.description.SetText("差一点！撤销上一步，或重新挑战。")
		m.status.SetText("本局结束 · 可以撤销上一步")
	} else if showWin {
		m.title.SetText("你拼出了 2048！")
		m.description.SetText("做得漂亮。继续向更大的数字进发吧。")
		m.status.SetText("目标达成 · 按 Enter 继续")
	} else {
		m.status.SetText("合并相同数字，向 2048 前进。")
	}
}
