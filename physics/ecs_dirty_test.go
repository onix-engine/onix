package physics

import (
	"slices"
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/scene"
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

// Replacing a body and copying a live collider component both require checking
// the owner even when the component still holds a valid collider handle.
func checkColliderOwnership[B, C any, ID comparable](t *testing.T, body B, collider C, system ecs.SystemConfig, id func(*C) ID, events func(*ecs.World, ID) []CollisionPhase) {
	t.Helper()
	w := ecs.NewWorld()
	bundle := ecs.NewBundle(3)
	bundle.Put(scene.NewTransform())
	bundle.Put(body)
	bundle.Put(collider)
	first, other := w.Spawn(bundle), w.Spawn(bundle)
	step := func() {
		t.Helper()
		if err := system.Run(w); err != nil {
			t.Fatal(err)
		}
	}
	step()
	original := id(w.GetRef[C](first))
	if phases := events(w, original); !slices.Equal(phases, []CollisionPhase{CollisionBegin}) {
		t.Fatalf("initial contact: %v", phases)
	}
	*w.GetMut[B](first) = body
	step()
	replacement := id(w.GetRef[C](first))
	if replacement == original || !slices.Equal(events(w, original), []CollisionPhase{CollisionEnd}) ||
		!slices.Equal(events(w, replacement), []CollisionPhase{CollisionBegin}) {
		t.Fatal("replacing the rigid body must end the old contact and begin the new one in the same step")
	}
	*w.GetMut[C](other) = *w.GetRef[C](first)
	step()
	copyID := id(w.GetRef[C](other))
	if copyID == replacement || !slices.Equal(events(w, copyID), []CollisionPhase{CollisionBegin}) {
		t.Fatal("copied collider must contact its source with an independent handle")
	}
	step()
	if !slices.Equal(events(w, replacement), []CollisionPhase{CollisionStay}) {
		t.Fatal("replacement collider lost its contact")
	}
}

func TestPhysics2DColliderOwnership(t *testing.T) {
	collider := NewCollider2D(Circle{Radius: 1})
	collider.Sensor = true
	checkColliderOwnership(t, NewRigidBody2D(BodyStatic), collider, StepSystem2D(""),
		func(c *CircleCollider2D) ColliderID { return c.Handle().ID() },
		func(w *ecs.World, id ColliderID) (phases []CollisionPhase) {
			r, _ := w.GetResource[*Runtime2D]()
			for _, e := range r.Simulation.Events() {
				if e.ColliderA == id || e.ColliderB == id {
					phases = append(phases, e.Phase)
				}
			}
			return phases
		})
}

func TestPhysics3DColliderOwnership(t *testing.T) {
	collider := NewCollider3D(Sphere{Radius: 1})
	collider.Sensor = true
	checkColliderOwnership(t, NewRigidBody3D(BodyStatic), collider, StepSystem3D(""),
		func(c *SphereCollider3D) ColliderID3D { return c.Handle().ID() },
		func(w *ecs.World, id ColliderID3D) (phases []CollisionPhase) {
			r, _ := w.GetResource[*Runtime3D]()
			for _, e := range r.Simulation.Events() {
				if e.ColliderA == id || e.ColliderB == id {
					phases = append(phases, e.Phase)
				}
			}
			return phases
		})
}
