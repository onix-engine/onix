package physics

import (
	"fmt"
	"math"
	"testing"
)

func TestReservePreservesLiveWorld(t *testing.T) {
	world := NewWorld()
	world.Gravity = Vector{}
	def := NewBodyDef(BodyDynamic)
	def.Position, def.Velocity, def.UserData = V(1, 2), V(3, 4), 17
	body := world.CreateBody(def)
	colliderDef := NewColliderDef(Circle{Radius: 2})
	colliderDef.Sensor = true
	collider := AddCollider(world, body, colliderDef)
	other := world.CreateBody(NewBodyDef(BodyStatic))
	AddCollider(world, other, NewColliderDef(Circle{Radius: 2}))
	world.Step(1.0 / 60)
	before, velocity := body.Position(), body.Velocity()
	world.Reserve(128, 128)
	if !body.Valid() || !collider.Valid() || body.Position() != before || body.Velocity() != velocity || body.UserData() != 17 || collider.Shape().Radius != 2 {
		t.Fatal("Reserve changed existing body or collider state")
	}
	if events := world.Events(); len(events) != 1 || events[0].Phase != CollisionBegin {
		t.Fatalf("Reserve discarded events: %+v", events)
	}
	world.Step(1.0 / 60)
	if events := world.Events(); len(events) != 1 || events[0].Phase != CollisionStay {
		t.Fatalf("Reserve discarded active contact: %+v", events)
	}
	world.RemoveBody(other)
	world.Reserve(256, 256)
	reused := world.CreateBody(NewBodyDef(BodyStatic))
	if !body.Valid() || !reused.Valid() || other.Valid() || len(world.bodies) != 2 {
		t.Fatal("Reserve corrupted free body pool")
	}
}

func TestReservePreservesLiveWorld3D(t *testing.T) {
	world := NewWorld3D()
	def := NewBodyDef3D(BodyDynamic)
	def.Position = V3(1, 2, 3)
	body := world.CreateBody(def)
	collider := AddCollider3D(world, body, NewColliderDef(Sphere{Radius: 2}))
	world.Reserve(128, 128)
	if !body.Valid() || !collider.Valid() || body.Position() != def.Position || collider.Shape().Radius != 2 {
		t.Fatal("shared generic capacity growth corrupted 3D state")
	}
}

func TestColliderReuseEndsOldContact(t *testing.T) {
	for _, removeBody := range []bool{false, true} {
		world := NewWorld()
		world.Gravity = Vector{}
		first := world.CreateBody(NewBodyDef(BodyDynamic))
		second := world.CreateBody(NewBodyDef(BodyStatic))
		def := NewColliderDef(Circle{Radius: 1})
		def.Sensor = true
		old := AddCollider(world, first, def)
		AddCollider(world, second, def)
		world.Step(1.0 / 60)
		if removeBody {
			world.RemoveBody(first)
			first = world.CreateBody(NewBodyDef(BodyDynamic))
		} else {
			RemoveCollider(world, old)
		}
		next := AddCollider(world, first, def)
		world.Step(1.0 / 60)
		events := world.Events()
		if len(events) != 2 || events[0].Phase != CollisionEnd || events[0].ColliderA != old.ID() || events[1].Phase != CollisionBegin || events[1].ColliderA != next.ID() {
			t.Fatalf("removeBody=%v: reused collider events = %+v", removeBody, events)
		}
		if old.Valid() || old.ID() == next.ID() {
			t.Fatal("removed collider handle became valid again")
		}
	}
}

func TestFixedRotationIgnoresAngularMotion(t *testing.T) {
	for _, bodyType := range []BodyType{BodyDynamic, BodyKinematic} {
		world := NewWorld()
		world.Gravity = Vector{}
		def := NewBodyDef(bodyType)
		def.FixedRotation, def.Rotation, def.AngularVelocity = true, .4, 12
		body := world.CreateBody(def)
		AddCollider(world, body, NewColliderDef(NewBox(2, 2)))
		body.SetAngularVelocity(20)
		body.ApplyTorque(30)
		body.ApplyImpulse(V(0, 1), V(1, 0))
		world.Step(.1)
		if body.Rotation() != .4 || body.AngularVelocity() != 0 {
			t.Fatalf("type=%v: fixed rotation moved: rotation=%v angularVelocity=%v", bodyType, body.Rotation(), body.AngularVelocity())
		}
	}
}

func TestStaticVelocityDoesNotPushDynamicBody(t *testing.T) {
	world := NewWorld()
	world.Gravity = Vector{}
	def := NewBodyDef(BodyStatic)
	def.Velocity = V(10, 0)
	def.AngularVelocity = 10
	AddCollider(world, world.CreateBody(def), NewColliderDef(Circle{Radius: 1}))
	def = NewBodyDef(BodyDynamic)
	def.Position = V(1.9, 0)
	body := world.CreateBody(def)
	AddCollider(world, body, NewColliderDef(Circle{Radius: 1}))
	world.Step(1.0 / 60)
	if body.Velocity() != (Vector{}) || body.AngularVelocity() != 0 {
		t.Fatalf("static body transferred velocity: %v, angular=%v", body.Velocity(), body.AngularVelocity())
	}
}

