package physics

import (
	"math"
	"testing"
)

func TestDynamicCircleSettlesOnStaticBox(t *testing.T) {
	world := NewWorld()
	world.Gravity = V(0, -20)
	world.Iterations = 10
	floor := world.CreateBody(NewBodyDef(BodyStatic))
	AddCollider(world, floor, NewColliderDef(NewBox(20, 2)))
	def := NewBodyDef(BodyDynamic)
	def.Position = V(0, 8)
	ball := world.CreateBody(def)
	AddCollider(world, ball, NewColliderDef(Circle{Radius: 1}))

	for range 360 {
		world.Step(1.0 / 60)
	}
	if position := ball.Position(); math.Abs(position.Y-2) > .08 || math.Abs(position.X) > .01 {
		t.Fatalf("settled position = %+v, want approximately (0, 2)", position)
	}
	if velocity := ball.Velocity(); math.Abs(velocity.Y) > .2 {
		t.Fatalf("settled velocity = %+v", velocity)
	}
}

func TestFilterAndSensorCollisionEvents(t *testing.T) {
	world := NewWorld()
	world.Gravity = Vector{}
	a := world.CreateBody(NewBodyDef(BodyDynamic))
	b := world.CreateBody(NewBodyDef(BodyStatic))
	first := NewColliderDef(Circle{Radius: 2})
	first.Sensor = true
	first.Filter = NewFilter(0, 1, 1)
	second := NewColliderDef(Circle{Radius: 2})
	second.Filter = NewFilter(0, 2, 2)
	firstCollider := AddCollider(world, a, first)
	secondCollider := AddCollider(world, b, second)
	world.Step(1.0 / 60)
	if len(world.Events()) != 0 {
		t.Fatal("rejected filters emitted a collision")
	}
	firstCollider.SetFilter(FilterAll)
	secondCollider.SetFilter(FilterAll)
	world.Step(1.0 / 60)
	if events := world.Events(); len(events) != 1 || events[0].Phase != CollisionBegin {
		t.Fatalf("begin events = %+v", events)
	}
	if a.Position() != (Vector{}) {
		t.Fatalf("sensor moved body to %+v", a.Position())
	}
	b.SetPosition(V(20, 0))
	world.Step(1.0 / 60)
	if events := world.Events(); len(events) != 1 || events[0].Phase != CollisionEnd {
		t.Fatalf("end events = %+v", events)
	}
}

func TestStepReusesScratchStorage(t *testing.T) {
	world := NewWorld()
	world.Reserve(64, 64)
	world.Gravity = V(0, -10)
	floor := world.CreateBody(NewBodyDef(BodyStatic))
	AddCollider(world, floor, NewColliderDef(NewBox(200, 2)))
	for index := range 32 {
		def := NewBodyDef(BodyDynamic)
		def.Position = V(float64(index*3-48), 5+float64(index%4)*3)
		body := world.CreateBody(def)
		AddCollider(world, body, NewColliderDef(Circle{Radius: 1}))
	}
	for range 120 {
		world.Step(1.0 / 60)
	}
	allocations := testing.AllocsPerRun(100, func() { world.Step(1.0 / 60) })
	if allocations != 0 {
		t.Fatalf("steady-state Step allocations = %v, want 0", allocations)
	}
}
