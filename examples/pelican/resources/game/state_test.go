package game

import (
	"testing"

	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/scene"
	htmlui "github.com/diiyw/onix/ui/html"
)

func isolated() game { return game{Phase: riding, Hearts: 3, Speed: 245} }
func tick(g *game, n int) {
	for range n {
		g.step(1.0 / 60)
	}
}

func TestJumpCargoGlideAndFlap(t *testing.T) {
	light, heavy := isolated(), isolated()
	heavy.Cargo = capacity
	light.jump()
	light.release()
	heavy.jump()
	heavy.release()
	tick(&light, 20)
	tick(&heavy, 20)
	if light.Height <= heavy.Height || heavy.Height < 65 {
		t.Fatalf("cargo must lower a still-useful jump: light=%f heavy=%f", light.Height, heavy.Height)
	}
	normal, glide := isolated(), isolated()
	normal.Height = 150
	glide.Height = 150
	normal.Velocity = -200
	glide.Velocity = -200
	glide.Held = true
	tick(&normal, 15)
	tick(&glide, 15)
	if !glide.Gliding || glide.Height <= normal.Height+30 {
		t.Fatal("holding jump must slow descent")
	}
	glide.release()
	glide.Cargo = 2
	glide.jump()
	if glide.Cargo != 1 || glide.Velocity != 430 || !glide.Flapped {
		t.Fatal("an air flap must spend exactly one fish")
	}
	glide.release()
	glide.jump()
	if glide.Cargo != 1 {
		t.Fatal("only one paid flap is allowed per flight")
	}
	empty := isolated()
	empty.Height = 100
	empty.jump()
	if empty.Flapped || empty.Velocity != 0 {
		t.Fatal("an empty pouch cannot buy an air flap")
	}
}

func TestPickupsCapacityAndObstacleClearance(t *testing.T) {
	g := isolated()
	g.Cargo = 5
	g.Count = 3
	g.Objects[0] = object{X: riderX + 65, Height: 135, Kind: fish}
	g.Objects[1] = object{X: riderX + 70, Height: 135, Kind: fish}
	g.Objects[2] = object{X: riderX, Kind: crab}
	g.Height = 50
	g.step(1.0 / 60)
	if g.Cargo != 6 || !g.Objects[0].Used || g.Objects[1].Used || g.Hearts != 3 {
		t.Fatalf("pickup capacity or crab clearance failed: %+v", g)
	}
	g.Height = 0
	g.Velocity = 0
	g.step(1.0 / 60)
	if g.Hearts != 2 || g.Cargo != 4 || g.Invincible <= 0 {
		t.Fatal("collision must cost a heart and two fish")
	}
	g.Count = 4
	g.Objects[3] = object{X: riderX + g.Distance, Kind: crate}
	g.step(1.0 / 60)
	if g.Hearts != 2 {
		t.Fatal("overlapping contacts must respect invincibility")
	}
	g.Invincible = 0
	g.Height = 75
	g.step(1.0 / 60)
	if g.Hearts != 2 {
		t.Fatal("a high enough jump must clear a crate")
	}
}

func TestDeliveryRangeBonusesAndNoDuplicate(t *testing.T) {
	g := isolated()
	g.Cargo = 6
	g.Count = 1
	g.Objects[0] = object{X: riderX + 400, Kind: station}
	if g.deliver() || g.Cargo != 6 {
		t.Fatal("delivery outside the station range changed cargo")
	}
	g.Distance = 220
	if !g.deliver() || g.Delivered != 6 || g.Cargo != 0 || g.Score != 850 || g.Boost != 3.5 || g.Stops != 1 {
		t.Fatalf("full pouch delivery failed: delivered=%d score=%d", g.Delivered, g.Score)
	}
	g.Cargo = 3
	if g.deliver() || g.Delivered != 6 || g.Cargo != 3 {
		t.Fatal("a station accepted a duplicate delivery")
	}
	g.Objects[0].Used = false
	g.Cargo = 0
	if g.deliver() || g.Objects[0].Used {
		t.Fatal("empty delivery consumed a station")
	}
	g.Cargo = 2
	g.deliver()
	if g.Score != 1150 || g.Combo != 2 {
		t.Fatal("consecutive delivery bonus did not increase")
	}
}

