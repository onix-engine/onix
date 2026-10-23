package game

import (
	"math/rand/v2"
	"testing"

	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/scene"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestSlideRulesInEveryDirection(t *testing.T) {
	cases := []struct {
		name  string
		in    [4]uint32
		want  [4]uint32
		score uint64
	}{
		{"four equal tiles merge in pairs", [4]uint32{2, 2, 2, 2}, [4]uint32{4, 4}, 8},
		{"new tile cannot merge twice", [4]uint32{2, 2, 4}, [4]uint32{4, 4}, 4},
		{"merge across holes", [4]uint32{4, 0, 4, 4}, [4]uint32{8, 4}, 8},
		{"two different pairs", [4]uint32{4, 4, 8, 8}, [4]uint32{8, 16}, 24},
		{"only compression", [4]uint32{0, 2, 0, 4}, [4]uint32{2, 4}, 0},
		{"already aligned", [4]uint32{2, 4, 8, 16}, [4]uint32{2, 4, 8, 16}, 0},
		{"empty", [4]uint32{}, [4]uint32{}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, d := range []direction{left, right, up, down} {
				p := position{}
				for line := range 4 {
					for offset, value := range tc.in {
						p.Board[cell(d, line, offset)] = value
					}
				}
				changed := p.slide(d)
				for line := range 4 {
					for offset, want := range tc.want {
						if got := p.Board[cell(d, line, offset)]; got != want {
							t.Fatalf("direction %d, line %d, offset %d: got %d, want %d", d, line, offset, got, want)
						}
					}
				}
				if p.Score != 4*tc.score || changed != (tc.in != tc.want) {
					t.Fatalf("direction %d: score=%d changed=%v", d, p.Score, changed)
				}
			}
		})
	}
}

func seededGame() game {
	return game{random: *rand.NewPCG(123, 456), Spawned: -1}
}

func TestMoveSpawnUndoAndNoOp(t *testing.T) {
	g := seededGame()
	g.Board = [16]uint32{2, 2}
	before := g.position
	if !g.move(left) || g.Score != 4 || g.Moves != 1 || !g.CanUndo {
		t.Fatalf("bad move: %+v", g)
	}
	if count, sum := boardStats(g.Board); count != 2 || sum != 4+uint64(g.Board[g.Spawned]) {
		t.Fatal("valid move must spawn exactly one 2 or 4", g.Board)
	}
	after, spawned := g.position, g.Spawned
	if !g.undo() || g.position != before || g.CanUndo || g.Best != 4 || g.undo() {
		t.Fatal("undo did not restore the previous position or preserve best score")
	}
	if !g.move(left) || g.position != after || g.Spawned != spawned {
		t.Fatal("undo must restore the random state too")
	}
	g.Board = [16]uint32{2, 4}
	before, rng, previous := g.position, g.random, g.previous
	if g.move(left) || g.position != before || g.random != rng || g.previous != previous {
		t.Fatal("no-op changed the board, score, random state or undo history")
	}
}

func TestReset(t *testing.T) {
	g := seededGame()
	g.position = position{Score: 100, Moves: 10, Won: true, Lost: true, KeepPlaying: true}
	g.CanUndo, g.Best = true, 200
	g.reset()
	count, _ := boardStats(g.Board)
	if count != 2 || g.Score != 0 || g.Moves != 0 || g.Won || g.Lost || g.KeepPlaying || g.CanUndo || g.Best != 200 {
		t.Fatalf("reset did not create a fresh game: %+v", g)
	}
	for _, tile := range g.Board {
		if tile != 0 && tile != 2 && tile != 4 {
			t.Fatal("invalid starting tile", tile)
		}
	}
}

