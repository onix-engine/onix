package physics

import (
	"fmt"
	"math"
	"testing"
)

func Test3DTiltedBoxComesToRest(t *testing.T) {
	for _, axis := range []Vector3D{V3(1, 0, 0), V3(0, 0, 1), V3(1, 0, 1)} {
		for _, restitution := range []float64{0, .55} {
			t.Run(fmt.Sprint(axis, restitution), func(t *testing.T) {
				world := NewWorld3D()
				world.Gravity, world.Iterations = V3(0, -20, 0), 12
				AddCollider3D(world, world.CreateBody(NewBodyDef3D(BodyStatic)), NewColliderDef(NewBox3D(40, 2, 40)))
				def := NewBodyDef3D(BodyDynamic)
				def.Position, def.Rotation = V3(3, 6, 2), QuaternionFromAxisAngle(axis, .3)
				body := world.CreateBody(def)
				collider := NewColliderDef(NewBox3D(2, 2, 2))
				collider.Restitution = restitution
				AddCollider3D(world, body, collider)
				for range 1200 {
					world.Step(1.0 / 60)
				}
				position := body.Position()
				for range 300 {
					world.Step(1.0 / 60)
					if body.Velocity().Length() > .01 || body.AngularVelocity().Length() > .001 || body.Position().Sub(position).Length() > .01 {
						t.Fatalf("box keeps moving: position=%v velocity=%v angular=%v", body.Position(), body.Velocity(), body.AngularVelocity())
					}
				}
				if allocations := testing.AllocsPerRun(100, func() { world.Step(1.0 / 60) }); allocations != 0 {
					t.Fatalf("resting box allocations=%v, want 0", allocations)
				}
				body.ApplyImpulse(V3(2, 5, 1), body.Position())
				world.Step(1.0 / 60)
				if body.Position().Y <= position.Y || body.Velocity().Y <= 0 {
					t.Fatal("resting body did not respond to an impulse")
				}
			})
		}
	}
}

func Test3DBoxSlopeFriction(t *testing.T) {
	rotation := QuaternionFromAxisAngle(V3(0, 0, 1), -.18)
	tangent, normal := rotation.Rotate(V3(1, 0, 0)), rotation.Rotate(V3(0, 1, 0))
	for _, friction := range []float64{.6, .02} {
		t.Run(fmt.Sprint(friction), func(t *testing.T) {
			world := NewWorld3D()
			world.Gravity, world.Iterations = V3(0, -20, 0), 12
			floorDef := NewBodyDef3D(BodyStatic)
			floorDef.Rotation = rotation
			floorCollider := NewColliderDef(NewBox3D(40, 2, 40))
			floorCollider.Friction = friction
			AddCollider3D(world, world.CreateBody(floorDef), floorCollider)
			def := NewBodyDef3D(BodyDynamic)
			def.Position, def.Rotation = normal.Mul(2), rotation
			body := world.CreateBody(def)
			collider := NewColliderDef(NewBox3D(2, 2, 2))
			collider.Friction = .65
			AddCollider3D(world, body, collider)
			for range 120 {
				world.Step(1.0 / 60)
			}
			distance, speed := body.Position().Sub(def.Position).Dot(tangent), body.Velocity().Dot(tangent)
			if math.Sqrt(friction*collider.Friction) < math.Tan(.18) {
				if distance < 1 || speed < 1 {
					t.Fatalf("low-friction box did not slide: distance=%v speed=%v", distance, speed)
				}
			} else if math.Abs(distance) > .01 || body.Velocity().Length() > .01 || body.AngularVelocity().Length() > .001 {
				t.Fatalf("static friction did not hold: distance=%v velocity=%v angular=%v", distance, body.Velocity(), body.AngularVelocity())
			}
		})
	}
}

func Test3DSphereInsideBoxUsesNearestExit(t *testing.T) {
	assert3DEmbeddedShapeExits(t, NewBox3D(2, 2, 2), Sphere{Radius: .25})
}

func Test3DSphereInsideHullUsesNearestExit(t *testing.T) {
	assert3DEmbeddedShapeExits(t, testBoxHull3D(t, V3(1, 1, 1)), Sphere{Radius: .25})
}

