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
	if err := InstallSystems[Physics2D](app); err != nil {
		t.Fatal(err)
	}
	runtime, ok := ecs.GetResource[*enginephysics.Runtime2D](app.World)
	if !ok || runtime == nil || runtime.Simulation == nil {
		t.Fatal("Physics2D did not install its runtime resource")
	}
	runtime.Simulation.Gravity = enginephysics.V(0, -20)
	ecs.InsertResource(app.World, &scene.Time{Delta: time.Second / 60})

	floorBundle := ecs.NewBundle(4)
	ecs.Put(&floorBundle, scene.TransformFromXYZ(0, 0, 0))
	ecs.Put(&floorBundle, scene.GlobalTransform{})
	ecs.Put(&floorBundle, enginephysics.NewRigidBody2D(enginephysics.BodyStatic))
	ecs.Put(&floorBundle, enginephysics.NewCollider2D(enginephysics.NewBox(20, 2)))
	app.World.Spawn(floorBundle)

	ballBundle := ecs.NewBundle(4)
	ecs.Put(&ballBundle, scene.TransformFromXYZ(0, 8, 0))
	ecs.Put(&ballBundle, scene.GlobalTransform{})
	ecs.Put(&ballBundle, enginephysics.NewRigidBody2D(enginephysics.BodyDynamic))
	ecs.Put(&ballBundle, enginephysics.NewCollider2D(enginephysics.Circle{Radius: 1}))
	ball := app.World.Spawn(ballBundle)

	for range 60 {
		if err := app.Schedule(StageFixedUpdate).Run(app.World); err != nil {
			t.Fatal(err)
		}
		if err := app.Schedule(StageFixedPostUpdate).Run(app.World); err != nil {
			t.Fatal(err)
		}
	}
	transform, _ := ecs.Get[scene.Transform](app.World, ball)
	global, _ := ecs.Get[scene.GlobalTransform](app.World, ball)
	if transform.Translation.Y() >= 8 || global.Matrix[13] != transform.Translation.Y() {
		t.Fatalf("physics transform was not synchronized: local=%v global=%v", transform.Translation.Y(), global.Matrix[13])
	}
}

func TestPhysics3DSystemSetStepsEntityComponents(t *testing.T) {
	app := NewApp()
	if err := InstallSystems[Physics3D](app); err != nil {
		t.Fatal(err)
	}
	runtime, ok := ecs.GetResource[*enginephysics.Runtime3D](app.World)
	if !ok || runtime == nil || runtime.Simulation == nil {
		t.Fatal("Physics3D did not install its runtime resource")
	}
	runtime.Simulation.Gravity = enginephysics.V3(0, -20, 0)
	ecs.InsertResource(app.World, &scene.Time{Delta: time.Second / 60})

	floorBundle := ecs.NewBundle(4)
	ecs.Put(&floorBundle, scene.TransformFromXYZ(0, 0, 0))
	ecs.Put(&floorBundle, scene.GlobalTransform{})
	ecs.Put(&floorBundle, enginephysics.NewRigidBody3D(enginephysics.BodyStatic))
	ecs.Put(&floorBundle, enginephysics.NewCollider3D(enginephysics.NewBox3D(20, 2, 20)))
	app.World.Spawn(floorBundle)

	ballBundle := ecs.NewBundle(4)
	ecs.Put(&ballBundle, scene.TransformFromXYZ(0, 8, 0))
	ecs.Put(&ballBundle, scene.GlobalTransform{})
	ecs.Put(&ballBundle, enginephysics.NewRigidBody3D(enginephysics.BodyDynamic))
	ecs.Put(&ballBundle, enginephysics.NewCollider3D(enginephysics.Sphere{Radius: 1}))
	ball := app.World.Spawn(ballBundle)

	for range 60 {
		if err := app.Schedule(StageFixedUpdate).Run(app.World); err != nil {
			t.Fatal(err)
		}
		if err := app.Schedule(StageFixedPostUpdate).Run(app.World); err != nil {
			t.Fatal(err)
		}
	}
	transform, _ := ecs.Get[scene.Transform](app.World, ball)
	global, _ := ecs.Get[scene.GlobalTransform](app.World, ball)
	if transform.Translation.Y() >= 8 || global.Matrix[13] != transform.Translation.Y() {
		t.Fatalf("3D physics transform was not synchronized: local=%v global=%v", transform.Translation.Y(), global.Matrix[13])
	}
}