func TestPauseFinishCrashAndRestart(t *testing.T) {
	g := isolated()
	g.Height = 80
	g.Velocity = 200
	g.Held = true
	g.togglePause()
	distance, height := g.Distance, g.Height
	tick(&g, 60)
	if g.Distance != distance || g.Height != height || g.Held {
		t.Fatal("paused simulation moved or retained held input")
	}
	g.togglePause()
	g.Distance = routeLength - 1
	g.Delivered = target
	g.step(1.0 / 60)
	if g.Phase != finished || g.Score != 1450 || g.Best != 1450 || g.Distance != routeLength {
		t.Fatal("successful finish scoring failed")
	}
	tick(&g, 60)
	if g.Score != 1450 {
		t.Fatal("end bonus was awarded twice")
	}
	g.start()
	if g.Best != 1450 || g.Cargo != 0 || g.Hearts != 3 || g.Phase != riding {
		t.Fatal("restart did not preserve only the session best")
	}
	g.Count = 1
	g.First = 0
	g.Hearts = 1
	g.Objects[0] = object{X: riderX, Kind: crate}
	g.step(1.0 / 60)
	if g.Phase != crashed {
		t.Fatal("last heart must end the run")
	}
}

func TestCourseCanBeCompleted(t *testing.T) {
	var g game
	g.start()
	for frame := 0; frame < 6000 && g.Phase == riding; frame++ {
		if g.nearStation() >= 0 && g.Cargo > 0 {
			g.deliver()
		}
		if g.Height == 0 {
			for i := g.First; i < g.Count; i++ {
				o := &g.Objects[i]
				dx := o.X - g.Distance - riderX
				lead := g.Speed * (660 - float32(g.Cargo)*12) / (1550 + float32(g.Cargo)*30)
				if dx > lead {
					break
				}
				if !o.Used && (o.Kind == crab || o.Kind == crate) && dx > 25 {
					g.jump()
					g.release()
					break
				}
			}
		}
		g.step(1.0 / 60)
	}
	if g.Phase != finished || g.Delivered < target || g.Hearts < 1 {
		t.Fatalf("route cannot be completed with timed jumps: phase=%d delivered=%d hearts=%d distance=%.0f", g.Phase, g.Delivered, g.Hearts, g.Distance)
	}
}

func TestInputDebounceAndPointerHold(t *testing.T) {
	var m Mod
	m.start()
	key := func(k input.KeyCode, kind scene.InputEventKind) {
		m.handleInput(module.Input{Event: scene.InputEvent{Kind: kind, Key: k}})
	}
	key(input.KeySpace, scene.InputEventKeyDown)
	m.step(1.0 / 60)
	m.Cargo = 3
	key(input.KeySpace, scene.InputEventKeyDown)
	if m.Flapped || m.Cargo != 3 {
		t.Fatal("OS key repeat must not spend a fish")
	}
	key(input.KeySpace, scene.InputEventKeyUp)
	key(input.KeySpace, scene.InputEventKeyDown)
	if !m.Flapped || m.Cargo != 2 {
		t.Fatal("release and repress should perform an air flap")
	}
	key(input.KeySpace, scene.InputEventKeyUp)
	m.handleInput(module.Input{HTMLID: "jump", Event: scene.InputEvent{Kind: scene.InputEventPress}})
	if !m.Held {
		t.Fatal("touch press did not begin holding")
	}
	m.handleInput(module.Input{HTMLID: "jump", Event: scene.InputEvent{Kind: scene.InputEventRelease}})
	m.handleInput(module.Input{HTMLID: "jump", HTMLKind: htmlui.EventClick})
	if m.Held || m.Cargo != 2 {
		t.Fatal("touch click repeated the press or stuck the hold")
	}
	key(input.KeyR, scene.InputEventKeyDown)
	m.step(1.0 / 60)
	d := m.Distance
	key(input.KeyR, scene.InputEventKeyDown)
	if m.Distance != d {
		t.Fatal("holding R repeatedly reset the route")
	}
}

func TestSimulationAllocations(t *testing.T) {
	var g game
	g.start()
	if allocs := testing.AllocsPerRun(100, func() { g.reset(); g.Phase = riding; g.jump(); g.release(); tick(&g, 60) }); allocs != 0 {
		t.Fatalf("simulation allocated %.1f times", allocs)
	}
}