func Test3DDegenerateCapsuleInsideBoxUsesNearestExit(t *testing.T) {
	assert3DEmbeddedShapeExits(t, NewBox3D(2, 2, 2), NewCapsule(.5, .25))
}

func assert3DEmbeddedShapeExits[F, S Shape3D](t *testing.T, floor F, shape S) {
	t.Helper()
	for _, rotation := range []Quaternion{QuaternionIdentity(), QuaternionFromAxisAngle(V3(1, 2, 3), .7)} {
		for _, position := range []Vector3D{V3(.8, 0, 0), V3(-.8, 0, 0), V3(0, .8, 0), V3(0, -.8, 0), V3(0, 0, .8), V3(0, 0, -.8), V3(0, 1, 0)} {
			world := NewWorld3D()
			world.Gravity, world.CollisionSlop = Vector3D{}, 0
			def := NewBodyDef3D(BodyStatic)
			def.Rotation = rotation
			AddCollider3D(world, world.CreateBody(def), NewColliderDef(floor))
			def = NewBodyDef3D(BodyDynamic)
			def.Position = rotation.Rotate(position)
			body := world.CreateBody(def)
			AddCollider3D(world, body, NewColliderDef(shape))
			world.Step(1.0 / 60)
			want := rotation.Rotate(position.Normalize().Mul(1.25))
			if body.Position().Sub(want).Length() > 1e-5 {
				t.Errorf("local=%v rotation=%v: position=%v want=%v", position, rotation, body.Position(), want)
			}
		}
	}
}

func Test3DFixedRotationAndStaticContactVelocity(t *testing.T) {
	for _, bodyType := range []BodyType{BodyDynamic, BodyKinematic} {
		world := NewWorld3D()
		world.Gravity = Vector3D{}
		def := NewBodyDef3D(bodyType)
		def.FixedRotation, def.AngularVelocity = true, V3(1, 2, 3)
		body := world.CreateBody(def)
		body.SetAngularVelocity(V3(3, 2, 1))
		world.Step(.1)
		if body.Rotation() != QuaternionIdentity() || body.AngularVelocity() != (Vector3D{}) {
			t.Errorf("type=%v: fixed body rotated: rotation=%v angular=%v", bodyType, body.Rotation(), body.AngularVelocity())
		}
	}
	world := NewWorld3D()
	world.Gravity = Vector3D{}
	def := NewBodyDef3D(BodyStatic)
	def.Velocity = V3(10, 0, 0)
	AddCollider3D(world, world.CreateBody(def), NewColliderDef(Sphere{Radius: 1}))
	def = NewBodyDef3D(BodyDynamic)
	def.Position = V3(1.9, 0, 0)
	body := world.CreateBody(def)
	AddCollider3D(world, body, NewColliderDef(Sphere{Radius: 1}))
	world.Step(1.0 / 60)
	if body.Velocity() != (Vector3D{}) {
		t.Fatalf("static body transferred velocity=%v", body.Velocity())
	}
}

func Test3DCapsuleAndHullComeToRest(t *testing.T) {
	t.Run("capsule", func(t *testing.T) {
		assert3DSupportedShapeRests(t, NewBox3D(40, 2, 40), NewCapsule(4, .5), QuaternionFromAxisAngle(V3(0, 0, 1), math.Pi/2+.2))
	})
	t.Run("hull", func(t *testing.T) {
		assert3DSupportedShapeRests(t, testBoxHull3D(t, V3(20, 1, 20)), testBoxHull3D(t, V3(1, 1, 1)), QuaternionFromAxisAngle(V3(1, 0, 1), .3))
	})
	t.Run("box-on-hull", func(t *testing.T) {
		assert3DSupportedShapeRests(t, testBoxHull3D(t, V3(20, 1, 20)), NewBox3D(2, 2, 2), QuaternionFromAxisAngle(V3(1, 0, 1), .3))
	})
	t.Run("hull-on-box", func(t *testing.T) {
		assert3DSupportedShapeRests(t, NewBox3D(40, 2, 40), testBoxHull3D(t, V3(1, 1, 1)), QuaternionFromAxisAngle(V3(1, 0, 1), .3))
	})
	t.Run("compound-on-hull", func(t *testing.T) {
		compound, err := NewCompound([]CompoundChild{{Shape: testBoxHull3D(t, V3(1, 1, 1)), Rotation: QuaternionIdentity()}})
		if err != nil {
			t.Fatal(err)
		}
		assert3DSupportedShapeRests(t, testBoxHull3D(t, V3(20, 1, 20)), compound, QuaternionFromAxisAngle(V3(1, 0, 1), .3))
	})
}

