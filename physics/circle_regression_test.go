package physics

import (
	"math"
	"testing"
)

func TestCircleRollsDownSlope(t *testing.T) {
	t.Run("box", func(t *testing.T) {
		assertCircleRollsDownSlope(t, NewBox(1000, 18), 9)
	})
	t.Run("segment", func(t *testing.T) {
		assertCircleRollsDownSlope(t, Segment{Start: V(-500, 0), End: V(500, 0), Radius: 3}, 3)
	})
}

func assertCircleRollsDownSlope[S Shape2D](t *testing.T, surface S, height float64) {
	t.Helper()
	world := NewWorld()
	world.Gravity, world.Iterations = V(0, -520), 10
	const angle, radius = -.18, 16
	tangent, normal := rotate(V(1, 0), angle), rotate(V(0, 1), angle)
	floorDef := NewBodyDef(BodyStatic)
	floorDef.Rotation = angle
	world.AddCollider(world.CreateBody(floorDef), NewColliderDef(surface))
	def := NewBodyDef(BodyDynamic)
	def.Position = normal.Mul(height + radius)
	body := world.CreateBody(def)
	world.AddCollider(body, NewColliderDef(Circle{Radius: radius}))
	for range 120 {
		world.Step(1.0 / 120)
	}
	distance, speed := body.Position().Sub(def.Position).Dot(tangent), body.Velocity().Dot(tangent)
	if distance < 10 || speed < 20 || body.AngularVelocity() > -.5 {
		t.Fatalf("circle failed to roll: distance=%v speed=%v angular=%v", distance, speed, body.AngularVelocity())
	}
	// The contact lever arm includes the solver's permitted penetration slop.
	contactRadius := body.Position().Dot(normal) - height
	if slip := speed + contactRadius*body.AngularVelocity(); math.Abs(slip) > .02 {
		t.Fatalf("circle slides instead of rolling: contact slip=%v", slip)
	}
	if clearance := contactRadius - radius; math.Abs(clearance) > .02 {
		t.Fatalf("circle left or penetrated slope: clearance=%v", clearance)
	}
	if allocations := testing.AllocsPerRun(100, func() { world.Step(1.0 / 120) }); allocations != 0 {
		t.Fatalf("rolling circle step allocations = %v, want 0", allocations)
	}
}

func TestCirclesExchangeMomentum(t *testing.T) {
	world := NewWorld()
	world.Gravity = Vector{}
	firstDef := NewBodyDef(BodyDynamic)
	firstDef.Position, firstDef.Velocity = V(-1, 0), V(6, 0)
	first := world.CreateBody(firstDef)
	secondDef := NewBodyDef(BodyDynamic)
	secondDef.Position, secondDef.Velocity, secondDef.Mass = V(1.4, 0), V(-2, 0), 3
	second := world.CreateBody(secondDef)
	collider := NewColliderDef(Circle{Radius: 1})
	collider.Restitution, collider.Friction = 1, 0
	world.AddCollider(first, collider)
	collider.Shape.Radius = 1.5
	world.AddCollider(second, collider)
	world.Step(1.0 / 60)
	if first.Velocity().Sub(V(-6, 0)).Length() > 1e-9 || second.Velocity().Sub(V(2, 0)).Length() > 1e-9 {
		t.Fatalf("elastic impact lost momentum or energy: first=%v second=%v", first.Velocity(), second.Velocity())
	}
	if second.Position().Sub(first.Position()).Length() < 2.5-world.CollisionSlop-1e-6 {
		t.Fatalf("circles remain overlapped: first=%v second=%v", first.Position(), second.Position())
	}
	if math.Abs(first.AngularVelocity())+math.Abs(second.AngularVelocity()) > 1e-9 {
		t.Fatal("central circle impact introduced spin")
	}
}

func TestBouncyCircleComesToRest(t *testing.T) {
	world := NewWorld()
	world.Gravity = V(0, -520)
	world.AddCollider(world.CreateBody(NewBodyDef(BodyStatic)), NewColliderDef(NewBox(560, 28)))
	def := NewBodyDef(BodyDynamic)
	def.Position = V(0, 100)
	body := world.CreateBody(def)
	collider := NewColliderDef(Circle{Radius: 12})
	collider.Restitution = .75
	world.AddCollider(body, collider)
	bounced := false
	for range 1200 {
		world.Step(1.0 / 60)
		bounced = bounced || body.Velocity().Y > 20
	}
	if !bounced {
		t.Fatal("elastic circle never bounced")
	}
	for range 300 {
		world.Step(1.0 / 60)
		if body.Velocity().Length() > .01 || math.Abs(body.Position().Y-26) > .02 {
			t.Fatalf("circle failed to settle: position=%v velocity=%v", body.Position(), body.Velocity())
		}
	}
}
