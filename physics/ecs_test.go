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
	ecs.InsertResource(world, &scene.Time{Delta: time.Second / 60})
	runtime := NewRuntime2D()
	runtime.Simulation.Gravity = V(0, -20)
	ecs.InsertResource(world, runtime)

	floorBundle := ecs.NewBundle(4)
	ecs.Put(&floorBundle, scene.TransformFromXYZ(0, 0, 0))
	ecs.Put(&floorBundle, scene.GlobalTransform{})
	ecs.Put(&floorBundle, NewRigidBody2D(BodyStatic))
	ecs.Put(&floorBundle, NewCollider2D(NewBox(20, 2)))
	world.Spawn(floorBundle)

	ballBundle := ecs.NewBundle(4)
	ecs.Put(&ballBundle, scene.TransformFromXYZ(0, 8, 0))
	ecs.Put(&ballBundle, scene.GlobalTransform{})
	ecs.Put(&ballBundle, NewRigidBody2D(BodyDynamic))
	ecs.Put(&ballBundle, NewCollider2D(Circle{Radius: 1}))
	ball := world.Spawn(ballBundle)

	plainBundle := ecs.NewBundle(2)
	ecs.Put(&plainBundle, scene.TransformFromXYZ(3, 8, 0))
	ecs.Put(&plainBundle, scene.GlobalTransform{})
	plain := world.Spawn(plainBundle)

	system := StepSystem2D("")
	for range 360 {
		if err := system.Run(world); err != nil {
			t.Fatal(err)
		}
		ecs.AdvanceMessages(world)
	}
	transform, _ := ecs.Get[scene.Transform](world, ball)
	if transform.Translation.Y() < 1.9 || transform.Translation.Y() > 2.1 {
		t.Fatalf("physics node y = %v, want approximately 2", transform.Translation.Y())
	}
	plainTransform, _ := ecs.Get[scene.Transform](world, plain)
	if plainTransform.Translation.Y() != 8 {
		t.Fatalf("node without RigidBody2D moved to %v", plainTransform.Translation.Y())
	}
}

func TestDynamicPoseRetainsPrecisionBetweenECSWrites(t *testing.T) {
	world := ecs.NewWorld()
	delta := time.Second / 60
	ecs.InsertResource(world, &scene.Time{Delta: delta})
	runtime := NewRuntime2D()
	runtime.Simulation.Gravity = Vector{}
	ecs.InsertResource(world, runtime)
	bundle := ecs.NewBundle(2)
	ecs.Put(&bundle, scene.TransformFromXYZ(1e6, 0, 0))
	component := NewRigidBody2D(BodyDynamic)
	component.Velocity, component.AngularVelocity = V(.01, .02), .123
	ecs.Put(&bundle, component)
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
	result, _ := ecs.Get[RigidBody2D](world, entity)
	if result.Handle().Position().Sub(position).Length() > 1e-10 || math.Abs(result.Handle().Rotation()-rotation) > 1e-12 {
		t.Fatalf("ECS rounded simulation pose: position=%v want=%v rotation=%v want=%v", result.Handle().Position(), position, result.Handle().Rotation(), rotation)
	}
}

func TestRemovingTransformDoesNotRecreateOrphanColliders(t *testing.T) {
	world := ecs.NewWorld()
	runtime := NewRuntime2D()
	runtime.Simulation.Gravity = Vector{}
	ecs.InsertResource(world, runtime)
	bundle := ecs.NewBundle(3)
	ecs.Put(&bundle, scene.TransformFromXYZ(0, 0, 0))
	ecs.Put(&bundle, NewRigidBody2D(BodyDynamic))
	ecs.Put(&bundle, NewCollider2D(Circle{Radius: 1}))
	entity := world.Spawn(bundle)
	system := StepSystem2D("")
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	body, _ := ecs.Get[RigidBody2D](world, entity)
	collider, _ := ecs.Get[CircleCollider2D](world, entity)
	ecs.Remove[scene.Transform](world, entity)
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
	ecs.InsertResource(world, &scene.Time{Delta: time.Second / 60})
	runtime := NewRuntime2D()
	runtime.Simulation.Gravity = Vector{}
	ecs.InsertResource(world, runtime)
	bodyBundle := ecs.NewBundle(3)
	ecs.Put(&bodyBundle, scene.TransformFromXYZ(0, 0, 0))
	ecs.Put(&bodyBundle, NewRigidBody2D(BodyDynamic))
	ecs.Put(&bodyBundle, NewCollider2D(Circle{Radius: 1}))
	entity := world.Spawn(bodyBundle)
	system := StepSystem2D("")
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	transform := ecs.GetMut[scene.Transform](world, entity)
	transform.Translation[0], transform.Translation[1] = 40, 30
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	body, _ := ecs.Get[RigidBody2D](world, entity)
	if got := body.Handle().Position(); got.X != 40 || got.Y != 30 {
		t.Fatalf("teleported body position = %+v", got)
	}
}