func testBoxHull3D(t *testing.T, half Vector3D) ConvexHull {
	t.Helper()
	var vertices [8]Vector3D
	for index := range vertices {
		vertices[index] = V3(float64(2*(index&1)-1)*half.X, float64(2*((index>>1)&1)-1)*half.Y, float64(2*((index>>2)&1)-1)*half.Z)
	}
	hull, err := NewConvexHull(vertices[:])
	if err != nil {
		t.Fatal(err)
	}
	return hull
}

func assert3DSupportedShapeRests[F, S Shape3D](t *testing.T, floor F, shape S, rotation Quaternion) {
	t.Helper()
	world := NewWorld3D()
	world.Gravity, world.Iterations = V3(0, -20, 0), 12
	AddCollider3D(world, world.CreateBody(NewBodyDef3D(BodyStatic)), NewColliderDef(floor))
	def := NewBodyDef3D(BodyDynamic)
	def.Position, def.Rotation = V3(3, 6, 2), rotation
	body := world.CreateBody(def)
	AddCollider3D(world, body, NewColliderDef(shape))
	for range 1200 {
		world.Step(1.0 / 60)
	}
	position := body.Position()
	for range 300 {
		world.Step(1.0 / 60)
		if body.Velocity().Length() > .01 || body.AngularVelocity().Length() > .001 || body.Position().Sub(position).Length() > .01 {
			t.Fatalf("shape keeps moving: position=%v velocity=%v angular=%v", body.Position(), body.Velocity(), body.AngularVelocity())
		}
	}
	if allocations := testing.AllocsPerRun(100, func() { world.Step(1.0 / 60) }); allocations != 0 {
		t.Fatalf("contact step allocated %v times", allocations)
	}
}

func Test3DBoxStackComesToRest(t *testing.T) {
	world := NewWorld3D()
	world.Gravity, world.Iterations = V3(0, -20, 0), 12
	AddCollider3D(world, world.CreateBody(NewBodyDef3D(BodyStatic)), NewColliderDef(NewBox3D(20, 2, 20)))
	var bodies [4]Body3D
	for index := range bodies {
		def := NewBodyDef3D(BodyDynamic)
		def.Position = V3(0, 2+2.1*float64(index), 0)
		bodies[index] = world.CreateBody(def)
		AddCollider3D(world, bodies[index], NewColliderDef(NewBox3D(2, 2, 2)))
	}
	for range 1200 {
		world.Step(1.0 / 60)
	}
	for range 300 {
		world.Step(1.0 / 60)
		for index, body := range bodies {
			if body.Position().Sub(V3(0, 2+2*float64(index), 0)).Length() > .1 || body.Velocity().Length() > .01 || body.AngularVelocity().Length() > .001 {
				t.Fatalf("box %v did not settle in stack: position=%v velocity=%v angular=%v", index, body.Position(), body.Velocity(), body.AngularVelocity())
			}
		}
	}
}

func Test3DSphereRollsDownSlope(t *testing.T) {
	world := NewWorld3D()
	world.Gravity, world.Iterations = V3(0, -20, 0), 12
	rotation := QuaternionFromAxisAngle(V3(1, 0, 1), -.18)
	normal := rotation.Rotate(V3(0, 1, 0))
	tangent := world.Gravity.Sub(normal.Mul(world.Gravity.Dot(normal))).Normalize()
	def := NewBodyDef3D(BodyStatic)
	def.Rotation = rotation
	AddCollider3D(world, world.CreateBody(def), NewColliderDef(NewBox3D(40, 2, 40)))
	def = NewBodyDef3D(BodyDynamic)
	def.Position = normal.Mul(2)
	body := world.CreateBody(def)
	AddCollider3D(world, body, NewColliderDef(Sphere{Radius: 1}))
	for range 120 {
		world.Step(1.0 / 120)
	}
	contactRadius := body.Position().Dot(normal) - 1
	slip := body.Velocity().Add(body.AngularVelocity().Cross(normal.Mul(-contactRadius)))
	if body.Position().Sub(def.Position).Dot(tangent) < .5 || body.Velocity().Dot(tangent) < 1 || body.AngularVelocity().Length() < 1 || slip.Length() > .01 {
		t.Fatalf("sphere did not roll: position=%v velocity=%v angular=%v slip=%v", body.Position(), body.Velocity(), body.AngularVelocity(), slip)
	}
}