func TestDynamicBoxSettlesWithoutArtificialTorque(t *testing.T) {
	world := NewWorld()
	floor := world.CreateBody(NewBodyDef(BodyStatic))
	AddCollider(world, floor, NewColliderDef(NewBox(20, 2)))
	def := NewBodyDef(BodyDynamic)
	def.Position = V(3, 4)
	body := world.CreateBody(def)
	AddCollider(world, body, NewColliderDef(NewBox(2, 2)))
	for range 360 {
		world.Step(1.0 / 60)
	}
	if math.Abs(body.Rotation()) > 1e-6 || math.Abs(body.Position().X-3) > 1e-6 || math.Abs(body.Position().Y-2) > .02 || body.Velocity().Length() > .01 {
		t.Fatalf("box failed to settle: position=%v rotation=%v velocity=%v", body.Position(), body.Rotation(), body.Velocity())
	}
}

func TestTiltedBoxComesToRest(t *testing.T) {
	for _, test := range []struct {
		restitution float64
		slope       float64
	}{{0, 0}, {.35, 0}, {.55, 0}, {.1, -.18}} {
		t.Run(fmt.Sprintf("restitution=%g/slope=%g", test.restitution, test.slope), func(t *testing.T) {
			world := NewWorld()
			world.Gravity, world.Iterations = V(0, -520), 10
			floorDef := NewBodyDef(BodyStatic)
			floorDef.Rotation = test.slope
			floor := world.CreateBody(floorDef)
			AddCollider(world, floor, NewColliderDef(NewBox(560, 28)))
			def := NewBodyDef(BodyDynamic)
			def.Position, def.Rotation = V(0, 100), .3
			body := world.CreateBody(def)
			collider := NewColliderDef(NewBox(28, 28))
			collider.Restitution = test.restitution
			AddCollider(world, body, collider)
			for range 1200 {
				world.Step(1.0 / 60)
			}
			position, rotation := body.Position(), body.Rotation()
			for range 300 {
				world.Step(1.0 / 60)
				if body.Velocity().Length() > .01 || math.Abs(body.AngularVelocity()) > .001 ||
					body.Position().Sub(position).Length() > .01 || math.Abs(body.Rotation()-rotation) > .001 {
					t.Fatalf("box keeps moving: position=%v rotation=%v velocity=%v angular=%v", body.Position(), body.Rotation(), body.Velocity(), body.AngularVelocity())
				}
			}
			if allocations := testing.AllocsPerRun(100, func() { world.Step(1.0 / 60) }); allocations != 0 {
				t.Fatalf("resting box step allocations = %v, want 0", allocations)
			}
			body.ApplyImpulse(V(28, 56), body.Position())
			world.Step(1.0 / 60)
			if body.Position().Y <= position.Y || body.Velocity().Y <= 0 {
				t.Fatal("settled body did not respond to an impulse")
			}
		})
	}
}

func TestBoxStackComesToRest(t *testing.T) {
	world := NewWorld()
	world.Gravity, world.Iterations = V(0, -20), 12
	floor := world.CreateBody(NewBodyDef(BodyStatic))
	AddCollider(world, floor, NewColliderDef(NewBox(20, 2)))
	var bodies [4]Body
	for index := range bodies {
		def := NewBodyDef(BodyDynamic)
		def.Position = V(0, 2+2.1*float64(index))
		bodies[index] = world.CreateBody(def)
		AddCollider(world, bodies[index], NewColliderDef(NewBox(2, 2)))
	}
	for range 1200 {
		world.Step(1.0 / 60)
	}
	for range 300 {
		world.Step(1.0 / 60)
		for index, body := range bodies {
			if body.Position().Sub(V(0, 2+2*float64(index))).Length() > .1 ||
				body.Velocity().Length() > .01 || math.Abs(body.Rotation()) > .001 || math.Abs(body.AngularVelocity()) > .001 {
				t.Fatalf("box %v failed to settle in stack: position=%v rotation=%v velocity=%v angular=%v", index, body.Position(), body.Rotation(), body.Velocity(), body.AngularVelocity())
			}
		}
	}
}

