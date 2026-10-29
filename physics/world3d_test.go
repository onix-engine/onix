package physics

import (
	"math"
	"testing"
)

func Test3DDynamicShapesSettleOnStaticBox(t *testing.T) {
	tests := []struct {
		name      string
		add       func(*World3D, Body3D)
		position  Vector3D
		settledAt float64
	}{
		{name: "sphere", add: func(world *World3D, body Body3D) {
			world.AddCollider(body, NewColliderDef(Sphere{Radius: 1}))
		}, position: V3(-4, 8, 0), settledAt: 2},
		{name: "box", add: func(world *World3D, body Body3D) {
			world.AddCollider(body, NewColliderDef(NewBox3D(1, 1, 1)))
		}, position: V3(0, 8, 0), settledAt: 1.5},
		{name: "capsule", add: func(world *World3D, body Body3D) {
			world.AddCollider(body, NewColliderDef(NewCapsule(4, .5)))
		}, position: V3(4, 8, 0), settledAt: 3},
	}

	world := NewWorld3D()
	world.Gravity = V3(0, -20, 0)
	world.Iterations = 10
	floor := world.CreateBody(NewBodyDef3D(BodyStatic))
	world.AddCollider(floor, NewColliderDef(NewBox3D(30, 2, 30)))
	for _, test := range tests {
		def := NewBodyDef3D(BodyDynamic)
		def.Position = test.position
		body := world.CreateBody(def)
		test.add(world, body)
		for range 360 {
			world.Step(1.0 / 60)
		}
		if position := body.Position(); math.Abs(position.Y-test.settledAt) > .08 {
			t.Errorf("%s settled at %+v, want y approximately %v", test.name, position, test.settledAt)
		}
		if velocity := body.Velocity(); math.Abs(velocity.Y) > .2 {
			t.Errorf("%s settled velocity = %+v", test.name, velocity)
		}
	}
}

func Test3DRotatedBoxCollidesWithSphere(t *testing.T) {
	world := NewWorld3D()
	world.Gravity = Vector3D{}
	boxDef := NewBodyDef3D(BodyStatic)
	boxDef.Rotation = QuaternionFromAxisAngle(V3(0, 0, 1), math.Pi/4)
	box := world.CreateBody(boxDef)
	world.AddCollider(box, NewColliderDef(NewBox3D(4, 1, 2)))
	sphereDef := NewBodyDef3D(BodyDynamic)
	sphereDef.Position = V3(1.5, 1.5, 0)
	sphere := world.CreateBody(sphereDef)
	world.AddCollider(sphere, NewColliderDef(Sphere{Radius: 1}))

	world.Step(1.0 / 60)
	if len(world.Events()) != 1 || world.Events()[0].Phase != CollisionBegin {
		t.Fatalf("collision events = %+v", world.Events())
	}
	if sphere.Position() == sphereDef.Position {
		t.Fatal("rotated box collision did not separate sphere")
	}
}

func Test3DEveryShapePairProducesContact(t *testing.T) {
	type addShape func(*World3D, Body3D, bool)
	addSphere := func(world *World3D, body Body3D, sensor bool) {
		definition := NewColliderDef(Sphere{Radius: 1})
		definition.Sensor = sensor
		world.AddCollider(body, definition)
	}
	addBox := func(world *World3D, body Body3D, sensor bool) {
		definition := NewColliderDef(NewBox3D(2, 2, 2))
		definition.Sensor = sensor
		world.AddCollider(body, definition)
	}
	addCapsule := func(world *World3D, body Body3D, sensor bool) {
		definition := NewColliderDef(NewCapsule(3, .5))
		definition.Sensor = sensor
		world.AddCollider(body, definition)
	}
	tests := []struct {
		name   string
		first  addShape
		second addShape
	}{
		{name: "sphere-sphere", first: addSphere, second: addSphere},
		{name: "sphere-box", first: addSphere, second: addBox},
		{name: "sphere-capsule", first: addSphere, second: addCapsule},
		{name: "box-box", first: addBox, second: addBox},
		{name: "box-capsule", first: addBox, second: addCapsule},
		{name: "capsule-capsule", first: addCapsule, second: addCapsule},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			world := NewWorld3D()
			world.Gravity = Vector3D{}
			first := world.CreateBody(NewBodyDef3D(BodyDynamic))
			second := world.CreateBody(NewBodyDef3D(BodyStatic))
			test.first(world, first, true)
			test.second(world, second, true)
			world.Step(1.0 / 60)
			if events := world.Events(); len(events) != 1 || events[0].Phase != CollisionBegin {
				t.Fatalf("events = %+v", events)
			}
		})
	}
}

func Test3DFilterAndSensorCollisionEvents(t *testing.T) {
	world := NewWorld3D()
	world.Gravity = Vector3D{}
	a := world.CreateBody(NewBodyDef3D(BodyDynamic))
	b := world.CreateBody(NewBodyDef3D(BodyStatic))
	first := NewColliderDef(Sphere{Radius: 2})
	first.Sensor = true
	first.Filter = NewFilter(0, 1, 1)
	second := NewColliderDef(Sphere{Radius: 2})
	second.Filter = NewFilter(0, 2, 2)
	firstCollider := world.AddCollider(a, first)
	secondCollider := world.AddCollider(b, second)
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
	if a.Position() != (Vector3D{}) {
		t.Fatalf("sensor moved body to %+v", a.Position())
	}
	b.SetPosition(V3(20, 0, 0))
	world.Step(1.0 / 60)
	if events := world.Events(); len(events) != 1 || events[0].Phase != CollisionEnd {
		t.Fatalf("end events = %+v", events)
	}
}

func Test3DAngularVelocityIntegratesOrientation(t *testing.T) {
	world := NewWorld3D()
	world.Gravity = Vector3D{}
	def := NewBodyDef3D(BodyDynamic)
	def.AngularVelocity = V3(0, math.Pi, 0)
	body := world.CreateBody(def)
	world.Step(.5)
	forward := body.Rotation().Rotate(V3(0, 0, 1))
	if forward.X < .85 || math.Abs(forward.Z) > .4 {
		t.Fatalf("rotated forward = %+v", forward)
	}
}

func Test3DStepReusesScratchStorage(t *testing.T) {
	world := NewWorld3D()
	world.Reserve(64, 64)
	world.Gravity = V3(0, -10, 0)
	floor := world.CreateBody(NewBodyDef3D(BodyStatic))
	world.AddCollider(floor, NewColliderDef(NewBox3D(100, 2, 100)))
	for index := range 24 {
		def := NewBodyDef3D(BodyDynamic)
		def.Position = V3(float64(index%6)*3-8, 4+float64(index/6)*3, float64(index%4)*3-5)
		body := world.CreateBody(def)
		world.AddCollider(body, NewColliderDef(Sphere{Radius: 1}))
	}
	for range 180 {
		world.Step(1.0 / 60)
	}
	allocations := testing.AllocsPerRun(100, func() { world.Step(1.0 / 60) })
	if allocations != 0 {
		t.Fatalf("steady-state Step allocations = %v, want 0", allocations)
	}
}
