package physics

import (
	"fmt"
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
	"math"
	"testing"
)

func TestPhysicsPoseDoesNotResetBetweenTicks(t *testing.T) {
	w := ecs.NewWorld()
	r := NewRuntime2D()
	r.Simulation.Gravity = Vector{}
	ecs.InsertResource(w, r)
	b := ecs.NewBundle(2)
	ecs.Put(&b, scene.TransformFromXYZ(0, 8, 0))
	body := NewRigidBody2D(BodyDynamic)
	body.Velocity = V(6, 0)
	ecs.Put(&b, body)
	e := w.Spawn(b)
	for range 60 {
		if err := scene.SyncGlobalTransformSystem("").Run(w); err != nil {
			t.Fatal(err)
		}
		if err := StepSystem2D("").Run(w); err != nil {
			t.Fatal(err)
		}
	}
	tr, _ := ecs.Get[scene.Transform](w, e)
	if math.Abs(float64(tr.Translation[0]-6)) > .001 || tr.Translation[1] != 8 {
		t.Fatalf("pose reset between ticks: %v", tr.Translation)
	}
}

func TestPhysicsRejectsScaledAndTiltedAncestry(t *testing.T) {
	for _, planar := range []bool{true, false} {
		w := ecs.NewWorld()
		parent := scene.TransformFromXYZ(10, 20, 0)
		parent.Scale = mgl32.Vec3{2, 2, 2}
		bundle := ecs.NewBundle(1)
		ecs.Put(&bundle, parent)
		p := w.Spawn(bundle)
		child := ecs.NewBundle(3)
		ecs.Put(&child, scene.Parent{Entity: p})
		ecs.Put(&child, scene.TransformFromXYZ(0, 0, 0))
		step := StepSystem3D("")
		if planar {
			ecs.Put(&child, NewRigidBody2D(BodyDynamic))
			step = StepSystem2D("")
		} else {
			ecs.Put(&child, NewRigidBody3D(BodyDynamic))
		}
		w.Spawn(child)
		if err := step.Run(w); err == nil {
			t.Fatal("scaled ancestry silently changed collision geometry")
		}
		parent.Scale = mgl32.Vec3{1, 1, 1}
		parent.Rotation = mgl32.QuatRotate(.2, mgl32.Vec3{1, 0, 0})
		*ecs.GetMut[scene.Transform](w, p) = parent
		if err := step.Run(w); (err != nil) != planar {
			t.Fatal("incorrect planar rotation validation", planar, err)
		}
	}
}

func TestPhysicsPlacementWriteback(t *testing.T) {
	w := ecs.NewWorld()
	r := NewRuntime2D()
	r.Simulation.Gravity = Vector{}
	ecs.InsertResource(w, r)
	p := scene.TransformFromXYZ(100, 20, 0)
	p.Rotation = mgl32.QuatRotate(float32(math.Pi/2), mgl32.Vec3{0, 0, 1})
	pb := ecs.NewBundle(1)
	ecs.Put(&pb, p)
	parent := w.Spawn(pb)
	b := ecs.NewBundle(3)
	ecs.Put(&b, scene.TransformFromXYZ(2, 0, 0))
	ecs.Put(&b, scene.Parent{Entity: parent})
	body := NewRigidBody2D(BodyDynamic)
	body.Velocity = V(6, 0)
	ecs.Put(&b, body)
	e := w.Spawn(b)
	for range 60 {
		if err := StepSystem2D("").Run(w); err != nil {
			t.Fatal(err)
		}
	}
	result, _ := ecs.Get[RigidBody2D](w, e)
	if math.Abs(result.Handle().Position().X-106) > .001 || math.Abs(result.Handle().Position().Y-22) > .001 {
		t.Fatalf("world pose=%v", result.Handle().Position())
	}
	local, _ := ecs.Get[scene.Transform](w, e)
	if math.Abs(float64(local.Translation[0]-2)) > .001 || math.Abs(float64(local.Translation[1]+6)) > .001 {
		t.Fatalf("local pose=%v", local.Translation)
	}
}