func TestBoxSlidesWhenSlopeExceedsFriction(t *testing.T) {
	const angle = -.18
	tangent, normal := rotate(V(1, 0), angle), rotate(V(0, 1), angle)
	for _, friction := range []float64{.6, .02} {
		t.Run(fmt.Sprint(friction), func(t *testing.T) {
			world := NewWorld()
			world.Gravity, world.Iterations = V(0, -520), 10
			floorDef := NewBodyDef(BodyStatic)
			floorDef.Rotation = angle
			floor := world.CreateBody(floorDef)
			floorCollider := NewColliderDef(NewBox(1000, 18))
			floorCollider.Friction = friction
			AddCollider(world, floor, floorCollider)
			def := NewBodyDef(BodyDynamic)
			def.Position, def.Rotation = normal.Mul(30), angle
			def.Mass, def.AngularDamping = 2, .2
			body := world.CreateBody(def)
			collider := NewColliderDef(NewBox(42, 42))
			collider.Friction, collider.Restitution = .65, .1
			AddCollider(world, body, collider)
			for range 120 {
				world.Step(1.0 / 60)
			}
			distance, speed := body.Position().Sub(def.Position).Dot(tangent), body.Velocity().Dot(tangent)
			coefficient := math.Sqrt(friction * collider.Friction)
			t.Logf("friction=%v critical=%v downhill distance=%v speed=%v", coefficient, math.Tan(-angle), distance, speed)
			if coefficient < math.Tan(-angle) {
				if distance < 20 || speed < 20 {
					t.Fatalf("insufficient friction held box on slope: distance=%v speed=%v", distance, speed)
				}
			} else if math.Abs(distance) > .01 || body.Velocity().Length() > .01 || math.Abs(body.AngularVelocity()) > .001 {
				t.Fatalf("static friction failed to hold box: distance=%v velocity=%v angular=%v", distance, body.Velocity(), body.AngularVelocity())
			}
		})
	}
}

func TestFrictionlessBoxKeepsSlidingAcrossTimesteps(t *testing.T) {
	world := NewWorld()
	world.Gravity = V(0, -20)
	floor := world.CreateBody(NewBodyDef(BodyStatic))
	AddCollider(world, floor, NewColliderDef(NewBox(100, 2)))
	def := NewBodyDef(BodyDynamic)
	def.Position, def.Velocity = V(0, 2), V(3, 0)
	body := world.CreateBody(def)
	collider := NewColliderDef(NewBox(2, 2))
	collider.Friction = 0
	AddCollider(world, body, collider)
	for _, delta := range []float64{1.0 / 60, 1.0 / 30, 1.0 / 120} {
		for range 120 {
			world.Step(delta)
			if body.Velocity().Sub(V(3, 0)).Length() > 1e-9 || math.Abs(body.AngularVelocity()) > 1e-9 {
				t.Fatalf("delta=%v: frictionless box velocity=%v angular=%v", delta, body.Velocity(), body.AngularVelocity())
			}
		}
	}
}

func TestBoxRestitutionSurvivesSolverIterations(t *testing.T) {
	for _, iterations := range []int{1, 2, 10} {
		world := NewWorld()
		world.Gravity, world.Iterations = Vector{}, iterations
		floor := world.CreateBody(NewBodyDef(BodyStatic))
		AddCollider(world, floor, NewColliderDef(NewBox(20, 2)))
		def := NewBodyDef(BodyDynamic)
		def.Position, def.Velocity = V(0, 2.1), V(0, -10)
		body := world.CreateBody(def)
		collider := NewColliderDef(NewBox(2, 2))
		collider.Restitution = .5
		AddCollider(world, body, collider)
		world.Step(1.0 / 60)
		if body.Velocity().Sub(V(0, 5)).Length() > 1e-9 || math.Abs(body.AngularVelocity()) > 1e-9 {
			t.Fatalf("iterations=%v: bounce velocity=%v angular=%v", iterations, body.Velocity(), body.AngularVelocity())
		}
	}
}

func TestPositionCorrectionsDiscoverNewContacts(t *testing.T) {
	world := NewWorld()
	world.Gravity = Vector{}
	def := NewBodyDef(BodyDynamic)
	body := world.CreateBody(def)
	AddCollider(world, body, NewColliderDef(Circle{Radius: 1}))
	def.Position = V(1.5, 0)
	neighbor := world.CreateBody(def)
	AddCollider(world, neighbor, NewColliderDef(Circle{Radius: 1}))
	def.Type, def.Position = BodyStatic, V(-2.1, 0)
	wall := world.CreateBody(def)
	AddCollider(world, wall, NewColliderDef(Circle{Radius: 1}))
	world.Step(1.0 / 60)
	if body.Position().X < -.111 || neighbor.Position().X-body.Position().X < 1.98 {
		t.Fatalf("correction missed new wall contact: body=%v neighbor=%v", body.Position(), neighbor.Position())
	}
}

func TestWorld2DAllocationFreeLifecycle(t *testing.T) {
	world := NewWorld()
	world.Reserve(2, 2)
	allocations := testing.AllocsPerRun(100, func() {
		body := world.CreateBody(NewBodyDef(BodyDynamic))
		AddCollider(world, body, NewColliderDef(Circle{Radius: 1}))
		world.RemoveBody(body)
	})
	if allocations != 0 {
		t.Fatalf("reserved body lifecycle allocations = %v, want 0", allocations)
	}
}

func TestWorld2DSensorStepReusesStorage(t *testing.T) {
	world := benchmarkWorld2D(64, true)
	if allocations := testing.AllocsPerRun(100, func() { world.Step(1.0 / 60) }); allocations != 0 {
		t.Fatalf("sensor Step allocations = %v, want 0", allocations)
	}
}
