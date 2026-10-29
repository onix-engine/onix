package physics

import (
	"math"
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func TestStepSystemSimulatesOnlyComponentBodies(t *testing.T) {
	world := ecs.NewWorld()
	world.InsertResource(&scene.Time{Delta: time.Second / 60})
	runtime := NewRuntime2D()
	runtime.Simulation.Gravity = V(0, -20)
	world.InsertResource(runtime)

	floorBundle := ecs.NewBundle(4)
	floorBundle.Put(scene.TransformFromXYZ(0, 0, 0))
	floorBundle.Put(scene.GlobalTransform{})
	floorBundle.Put(NewRigidBody2D(BodyStatic))
	floorBundle.Put(NewCollider2D(NewBox(20, 2)))
	world.Spawn(floorBundle)

	ballBundle := ecs.NewBundle(4)
	ballBundle.Put(scene.TransformFromXYZ(0, 8, 0))
	ballBundle.Put(scene.GlobalTransform{})
	ballBundle.Put(NewRigidBody2D(BodyDynamic))
	ballBundle.Put(NewCollider2D(Circle{Radius: 1}))
	ball := world.Spawn(ballBundle)

	plainBundle := ecs.NewBundle(2)
	plainBundle.Put(scene.TransformFromXYZ(3, 8, 0))
	plainBundle.Put(scene.GlobalTransform{})
	plain := world.Spawn(plainBundle)

	system := StepSystem2D("")
	for range 360 {
		if err := system.Run(world); err != nil {
			t.Fatal(err)
		}
		world.AdvanceMessages()
	}
	transform, _ := world.Get[scene.Transform](ball)
	if transform.Translation.Y() < 1.9 || transform.Translation.Y() > 2.1 {
		t.Fatalf("physics node y = %v, want approximately 2", transform.Translation.Y())
	}
	plainTransform, _ := world.Get[scene.Transform](plain)
	if plainTransform.Translation.Y() != 8 {
		t.Fatalf("node without RigidBody2D moved to %v", plainTransform.Translation.Y())
	}
}

func TestDynamicPoseRetainsPrecisionBetweenECSWrites(t *testing.T) {
	world := ecs.NewWorld()
	delta := time.Second / 60
	world.InsertResource(&scene.Time{Delta: delta})
	runtime := NewRuntime2D()
	runtime.Simulation.Gravity = Vector{}
	world.InsertResource(runtime)
	bundle := ecs.NewBundle(2)
	bundle.Put(scene.TransformFromXYZ(1e6, 0, 0))
	component := NewRigidBody2D(BodyDynamic)
	component.Velocity, component.AngularVelocity = V(.01, .02), .123
	bundle.Put(component)
	entity := world.Spawn(bundle)
	system := StepSystem2D("")
	position, rotation := V(1e6, 0), 0.0
	for range 200 {
		if err := system.Run(world); err != nil {
			t.Fatal(err)
		}
		position = position.Add(component.Velocity.Mul(delta.Seconds()))
		rotation += component.AngularVelocity * delta.Seconds()
	}
	result, _ := world.Get[RigidBody2D](entity)
	if result.Handle().Position().Sub(position).Length() > 1e-10 || math.Abs(result.Handle().Rotation()-rotation) > 1e-12 {
		t.Fatalf("ECS rounded simulation pose: position=%v want=%v rotation=%v want=%v", result.Handle().Position(), position, result.Handle().Rotation(), rotation)
	}
}

func TestRemovingTransformDoesNotRecreateOrphanColliders(t *testing.T) {
	world := ecs.NewWorld()
	runtime := NewRuntime2D()
	runtime.Simulation.Gravity = Vector{}
	world.InsertResource(runtime)
	bundle := ecs.NewBundle(3)
	bundle.Put(scene.TransformFromXYZ(0, 0, 0))
	bundle.Put(NewRigidBody2D(BodyDynamic))
	bundle.Put(NewCollider2D(Circle{Radius: 1}))
	entity := world.Spawn(bundle)
	system := StepSystem2D("")
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	body, _ := world.Get[RigidBody2D](entity)
	collider, _ := world.Get[CircleCollider2D](entity)
	world.Remove[scene.Transform](entity)
	for range 3 {
		if err := system.Run(world); err != nil {
			t.Fatal(err)
		}
	}
	if body.Handle().Valid() || collider.Handle().Valid() || len(runtime.Simulation.freeBodies) != 1 || len(runtime.Simulation.freeColliders) != 1 {
		t.Fatal("removing Transform left live physics objects")
	}
}

func TestDynamicTransformEditTeleportsBody(t *testing.T) {
	world := ecs.NewWorld()
	world.InsertResource(&scene.Time{Delta: time.Second / 60})
	runtime := NewRuntime2D()
	runtime.Simulation.Gravity = Vector{}
	world.InsertResource(runtime)
	bodyBundle := ecs.NewBundle(3)
	bodyBundle.Put(scene.TransformFromXYZ(0, 0, 0))
	bodyBundle.Put(NewRigidBody2D(BodyDynamic))
	bodyBundle.Put(NewCollider2D(Circle{Radius: 1}))
	entity := world.Spawn(bodyBundle)
	system := StepSystem2D("")
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	transform := world.GetMut[scene.Transform](entity)
	transform.Translation[0], transform.Translation[1] = 40, 30
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	body, _ := world.Get[RigidBody2D](entity)
	if got := body.Handle().Position(); got.X != 40 || got.Y != 30 {
		t.Fatalf("teleported body position = %+v", got)
	}
}
