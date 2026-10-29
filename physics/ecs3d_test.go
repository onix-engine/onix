package physics

import (
	"math"
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func Test3DTransformWritebackPreservesSimulationPrecision(t *testing.T) {
	world := ecs.NewWorld()
	runtime := NewRuntime3D()
	runtime.Simulation.Gravity = Vector3D{}
	world.InsertResource(runtime)
	bundle := ecs.NewBundle(2)
	bundle.Put(scene.TransformFromXYZ(1e6, 0, 0))
	body := NewRigidBody3D(BodyDynamic)
	body.Velocity = V3(.02, 0, 0)
	bundle.Put(body)
	entity := world.Spawn(bundle)
	for range 120 {
		if err := StepSystem3D("").Run(world); err != nil {
			t.Fatal(err)
		}
	}
	body, _ = world.Get[RigidBody3D](entity)
	if math.Abs(body.Handle().Position().X-(1e6+.04)) > 1e-7 {
		t.Fatalf("float32 writeback discarded motion: position=%v", body.Handle().Position())
	}
}

func Test3DStepSystemSimulatesOnly3DComponentBodies(t *testing.T) {
	world := ecs.NewWorld()
	world.InsertResource(&scene.Time{Delta: time.Second / 60})
	runtime := NewRuntime3D()
	runtime.Simulation.Gravity = V3(0, -20, 0)
	world.InsertResource(runtime)

	floorBundle := ecs.NewBundle(3)
	floorBundle.Put(scene.TransformFromXYZ(0, 0, 0))
	floorBundle.Put(NewRigidBody3D(BodyStatic))
	floorBundle.Put(NewCollider3D(NewBox3D(20, 2, 20)))
	world.Spawn(floorBundle)

	ballBundle := ecs.NewBundle(3)
	ballBundle.Put(scene.TransformFromXYZ(0, 8, 0))
	ballBundle.Put(NewRigidBody3D(BodyDynamic))
	ballBundle.Put(NewCollider3D(Sphere{Radius: 1}))
	ball := world.Spawn(ballBundle)

	plainBundle := ecs.NewBundle(1)
	plainBundle.Put(scene.TransformFromXYZ(3, 8, 4))
	plain := world.Spawn(plainBundle)

	system := StepSystem3D("")
	for range 360 {
		if err := system.Run(world); err != nil {
			t.Fatal(err)
		}
		world.AdvanceMessages()
	}
	transform, _ := world.Get[scene.Transform](ball)
	if transform.Translation.Y() < 1.9 || transform.Translation.Y() > 2.1 {
		t.Fatalf("physics entity y = %v, want approximately 2", transform.Translation.Y())
	}
	plainTransform, _ := world.Get[scene.Transform](plain)
	if plainTransform.Translation != [3]float32{3, 8, 4} {
		t.Fatalf("entity without RigidBody3D moved to %v", plainTransform.Translation)
	}
}

func Test3DDynamicTransformEditTeleports3DBody(t *testing.T) {
	world := ecs.NewWorld()
	world.InsertResource(&scene.Time{Delta: time.Second / 60})
	runtime := NewRuntime3D()
	runtime.Simulation.Gravity = Vector3D{}
	world.InsertResource(runtime)
	bundle := ecs.NewBundle(3)
	bundle.Put(scene.TransformFromXYZ(0, 0, 0))
	bundle.Put(NewRigidBody3D(BodyDynamic))
	bundle.Put(NewCollider3D(Sphere{Radius: 1}))
	entity := world.Spawn(bundle)
	system := StepSystem3D("")
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	transform := world.GetMut[scene.Transform](entity)
	transform.Translation = [3]float32{40, 30, -20}
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	body, _ := world.Get[RigidBody3D](entity)
	if got := body.Handle().Position(); got != V3(40, 30, -20) {
		t.Fatalf("teleported body position = %+v", got)
	}
}

func Test3DStepSystemEmitsEntityCollisionMessages(t *testing.T) {
	world := ecs.NewWorld()
	world.InsertResource(&scene.Time{Delta: time.Second / 60})
	runtime := NewRuntime3D()
	runtime.Simulation.Gravity = Vector3D{}
	world.InsertResource(runtime)

	spawn := func(bodyType BodyType, collisionType uint32) ecs.Entity {
		bundle := ecs.NewBundle(3)
		bundle.Put(scene.TransformFromXYZ(0, 0, 0))
		bundle.Put(NewRigidBody3D(bodyType))
		collider := NewCollider3D(Sphere{Radius: 1})
		collider.Sensor, collider.CollisionType = true, collisionType
		bundle.Put(collider)
		return world.Spawn(bundle)
	}
	first, second := spawn(BodyDynamic, 10), spawn(BodyStatic, 20)
	if err := StepSystem3D("").Run(world); err != nil {
		t.Fatal(err)
	}
	reader := ecs.MessageReader[CollisionMessage3D]{}
	messages := reader.Read(world)
	if len(messages) != 2 {
		t.Fatalf("collision message count = %d, want 2", len(messages))
	}
	if messages[0].Entity != first || messages[0].Other != second || messages[0].CollisionType != 10 || messages[0].OtherType != 20 {
		t.Fatalf("first collision message = %+v", messages[0])
	}
	if messages[1].Entity != second || messages[1].Other != first || messages[1].Normal != messages[0].Normal.Mul(-1) {
		t.Fatalf("second collision message = %+v", messages[1])
	}
}

func Test3DStepSystemReclaimsRemovedComponents(t *testing.T) {
	world := ecs.NewWorld()
	world.InsertResource(&scene.Time{Delta: time.Second / 60})
	bundle := ecs.NewBundle(3)
	bundle.Put(scene.TransformFromXYZ(0, 0, 0))
	bundle.Put(NewRigidBody3D(BodyDynamic))
	bundle.Put(NewCollider3D(Sphere{Radius: 1}))
	entity := world.Spawn(bundle)
	system := StepSystem3D("")
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	body, _ := world.Get[RigidBody3D](entity)
	handle := body.Handle()
	if !handle.Valid() {
		t.Fatal("managed body was not created")
	}
	world.Remove[RigidBody3D](entity)
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	if handle.Valid() {
		t.Fatal("removed RigidBody3D left a live simulation body")
	}
}
