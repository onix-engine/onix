package simulation

import (
	"testing"

	enginephysics "github.com/diiyw/onix/physics"
)

var defaultBirdStart = enginephysics.V(-92, 197)

func TestFlappyWorldCoordinatesMapIntoUIViewport(t *testing.T) {
	x, y := flappyUIPosition(0, 0)
	if x != flappyViewportWidth*.5 || y != flappyViewportHeight*.5 {
		t.Fatalf("world origin = (%v,%v)", x, y)
	}
	x, y = flappyUIPosition(float32(defaultBirdStart.X), float32(defaultBirdStart.Y))
	if x != 283 || y != 470 || flappyWorldPosition(x, y) != defaultBirdStart {
		t.Fatal("bird coordinate conversion failed")
	}
}

func TestFlappyScoresAndResetsAfterCollision(t *testing.T) {
	s := newFlappyState(defaultBirdStart)
	s.action()
	s.pipeX = defaultBirdStart.X + 1
	s.positionPipes()
	s.update(1.0 / 60)
	if s.score != 1 {
		t.Fatalf("score = %d", s.score)
	}
	s.collided = true
	s.update(1.0 / 60)
	if s.phase != flappyGameOver {
		t.Fatal("collision did not end game")
	}
	s.action()
	if s.phase != flappyReady || s.score != 0 || s.bird.Position() != defaultBirdStart {
		t.Fatal("restart did not reset game")
	}
}

func TestFlappyPipeCollisionEndsGame(t *testing.T) {
	s := newFlappyState(defaultBirdStart)
	s.action()
	s.pipeX, s.gapCenter = defaultBirdStart.X, -250
	s.positionPipes()
	for range 5 {
		s.update(1.0 / 60)
	}
	if s.phase != flappyGameOver {
		t.Fatal("pipe collision did not end game")
	}
}

func TestFlappyResetUsesAuthoredBirdPosition(t *testing.T) {
	want := enginephysics.V(35, 82)
	s := newFlappyState(want)
	s.bird.SetPosition(enginephysics.Vector{})
	s.reset()
	if s.bird.Position() != want {
		t.Fatal("authored start lost on reset")
	}
}
