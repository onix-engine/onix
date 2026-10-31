package physics

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func TestPhysicsTransformDirtySubtrees(t *testing.T) {
	for _, dimension := range []string{"2D", "3D"} {
		t.Run(dimension, func(t *testing.T) {
			world := ecs.NewWorld()
			step := StepSystem2D("")
			if dimension == "3D" {
				step = StepSystem3D("")
			}
			run := func() {
				if err := step.Run(world); err != nil {
					t.Fatal(err)
				}
			}
			parentBundle := ecs.NewBundle(1)
			parentBundle.Put(scene.TransformFromXYZ(1, 0, 0))
			parent, unrelated := world.Spawn(parentBundle), world.Spawn(parentBundle)
			bodyBundle := ecs.NewBundle(3)
			bodyBundle.Put(scene.TransformFromXYZ(2, 0, 0))
			if dimension == "2D" {
				bodyBundle.Put(NewRigidBody2D(BodyStatic))
			} else {
				bodyBundle.Put(NewRigidBody3D(BodyStatic))
			}
			other := world.Spawn(bodyBundle)
			bodyBundle.Put(scene.Parent{Entity: parent})
			child := world.Spawn(bodyBundle)
			state := func(entity ecs.Entity) (uint64, float64) {
				if dimension == "2D" {
					body := world.GetRef[RigidBody2D](entity).body.state()
					return body.touched, body.position.X
				}
				body := world.GetRef[RigidBody3D](entity).body.state()
				return body.touched, body.position.X
			}
			run()
			otherTouch, _ := state(other)
			world.GetMut[scene.Transform](parent).Translation[0] += 4
			world.GetMut[scene.Transform](child).Translation[0] += 1
			world.GetMut[scene.Transform](unrelated).Translation[0] += 1
			run()
			childTouch, x := state(child)
			if x != 8 {
				t.Fatalf("ancestor and child edits produced x=%g; want 8", x)
			}
			if touch, _ := state(other); touch != otherTouch {
				t.Fatal("an unrelated rigid body was synchronized")
			}
			world.GetMut[scene.Transform](unrelated).Translation[0] += 1
			run()
			if touch, _ := state(child); touch != childTouch {
				t.Fatal("an unrelated transform synchronized the child")
			}
			world.GetMut[scene.Transform](parent).Scale[0] = 2
			if err := step.Run(world); err == nil {
				t.Fatal("invalid ancestor scaling escaped transform validation")
			}
			world.GetMut[scene.Transform](parent).Scale[0] = 1
			run()
			cycle := func() {
				world.GetMut[scene.Transform](parent).Translation[0] += 1
				world.GetMut[scene.Transform](child).Translation[0] += 1
				if dimension == "2D" {
					world.GetMut[RigidBody2D](child).Mass += 1
				} else {
					world.GetMut[RigidBody3D](child).Mass += 1
				}
				run()
			}
			cycle()
			if allocs := testing.AllocsPerRun(50, cycle); allocs != 0 {
				t.Fatalf("warm subtree synchronization allocated %g times", allocs)
			}
		})
	}
}

func TestPhysicsTransformMembershipStartsPendingBody(t *testing.T) {
	for _, dimension := range []string{"2D", "3D"} {
		t.Run(dimension, func(t *testing.T) {
			world := ecs.NewWorld()
			bundle := ecs.NewBundle(1)
			step := StepSystem2D("")
			if dimension == "2D" {
				bundle.Put(NewRigidBody2D(BodyStatic))
			} else {
				bundle.Put(NewRigidBody3D(BodyStatic))
				step = StepSystem3D("")
			}
			entity := world.Spawn(bundle)
			if err := step.Run(world); err != nil {
				t.Fatal(err)
			}
			if err := world.Insert(entity, scene.TransformFromXYZ(7, 0, 0)); err != nil {
				t.Fatal(err)
			}
			if err := step.Run(world); err != nil {
				t.Fatal(err)
			}
			if dimension == "2D" {
				if state := world.GetRef[RigidBody2D](entity).body.state(); state == nil || state.position.X != 7 {
					t.Fatal("adding a transform failed to create the pending 2D body")
				}
			} else if state := world.GetRef[RigidBody3D](entity).body.state(); state == nil || state.position.X != 7 {
				t.Fatal("adding a transform failed to create the pending 3D body")
			}
		})
	}
}

func TestPhysicsTransformAncestorOutsideQueryScope(t *testing.T) {
	for _, dimension := range []string{"2D", "3D"} {
		t.Run(dimension, func(t *testing.T) {
			world := ecs.NewWorld()
			step := StepSystem2D("")
			spawn := world.NewSpawner1[scene.Transform]()
			parent := spawn.Spawn(scene.TransformFromXYZ(1, 0, 0))
			bundle := ecs.NewBundle(3)
			bundle.Put(scene.Parent{Entity: parent})
			bundle.Put(scene.TransformFromXYZ(2, 0, 0))
			if dimension == "2D" {
				bundle.Put(NewRigidBody2D(BodyStatic))
			} else {
				bundle.Put(NewRigidBody3D(BodyStatic))
				step = StepSystem3D("")
			}
			child, excluded := world.Spawn(bundle), world.Spawn(bundle)
			var scope ecs.QueryScope
			scope.Reset([]ecs.Entity{child})
			world.SetQueryScope(&scope)
			if err := step.Run(world); err != nil {
				t.Fatal(err)
			}
			world.GetMut[scene.Transform](parent).Translation[0] = 5
			if err := step.Run(world); err != nil {
				t.Fatal(err)
			}
			if dimension == "2D" {
				if body := world.GetRef[RigidBody2D](child).body.state(); body == nil || body.position.X != 7 || world.GetRef[RigidBody2D](excluded).body.Valid() {
					t.Fatal("ancestor edit missed the scoped body or synchronized an excluded body")
				}
			} else if body := world.GetRef[RigidBody3D](child).body.state(); body == nil || body.position.X != 7 || world.GetRef[RigidBody3D](excluded).body.Valid() {
				t.Fatal("ancestor edit missed the scoped body or synchronized an excluded body")
			}
		})
	}
}
