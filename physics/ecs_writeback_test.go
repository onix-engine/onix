package physics

import (
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/scene"
)

func TestPhysicsWritebackTracksOnlyAffectedBodies(t *testing.T) {
	for _, dimension := range []string{"2D", "3D"} {
		t.Run(dimension, func(t *testing.T) {
			w := ecs.NewWorld()
			plain := ecs.NewBundle(1)
			plain.Put(scene.NewTransform())
			bodyBundle := ecs.NewBundle(3)
			bodyBundle.Put(scene.NewTransform())
			step := StepSystem2D("")
			var stats func() StepStatistics
			var setPosition func(ecs.Entity, float64)
			var x func(ecs.Entity) float64
			var setVelocity func(ecs.Entity, float64)
			var velocity func(ecs.Entity) float64
			if dimension == "2D" {
				r := NewRuntime2D()
				w.InsertResource(r)
				bodyBundle.Put(NewRigidBody2D(BodyStatic))
				stats = r.Simulation.Statistics
				setPosition = func(e ecs.Entity, v float64) { w.GetRef[RigidBody2D](e).Handle().SetPosition(V(v, 0)) }
				x = func(e ecs.Entity) float64 { return w.GetRef[RigidBody2D](e).Handle().Position().X }
				setVelocity = func(e ecs.Entity, v float64) { w.GetRef[RigidBody2D](e).Handle().SetVelocity(V(v, 0)) }
				velocity = func(e ecs.Entity) float64 { return w.GetRef[RigidBody2D](e).Velocity.X }
			} else {
				r := NewRuntime3D()
				w.InsertResource(r)
				step = StepSystem3D("")
				bodyBundle.Put(NewRigidBody3D(BodyStatic))
				stats = r.Simulation.Statistics
				setPosition = func(e ecs.Entity, v float64) { w.GetRef[RigidBody3D](e).Handle().SetPosition(V3(v, 0, 0)) }
				x = func(e ecs.Entity) float64 { return w.GetRef[RigidBody3D](e).Handle().Position().X }
				setVelocity = func(e ecs.Entity, v float64) { w.GetRef[RigidBody3D](e).Handle().SetVelocity(V3(v, 0, 0)) }
				velocity = func(e ecs.Entity) float64 { return w.GetRef[RigidBody3D](e).Velocity.X }
			}
			run := func() {
				t.Helper()
				if err := step.Run(w); err != nil {
					t.Fatal(err)
				}
			}
			parent, other := w.Spawn(bodyBundle), w.Spawn(bodyBundle)
			for range 128 {
				w.Spawn(bodyBundle)
			}
			bridge := w.Spawn(plain)
			if err := w.Insert(bridge, scene.Parent{Entity: parent}); err != nil {
				t.Fatal(err)
			}
			bodyBundle.Put(scene.Parent{Entity: bridge})
			child := w.Spawn(bodyBundle)
			*w.GetMut[scene.Transform](child) = scene.TransformFromXYZ(2, 0, 0)
			run()
			run()
			if count := stats().WritebackBodies; count != 0 {
				t.Fatalf("unchanged bodies visited=%d", count)
			}
			setVelocity(parent, 3)
			run()
			if count := stats().WritebackBodies; count != 1 || velocity(parent) != 3 {
				t.Fatalf("velocity-only writeback: count=%d velocity=%g", count, velocity(parent))
			}
			setPosition(parent, 4)
			setPosition(parent, 5)
			run()
			if count := stats().WritebackBodies; count != 2 || x(child) != 2 || w.GetRef[scene.Transform](child).Translation[0] != -3 {
				t.Fatalf("teleport through intermediate transform: count=%d child world=%v local=%v", count, x(child), w.GetRef[scene.Transform](child).Translation)
			}
			*w.GetMut[scene.Parent](bridge) = scene.Parent{Entity: other}
			run()
			if x(child) != -3 {
				t.Fatalf("reparented child world=%g", x(child))
			}
			w.Spawn(plain)
			run()
			if count := stats().WritebackBodies; count != 0 {
				t.Fatalf("unrelated spawn visited %d bodies", count)
			}
			toggle := float64(0)
			if allocs := testing.AllocsPerRun(40, func() { toggle = 1 - toggle; setPosition(other, toggle); run() }); allocs != 0 || stats().WritebackBodies != 2 {
				t.Fatalf("warm hierarchy writeback: allocs=%g count=%d", allocs, stats().WritebackBodies)
			}
		})
	}
}
