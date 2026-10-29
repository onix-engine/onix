package simulation

import (
	"math/rand/v2"

	"github.com/diiyw/onix"
	enginephysics "github.com/diiyw/onix/physics"
)

const (
	flappyViewportWidth  = 750.0
	flappyViewportHeight = 1334.0
	flappyBirdRadius     = 24.0
	flappyPipeWidth      = 129.0
	flappyPipeHeight     = 801.0
	flappyPipeGap        = 300.0
	flappyGroundY        = -383.0
	flappyPipeStartX     = 440.0
	flappyPipeResetX     = 470.0
	flappyPipeFinishX    = -470.0
	flappySpeed          = 210.0
)

type flappyPhase uint8

const (
	flappyReady flappyPhase = iota
	flappyPlaying
	flappyGameOver
)

type flappyState struct {
	space      *enginephysics.World
	bird       enginephysics.Body
	birdStart  enginephysics.Vector
	pipeTop    enginephysics.Body
	pipeBottom enginephysics.Body
	pipeShapes [2]enginephysics.ColliderHandle[enginephysics.Box]
	collided   bool
	phase      flappyPhase
	pipeX      float64
	gapCenter  float64
	groundX    float64
	score      int
	passed     bool
}

type flappySnapshot struct {
	birdX, birdY, pipeX, gapCenter, groundX float32
	score                                   int
	phase                                   flappyPhase
}

func newFlappyState(birdStart enginephysics.Vector) *flappyState {
	s := &flappyState{space: enginephysics.NewWorld(), birdStart: birdStart}
	s.space.Gravity = enginephysics.V(0, -1100)
	birdDef := enginephysics.NewBodyDef(enginephysics.BodyDynamic)
	birdDef.Position = birdStart
	s.bird = s.space.CreateBody(birdDef)
	birdShape := enginephysics.NewColliderDef(enginephysics.Circle{Radius: flappyBirdRadius})
	birdShape.CollisionType = 1
	s.space.AddCollider(s.bird, birdShape)

	groundDef := enginephysics.NewBodyDef(enginephysics.BodyStatic)
	groundDef.Position = enginephysics.V(0, flappyGroundY-20)
	ground := s.space.CreateBody(groundDef)
	groundShape := enginephysics.NewColliderDef(enginephysics.NewBox(2000, 40))
	groundShape.CollisionType = 2
	s.space.AddCollider(ground, groundShape)

	s.pipeTop = s.space.CreateBody(enginephysics.NewBodyDef(enginephysics.BodyKinematic))
	s.pipeBottom = s.space.CreateBody(enginephysics.NewBodyDef(enginephysics.BodyKinematic))
	pipeShape := enginephysics.NewColliderDef(enginephysics.NewBox(flappyPipeWidth, flappyPipeHeight))
	pipeShape.CollisionType = 2
	s.pipeShapes[0] = s.space.AddCollider(s.pipeTop, pipeShape)
	s.pipeShapes[1] = s.space.AddCollider(s.pipeBottom, pipeShape)
	s.reset()
	return s
}

func (s *flappyState) action() {
	switch s.phase {
	case flappyReady:
		s.phase = flappyPlaying
		s.bird.SetVelocity(enginephysics.V(0, 470))
	case flappyPlaying:
		s.bird.SetVelocity(enginephysics.V(0, 470))
	case flappyGameOver:
		s.reset()
	}
}

func (s *flappyState) reset() {
	s.phase, s.collided, s.score, s.passed = flappyReady, false, 0, false
	s.pipeX, s.gapCenter, s.groundX = flappyPipeStartX, 90, 45
	s.bird.SetPosition(s.birdStart)
	s.bird.SetVelocity(enginephysics.Vector{})
	s.bird.ClearForces()
	s.positionPipes()
}

func (s *flappyState) update(dt float64) flappySnapshot {
	if dt <= 0 {
		dt = 1.0 / onix.DefaultTargetTPS
	}
	if s.phase == flappyPlaying {
		s.pipeX -= flappySpeed * dt
		s.groundX -= flappySpeed * dt
		if s.groundX <= -795 {
			s.groundX += 840
		}
		if s.pipeX < flappyPipeFinishX {
			s.pipeX, s.gapCenter, s.passed = flappyPipeResetX, rand.Float64()*360-40, false
		}
		if !s.passed && s.pipeX < s.bird.Position().X {
			s.score++
			s.passed = true
		}
		s.positionPipes()
		s.space.Step(dt)
		for _, event := range s.space.Events() {
			if event.Phase == enginephysics.CollisionBegin && (event.BodyA == s.bird.ID() || event.BodyB == s.bird.ID()) {
				s.collided = true
				break
			}
		}
		if s.collided || s.bird.Position().Y > 667 {
			s.phase = flappyGameOver
			s.bird.SetVelocity(enginephysics.Vector{})
		}
	}
	return s.snapshot()
}

func (s *flappyState) positionPipes() {
	offset := (flappyPipeHeight + flappyPipeGap) * 0.5
	s.pipeTop.SetPosition(enginephysics.V(s.pipeX, s.gapCenter+offset))
	s.pipeBottom.SetPosition(enginephysics.V(s.pipeX, s.gapCenter-offset))
}

func (s *flappyState) snapshot() flappySnapshot {
	return flappySnapshot{
		birdX: float32(s.bird.Position().X), birdY: float32(s.bird.Position().Y),
		pipeX: float32(s.pipeX), gapCenter: float32(s.gapCenter),
		groundX: float32(s.groundX), score: s.score, phase: s.phase,
	}
}

func flappyUIPosition(x, y float32) (float32, float32) {
	return x + flappyViewportWidth*0.5, flappyViewportHeight*0.5 - y
}

func flappyWorldPosition(x, y float32) enginephysics.Vector {
	return enginephysics.V(float64(x-flappyViewportWidth*0.5), float64(flappyViewportHeight*0.5-y))
}