func TestPhysics3DPlacementAndNestedBodyWriteback(t *testing.T) {
	w := ecs.NewWorld()
	r := NewRuntime3D()
	r.Simulation.Gravity = Vector3D{}
	ecs.InsertResource(w, r)
	p := scene.TransformFromXYZ(100, 20, 30)
	p.Rotation = mgl32.QuatRotate(float32(math.Pi/2), mgl32.Vec3{0, 0, 1})
	pb := ecs.NewBundle(2)
	ecs.Put(&pb, p)
	parentBody := NewRigidBody3D(BodyDynamic)
	parentBody.Velocity = V3(3, 0, 0)
	ecs.Put(&pb, parentBody)
	parent := w.Spawn(pb)
	b := ecs.NewBundle(3)
	ecs.Put(&b, scene.TransformFromXYZ(2, 0, 1))
	ecs.Put(&b, scene.Parent{Entity: parent})
	body := NewRigidBody3D(BodyDynamic)
	body.Velocity = V3(6, 0, 0)
	ecs.Put(&b, body)
	e := w.Spawn(b)
	system := StepSystem3D("")
	for range 60 {
		if err := system.Run(w); err != nil {
			t.Fatal(err)
		}
	}
	result, _ := ecs.Get[RigidBody3D](w, e)
	if result.Handle().Position().Sub(V3(106, 22, 31)).Length() > .001 {
		t.Fatalf("world pose=%v", result.Handle().Position())
	}
	local, _ := ecs.Get[scene.Transform](w, e)
	if local.Translation.Sub(mgl32.Vec3{2, -3, 1}).Len() > .001 {
		t.Fatalf("local pose=%v", local.Translation)
	}
	if allocs := testing.AllocsPerRun(100, func() {
		if err := system.Run(w); err != nil {
			panic(err)
		}
	}); allocs != 0 {
		t.Fatalf("warm nested body simulation allocated %g", allocs)
	}
}

func TestPhysicsActivityPauseAndResume(t *testing.T) {
	for _, dimensions := range []int{2, 3} {
		t.Run(fmt.Sprint(dimensions), func(t *testing.T) {
			w := ecs.NewWorld()
			b := ecs.NewBundle(3)
			ecs.Put(&b, scene.TransformFromXYZ(0, 8, 0))
			ecs.Put(&b, scene.Activity{Active: true})
			var system ecs.SystemConfig
			if dimensions == 2 {
				r := NewRuntime2D()
				r.Simulation.Gravity = Vector{}
				ecs.InsertResource(w, r)
				body := NewRigidBody2D(BodyDynamic)
				body.Velocity = V(6, 0)
				ecs.Put(&b, body)
				system = StepSystem2D("")
			} else {
				r := NewRuntime3D()
				r.Simulation.Gravity = Vector3D{}
				ecs.InsertResource(w, r)
				body := NewRigidBody3D(BodyDynamic)
				body.Velocity = V3(6, 0, 0)
				ecs.Put(&b, body)
				system = StepSystem3D("")
			}
			e := w.Spawn(b)
			step := func() {
				t.Helper()
				if err := system.Run(w); err != nil {
					t.Fatal(err)
				}
			}
			step()
			before, _ := ecs.Get[scene.Transform](w, e)
			ecs.GetMut[scene.Activity](w, e).Active = false
			for range 60 {
				step()
			}
			paused, _ := ecs.Get[scene.Transform](w, e)
			if paused != before {
				t.Fatal("inactive physics advanced", paused, before)
			}
			ecs.GetMut[scene.Activity](w, e).Active = true
			for range 60 {
				step()
			}
			after, _ := ecs.Get[scene.Transform](w, e)
			if math.Abs(float64(after.Translation[0]-before.Translation[0]-6)) > .001 {
				t.Fatal("resume lost pose or velocity", after)
			}
		})
	}
}
