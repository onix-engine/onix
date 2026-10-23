package game

import "math/rand/v2"

type direction uint8

const (
	left direction = iota
	right
	up
	down
)

type position struct {
	Board       [16]uint32
	Score       uint64
	Moves       uint64
	Won         bool
	Lost        bool
	KeepPlaying bool
}

type game struct {
	position
	Best        uint64
	CanUndo     bool
	Spawned     int
	previous    position
	random      rand.PCG
	previousRNG rand.PCG
}

func (g *game) reset() {
	g.position = position{}
	g.CanUndo = false
	// Keep the initial layout stable so the retained HTML has its two initial
	// glyphs before the first incremental synchronization pass.
	g.Board[6], g.Board[11] = 2, 2
	g.Spawned = -1
}

func (g *game) spawn() {
	var empty [16]int
	n := 0
	for i, value := range g.Board {
		if value == 0 {
			empty[n] = i
			n++
		}
	}
	g.Spawned = -1
	if n == 0 {
		return
	}
	random := rand.New(&g.random)
	g.Spawned = empty[random.IntN(n)]
	value := uint32(2)
	if random.IntN(10) == 0 {
		value = 4
	}
	g.Board[g.Spawned] = value
}

// slide visits each row/column in movement order. A written tile can merge
// only once per turn; holes are removed before comparing adjacent values.
func (p *position) slide(d direction) bool {
	before := p.Board
	for line := range 4 {
		var result [4]uint32
		count := 0
		merged := false
		for offset := range 4 {
			value := p.Board[cell(d, line, offset)]
			if value == 0 {
				continue
			}
			if count > 0 && !merged && result[count-1] == value {
				result[count-1] *= 2
				p.Score += uint64(result[count-1])
				p.Won = p.Won || result[count-1] >= 2048
				merged = true
			} else {
				result[count] = value
				count++
				merged = false
			}
		}
		for offset, value := range result {
			p.Board[cell(d, line, offset)] = value
		}
	}
	return p.Board != before
}

func cell(d direction, line, offset int) int {
	switch d {
	case right:
		return line*4 + 3 - offset
	case up:
		return offset*4 + line
	case down:
		return (3-offset)*4 + line
	default:
		return line*4 + offset
	}
}

func (g *game) move(d direction) bool {
	if g.Lost || g.Won && !g.KeepPlaying {
		return false
	}
	before, random := g.position, g.random
	if !g.slide(d) {
		return false
	}
	g.previous, g.previousRNG, g.CanUndo = before, random, true
	g.Moves++
	g.spawn()
	g.Lost = !g.canMove()
	g.Best = max(g.Best, g.Score)
	return true
}

func (p *position) canMove() bool {
	for i, value := range p.Board {
		if value == 0 || i%4 < 3 && value == p.Board[i+1] || i < 12 && value == p.Board[i+4] {
			return true
		}
	}
	return false
}

func (g *game) undo() bool {
	if !g.CanUndo {
		return false
	}
	g.position, g.random = g.previous, g.previousRNG
	g.CanUndo, g.Spawned = false, -1
	return true
}
