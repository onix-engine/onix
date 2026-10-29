package onix

import (
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	enginephysics "github.com/diiyw/onix/physics"
	"github.com/diiyw/onix/scene"
)

func TestPhysics2DSystemSetStepsNodeComponents(t *testing.T) {
	app := NewApp()
	if err := app.InstallSystems[Physics2D](); err != nil {
		t.Fatal(err)
	}
	runtime, ok := app.World.GetResource[*enginephysics.Runtime2D]()
	if !ok || runtime == nil || runtime.Simulation == nil {
		t.Fatal("Physics2D did not install its runtime resource")
	}
	runtime.Simulation.Gravity = enginephysics.V(0, -20)
	app.World.InsertResource(&scene.Time{Delta: time.Second / 60})

	floorBundle := ecs.NewBundle(4)
	floorBundle.Put(scene.TransformFromXYZ(0, 0, 0))
	floorBundle.Put(scene.GlobalTransform{})
	floorBundle.Put(enginephysics.NewRigidBody2D(enginephysics.BodyStatic))
	floorBundle.Put(enginephysics.NewCollider2D(enginephysics.NewBox(20, 2)))
	app.World.Spawn(floorBundle)

	ballBundle := ecs.NewBundle(4)
	ballBundle.Put(scene.TransformFromXYZ(0, 8, 0))
	ballBundle.Put(scene.GlobalTransform{})
	ballBundle.Put(enginephysics.NewRigidBody2D(enginephysics.BodyDynamic))
	ballBundle.Put(enginephysics.NewCollider2D(enginephysics.Circle{Radius: 1}))
	ball := app.World.Spawn(ballBundle)

	for range 60 {
		if err := app.Schedule(StageFixedUpdate).Run(app.World); err != nil {
			t.Fatal(err)
		}
		if err := app.Schedule(StageFixedPostUpdate).Run(app.World); err != nil {
			t.Fatal(err)
		}
	}
	transform, _ := app.World.Get[scene.Transform](ball)
	global, _ := app.World.Get[scene.GlobalTransform](ball)
	if transform.Translation.Y() >= 8 || global.Matrix[13] != transform.Translation.Y() {
		t.Fatalf("physics transform was not synchronized: local=%v global=%v", transform.Translation.Y(), global.Matrix[13])
	}
}

func TestPhysics3DSystemSetStepsEntityComponents(t *testing.T) {
	app := NewApp()
	if err := app.InstallSystems[Physics3D](); err != nil {
		t.Fatal(err)
	}
	runtime, ok := app.World.GetResource[*enginephysics.Runtime3D]()
	if !ok || runtime == nil || runtime.Simulation == nil {
		t.Fatal("Physics3D did not install its runtime resource")
	}
	runtime.Simulation.Gravity = enginephysics.V3(0, -20, 0)
	app.World.InsertResource(&scene.Time{Delta: time.Second / 60})

	floorBundle := ecs.NewBundle(4)
	floorBundle.Put(scene.TransformFromXYZ(0, 0, 0))
	floorBundle.Put(scene.GlobalTransform{})
	floorBundle.Put(enginephysics.NewRigidBody3D(enginephysics.BodyStatic))
	floorBundle.Put(enginephysics.NewCollider3D(enginephysics.NewBox3D(20, 2, 20)))
	app.World.Spawn(floorBundle)

	ballBundle := ecs.NewBundle(4)
	ballBundle.Put(scene.TransformFromXYZ(0, 8, 0))
	ballBundle.Put(scene.GlobalTransform{})
	ballBundle.Put(enginephysics.NewRigidBody3D(enginephysics.BodyDynamic))
	ballBundle.Put(enginephysics.NewCollider3D(enginephysics.Sphere{Radius: 1}))
	ball := app.World.Spawn(ballBundle)

	for range 60 {
		if err := app.Schedule(StageFixedUpdate).Run(app.World); err != nil {
			t.Fatal(err)
		}
		if err := app.Schedule(StageFixedPostUpdate).Run(app.World); err != nil {
			t.Fatal(err)
		}
	}
	transform, _ := app.World.Get[scene.Transform](ball)
	global, _ := app.World.Get[scene.GlobalTransform](ball)
	if transform.Translation.Y() >= 8 || global.Matrix[13] != transform.Translation.Y() {
		t.Fatalf("3D physics transform was not synchronized: local=%v global=%v", transform.Translation.Y(), global.Matrix[13])
	}
}
