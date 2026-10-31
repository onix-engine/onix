package physics

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func TestPhysics2DSkipsStableInputSyncAndResumesAfterEdit(t *testing.T) {
	world := ecs.NewWorld()
	runtime := NewRuntime2D()
	runtime.Simulation.Gravity = Vector{}
	world.InsertResource(runtime)
	bundle := ecs.NewBundle(3)
	bundle.Put(scene.TransformFromXYZ(0, 0, 0))
	bundle.Put(NewRigidBody2D(BodyStatic))
	bundle.Put(NewCollider2D(Circle{Radius: 1}))
	entity := world.Spawn(bundle)
	system := StepSystem2D("")
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	body, _ := world.Get[RigidBody2D](entity)
	firstTouch := body.body.state().touched
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	if got := body.body.state().touched; got != firstTouch {
		t.Fatalf("stable body synced again: touched %d → %d", firstTouch, got)
	}
	world.GetMut[scene.Transform](entity).Translation[0] = 4
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	if got := body.body.state(); got.touched != runtime.frame || got.position.X != 4 {
		t.Fatalf("edited body was not synchronized: touched=%d position=%v", got.touched, got.position)
	}
}

func TestPhysics3DSkipsStableInputSyncAndResumesAfterEdit(t *testing.T) {
	world := ecs.NewWorld()
	runtime := NewRuntime3D()
	runtime.Simulation.Gravity = Vector3D{}
	world.InsertResource(runtime)
	bundle := ecs.NewBundle(3)
	bundle.Put(scene.TransformFromXYZ(0, 0, 0))
	bundle.Put(NewRigidBody3D(BodyStatic))
	bundle.Put(NewCollider3D(Sphere{Radius: 1}))
	entity := world.Spawn(bundle)
	system := StepSystem3D("")
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	body, _ := world.Get[RigidBody3D](entity)
	firstTouch := body.body.state().touched
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	if got := body.body.state().touched; got != firstTouch {
		t.Fatalf("stable body synced again: touched %d → %d", firstTouch, got)
	}
	world.GetMut[scene.Transform](entity).Translation[0] = 4
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	if got := body.body.state(); got.touched != runtime.frame || got.position.X != 4 {
		t.Fatalf("edited body was not synchronized: touched=%d position=%v", got.touched, got.position)
	}
}

func TestPhysics2DColliderEditSkipsBodySync(t *testing.T) {
	world := ecs.NewWorld()
	runtime := NewRuntime2D()
	runtime.Simulation.Gravity = Vector{}
	world.InsertResource(runtime)
	bundle := ecs.NewBundle(3)
	bundle.Put(scene.TransformFromXYZ(0, 0, 0))
	bundle.Put(NewRigidBody2D(BodyStatic))
	bundle.Put(NewCollider2D(Circle{Radius: 1}))
	entity := world.Spawn(bundle)
	system := StepSystem2D("")
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	body, _ := world.Get[RigidBody2D](entity)
	initialTouch := body.body.state().touched
	world.GetMut[CircleCollider2D](entity).Shape.Radius = 2
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	if body.body.state().touched != initialTouch {
		t.Fatal("collider edit synchronized an unchanged body")
	}
	collider, _ := world.Get[CircleCollider2D](entity)
	if collider.Handle().Shape().Radius != 2 {
		t.Fatal("collider edit did not reach the solver")
	}
	world.Remove[CircleCollider2D](entity)
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	if collider.Handle().Valid() || !body.Handle().Valid() || body.body.state().touched != initialTouch {
		t.Fatal("collider removal changed the body or left a live collider")
	}
}

func TestPhysics3DColliderEditSkipsBodySync(t *testing.T) {
	world := ecs.NewWorld()
	runtime := NewRuntime3D()
	runtime.Simulation.Gravity = Vector3D{}
	world.InsertResource(runtime)
	bundle := ecs.NewBundle(3)
	bundle.Put(scene.TransformFromXYZ(0, 0, 0))
	bundle.Put(NewRigidBody3D(BodyStatic))
	bundle.Put(NewCollider3D(Sphere{Radius: 1}))
	entity := world.Spawn(bundle)
	system := StepSystem3D("")
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	body, _ := world.Get[RigidBody3D](entity)
	initialTouch := body.body.state().touched
	world.GetMut[SphereCollider3D](entity).Shape.Radius = 2
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	if body.body.state().touched != initialTouch {
		t.Fatal("collider edit synchronized an unchanged body")
	}
	collider, _ := world.Get[SphereCollider3D](entity)
	if collider.Handle().Shape().Radius != 2 {
		t.Fatal("collider edit did not reach the solver")
	}
	world.Remove[SphereCollider3D](entity)
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	if collider.Handle().Valid() || !body.Handle().Valid() || body.body.state().touched != initialTouch {
		t.Fatal("collider removal changed the body or left a live collider")
	}
}

func TestPhysics2DBodyEditSynchronizesOnlyChangedEntity(t *testing.T) {
	world := ecs.NewWorld()
	runtime := NewRuntime2D()
	runtime.Simulation.Gravity = Vector{}
	world.InsertResource(runtime)
	bundle := ecs.NewBundle(2)
	bundle.Put(scene.TransformFromXYZ(0, 0, 0))
	bundle.Put(NewRigidBody2D(BodyStatic))
	first, second := world.Spawn(bundle), world.Spawn(bundle)
	system := StepSystem2D("")
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	secondBody, _ := world.Get[RigidBody2D](second)
	secondTouch := secondBody.body.state().touched
	world.GetMut[RigidBody2D](first).Mass = 3
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	firstBody, _ := world.Get[RigidBody2D](first)
	if firstBody.body.state().touched != runtime.frame || firstBody.body.state().mass != 3 || secondBody.body.state().touched != secondTouch {
		t.Fatal("body edit synchronized an unchanged entity or missed the edited one")
	}
	old := firstBody.Handle()
	*world.GetMut[RigidBody2D](first) = NewRigidBody2D(BodyStatic)
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	firstBody, _ = world.Get[RigidBody2D](first)
	if old.Valid() || !firstBody.Handle().Valid() || secondBody.body.state().touched != runtime.frame {
		t.Fatal("body replacement did not run the full ownership sweep")
	}
}

func TestPhysics3DBodyEditSynchronizesOnlyChangedEntity(t *testing.T) {
	world := ecs.NewWorld()
	runtime := NewRuntime3D()
	runtime.Simulation.Gravity = Vector3D{}
	world.InsertResource(runtime)
	bundle := ecs.NewBundle(2)
	bundle.Put(scene.TransformFromXYZ(0, 0, 0))
	bundle.Put(NewRigidBody3D(BodyStatic))
	first, second := world.Spawn(bundle), world.Spawn(bundle)
	system := StepSystem3D("")
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	secondBody, _ := world.Get[RigidBody3D](second)
	secondTouch := secondBody.body.state().touched
	world.GetMut[RigidBody3D](first).Mass = 3
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	firstBody, _ := world.Get[RigidBody3D](first)
	if firstBody.body.state().touched != runtime.frame || firstBody.body.state().mass != 3 || secondBody.body.state().touched != secondTouch {
		t.Fatal("body edit synchronized an unchanged entity or missed the edited one")
	}
	old := firstBody.Handle()
	*world.GetMut[RigidBody3D](first) = NewRigidBody3D(BodyStatic)
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	firstBody, _ = world.Get[RigidBody3D](first)
	if old.Valid() || !firstBody.Handle().Valid() || secondBody.body.state().touched != runtime.frame {
		t.Fatal("body replacement did not run the full ownership sweep")
	}
}