func Test3DBouncySphereComesToRest(t *testing.T) {
	world := NewWorld3D()
	world.Gravity = V3(0, -20, 0)
	AddCollider3D(world, world.CreateBody(NewBodyDef3D(BodyStatic)), NewColliderDef(NewBox3D(20, 2, 20)))
	def := NewBodyDef3D(BodyDynamic)
	def.Position = V3(0, 6, 0)
	body := world.CreateBody(def)
	collider := NewColliderDef(Sphere{Radius: 1})
	collider.Restitution = .75
	AddCollider3D(world, body, collider)
	bounced := false
	for range 1200 {
		world.Step(1.0 / 60)
		bounced = bounced || body.Velocity().Y > 2
	}
	if !bounced {
		t.Fatal("elastic sphere did not bounce")
	}
	for range 300 {
		world.Step(1.0 / 60)
		if math.Abs(body.Position().Y-2) > .01 || body.Velocity().Length() > .01 {
			t.Fatalf("sphere keeps bouncing: position=%v velocity=%v", body.Position(), body.Velocity())
		}
	}
}

func Test3DColliderReuseEndsOldContact(t *testing.T) {
	world := NewWorld3D()
	world.Gravity = Vector3D{}
	body := world.CreateBody(NewBodyDef3D(BodyDynamic))
	other := world.CreateBody(NewBodyDef3D(BodyStatic))
	def := NewColliderDef(Sphere{Radius: 1})
	def.Sensor = true
	old := AddCollider3D(world, body, def)
	AddCollider3D(world, other, def)
	world.Step(1.0 / 60)
	RemoveCollider3D(world, old)
	replacement := AddCollider3D(world, body, def)
	world.Step(1.0 / 60)
	events := world.Events()
	if len(events) != 2 || events[0].Phase != CollisionEnd || events[0].ColliderA != old.ID() || events[1].Phase != CollisionBegin || events[1].ColliderA != replacement.ID() {
		t.Fatalf("reused collider events=%v", events)
	}
}

func Test3DBoxHullRejectsSeparatedRotatedFaces(t *testing.T) {
	world := NewWorld3D()
	world.Gravity = Vector3D{}
	rotation := QuaternionFromAxisAngle(V3(0, 0, 1), math.Pi/4)
	def := NewBodyDef3D(BodyStatic)
	def.Rotation = rotation
	AddCollider3D(world, world.CreateBody(def), NewColliderDef(NewBox3D(4, .2, .2)))
	def.Type, def.Position = BodyDynamic, rotation.Rotate(V3(0, .3, 0))
	AddCollider3D(world, world.CreateBody(def), NewColliderDef(testBoxHull3D(t, V3(2, .1, .1))))
	world.buildBroadphase()
	if len(world.pairs) == 0 {
		t.Fatal("fixture must have overlapping broadphase bounds")
	}
	world.Step(1.0 / 60)
	if len(world.Events()) != 0 {
		t.Fatalf("separated box and hull produced contacts: %v", world.Events())
	}
}

func Test3DEmptyHullDoesNotCollide(t *testing.T) {
	empty := colliderState3D{kind: shapeConvexHull3D}
	body := bodyState3D{rotation: QuaternionIdentity()}
	for _, other := range []colliderState3D{
		{kind: shapeSphere3D, radius: 1},
		{kind: shapeBox3D, half: V3(1, 1, 1)},
		{kind: shapeConvexHull3D, hull: testBoxHull3D(t, V3(1, 1, 1))},
		empty,
	} {
		if _, ok := collideShapes3D(colliderPair{}, empty, body, other, body); ok {
			t.Fatalf("empty hull collided with shape %v", other.kind)
		}
		if _, ok := collideShapes3D(colliderPair{}, other, body, empty, body); ok {
			t.Fatalf("shape %v collided with empty hull", other.kind)
		}
	}
}