func TestWinContinueAndLoss(t *testing.T) {
	g := seededGame()
	g.Board = [16]uint32{1024, 1024}
	if !g.move(left) || !g.Won || g.Score != 2048 || g.move(right) {
		t.Fatal("2048 must pause play after counting the merge")
	}
	g.KeepPlaying = true
	if !g.move(right) {
		t.Fatal("cannot continue after winning")
	}
	blocked := [16]uint32{2, 4, 2, 4, 4, 2, 4, 2, 2, 4, 2, 4, 4, 2, 4, 2}
	g = seededGame()
	g.Board = blocked
	if g.canMove() {
		t.Fatal("checkerboard has no legal moves")
	}
	// A final compression fills the only empty cell. Either spawn value is
	// surrounded by larger, different neighbors, leaving no legal move.
	g.Board = [16]uint32{0, 4, 8, 16, 8, 16, 32, 64, 2, 4, 2, 4, 4, 2, 4, 2}
	if !g.move(left) || !g.Lost || g.move(up) || !g.undo() || g.Lost {
		t.Fatal("loss detection or undo from game over failed", g.Board)
	}
	p := position{Board: blocked}
	p.Board[15] = 0
	if !p.canMove() {
		t.Fatal("empty cell must permit another move")
	}
	p.Board[15], p.Board[14] = 4, 4
	if !p.canMove() {
		t.Fatal("horizontal pair must permit another move")
	}
	p.Board = blocked
	p.Board[12] = 2
	if !p.canMove() {
		t.Fatal("vertical pair must permit another move")
	}
}

func boardStats(board [16]uint32) (count int, sum uint64) {
	for _, value := range board {
		if value != 0 {
			count++
		}
		sum += uint64(value)
	}
	return
}

func TestRandomGamesConserveTileSum(t *testing.T) {
	random := rand.New(rand.NewPCG(1, 2))
	for range 30 {
		g := seededGame()
		g.reset()
		for range 2000 {
			_, before := boardStats(g.Board)
			if g.move(direction(random.IntN(4))) {
				_, after := boardStats(g.Board)
				if value := g.Board[g.Spawned]; (value != 2 && value != 4) || after != before+uint64(value) {
					t.Fatal("move lost tile value or spawned an invalid tile")
				}
			}
			if g.Lost {
				break
			}
			g.KeepPlaying = g.Won
		}
	}
}

func TestMoveDoesNotAllocate(t *testing.T) {
	g := seededGame()
	allocs := testing.AllocsPerRun(1000, func() {
		g.position = position{Board: [16]uint32{2, 2, 4, 4}}
		g.move(left)
		g.undo()
	})
	if allocs != 0 {
		t.Fatalf("move and undo allocated %g times", allocs)
	}
}

func TestInputMovesOncePerAction(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input []module.Input
	}{
		{"keyboard", []module.Input{{Event: scene.InputEvent{Kind: scene.InputEventKeyDown, Key: input.KeyLeft}}}},
		{"WASD", []module.Input{{Event: scene.InputEvent{Kind: scene.InputEventKeyDown, Key: input.KeyA}}}},
		{"button", []module.Input{
			{HTMLID: "left", Event: scene.InputEvent{Kind: scene.InputEventTap}},
			{HTMLID: "left", HTMLKind: htmlui.EventClick},
		}},
		{"swipe", []module.Input{
			{Event: scene.InputEvent{Kind: scene.InputEventPress, X: 400, Y: 300}},
			{Event: scene.InputEvent{Kind: scene.InputEventRelease, X: 100, Y: 300}},
			{Event: scene.InputEvent{Kind: scene.InputEventTap, X: 100, Y: 300}},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := Mod{game: seededGame()}
			m.Board = [16]uint32{2, 2}
			for _, event := range tc.input {
				m.handleInput(event)
			}
			if m.Moves != 1 || m.Score != 4 {
				t.Fatal("action should perform exactly one move", m.position)
			}
		})
	}
	m := Mod{game: seededGame()}
	m.Board = [16]uint32{2, 2}
	for _, events := range [][]module.Input{
		{{Event: scene.InputEvent{Kind: scene.InputEventPress, X: 400, Y: 300}}, {Event: scene.InputEvent{Kind: scene.InputEventRelease, X: 390, Y: 300}}},
		{{Event: scene.InputEvent{Kind: scene.InputEventPress, X: 400, Y: 180}}, {Event: scene.InputEvent{Kind: scene.InputEventRelease, X: 100, Y: 180}}},
		{{Event: scene.InputEvent{Kind: scene.InputEventPress, X: 400, Y: 300}}, {Event: scene.InputEvent{Kind: scene.InputEventCancel}}, {Event: scene.InputEvent{Kind: scene.InputEventRelease, X: 100, Y: 300}}},
	} {
		for _, event := range events {
			m.handleInput(event)
		}
	}
	if m.Moves != 0 {
		t.Fatal("short, canceled or off-board gestures must not move")
	}
}
