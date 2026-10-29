package physics

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func TestTaskCountsTrackBodyMembershipAndUnmanagedSimulation(t *testing.T) {
	w := ecs.NewWorld()
	spawner := w.NewSpawner2[RigidBody2D, RigidBody3D]()
	e := spawner.Spawn(NewRigidBody2D(BodyDynamic), NewRigidBody3D(BodyDynamic))
	check := func(want int) {
		t.Helper()
		if TaskCount2D(w) != want || TaskCount3D(w) != want {
			t.Fatalf("2D/3D tasks = %d/%d, want %d", TaskCount2D(w), TaskCount3D(w), want)
		}
	}
	check(0)
	if err := w.Insert(e, scene.NewTransform()); err != nil {
		t.Fatal(err)
	}
	check(1)
	var scope ecs.QueryScope
	scope.Reset(nil)
	w.SetQueryScope(&scope)
	check(0)
	w.SetQueryScope(nil)
	w.Remove[scene.Transform](e)
	check(0)
	r2, r3 := NewRuntime2D(), NewRuntime3D()
	w.InsertResource(r2)
	w.InsertResource(r3)
	r2.Simulation.CreateBody(NewBodyDef(BodyStatic))
	r3.Simulation.CreateBody(NewBodyDef3D(BodyStatic))
	check(0)
	r2.Simulation.CreateBody(NewBodyDef(BodyDynamic))
	r3.Simulation.CreateBody(NewBodyDef3D(BodyKinematic))
	check(1)
}
