package physics

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

type tasks2D struct{}
type tasks3D struct{}

// Input revisions include the execution scope: hiding the last managed body
// still requires a fixed step to remove it from the simulation.
type physicsInputs struct {
	versions     [11]uint64
	scope        *ecs.QueryScope
	scopeVersion uint64
}

func valueVersion[T any](world *ecs.World) uint64 {
	version, _ := ecs.ComponentValueRevision[T](world)
	return version
}

func bodyInputs[T any](world *ecs.World) physicsInputs {
	scope, version := world.QueryScopeKey()
	return physicsInputs{scope: scope, scopeVersion: version, versions: [11]uint64{
		valueVersion[T](world), valueVersion[scene.Transform](world),
		valueVersion[scene.Activity](world), valueVersion[scene.Parent](world), world.EntityVersion(),
	}}
}

func physicsInputs2D(world *ecs.World) physicsInputs {
	inputs := bodyInputs[RigidBody2D](world)
	inputs.versions[5] = valueVersion[Collider2D[Circle]](world)
	inputs.versions[6] = valueVersion[Collider2D[Box]](world)
	inputs.versions[7] = valueVersion[Collider2D[Segment]](world)
	return inputs
}

func physicsInputs3D(world *ecs.World) physicsInputs {
	inputs := bodyInputs[RigidBody3D](world)
	inputs.versions[5] = valueVersion[Collider3D[Sphere]](world)
	inputs.versions[6] = valueVersion[Collider3D[Box3D]](world)
	inputs.versions[7] = valueVersion[Collider3D[Capsule]](world)
	inputs.versions[8] = valueVersion[Collider3D[TriangleMesh]](world)
	inputs.versions[9] = valueVersion[Collider3D[ConvexHull]](world)
	inputs.versions[10] = valueVersion[Collider3D[Compound]](world)
	return inputs
}

type pendingBodies[T any] struct{}

func pendingBodySync[T any](world *ecs.World, previous, current physicsInputs, managed int) bool {
	if previous == current {
		return false
	}
	if managed != 0 {
		return true
	}
	_, membership := ecs.ComponentValueRevision[scene.Transform](world)
	return scene.ActiveTaskCount[pendingBodies[T], T](world, membership, func(w *ecs.World, e ecs.Entity, _ *T) bool {
		return ecs.Has[scene.Transform](w, e)
	}) != 0
}

// TaskCount2D includes active ECS simulation bodies, manually created bodies,
// and pending synchronization. Idle requires all external inputs to be consumed.
func TaskCount2D(world *ecs.World) int {
	r, _ := ecs.GetResource[*Runtime2D](world)
	previous, managed := physicsInputs{}, 0
	if r != nil {
		previous, managed = r.syncedInputs, r.managedBodies
	}
	pending := pendingBodySync[RigidBody2D](world, previous, physicsInputs2D(world), managed)
	if r != nil && r.Simulation != nil && r.Simulation.Sleep.Enabled {
		sim := r.Simulation
		pending = pending || sim.sleepInputsChanged()
		count := 0
		for e, a := range ecs.Query[RigidBody2D](world).All {
			b := a.Ref()
			if scene.IsActive(world, e) && ecs.Has[scene.Transform](world, e) && b.Type != BodyStatic && (pending || !b.body.IsSleeping()) {
				count++
			}
		}
		for _, b := range sim.bodies {
			if b.alive && !b.managed && b.bodyType != BodyStatic && (pending || !b.sleeping) {
				count++
			}
		}
		if pending {
			return max(count, 1)
		}
		return count
	}

	_, membership := ecs.ComponentValueRevision[scene.Transform](world)
	count := scene.ActiveTaskCount[tasks2D, RigidBody2D](world, membership, func(w *ecs.World, e ecs.Entity, b *RigidBody2D) bool {
		return ecs.Has[scene.Transform](w, e) && b.Type != BodyStatic
	})
	if r != nil && r.Simulation != nil {
		for _, b := range r.Simulation.bodies {
			if b.alive && !b.managed && b.bodyType != BodyStatic {
				count++
			}
		}
	}
	if pending {
		return max(count, 1)
	}
	return count
}

// TaskCount3D includes non-static bodies and pending synchronization, including
// removal or deactivation of the last managed body.
func TaskCount3D(world *ecs.World) int {
	r, _ := ecs.GetResource[*Runtime3D](world)
	previous, managed := physicsInputs{}, 0
	if r != nil {
		previous, managed = r.syncedInputs, r.managedBodies
	}
	pending := pendingBodySync[RigidBody3D](world, previous, physicsInputs3D(world), managed)
	_, membership := ecs.ComponentValueRevision[scene.Transform](world)
	count := scene.ActiveTaskCount[tasks3D, RigidBody3D](world, membership, func(w *ecs.World, e ecs.Entity, b *RigidBody3D) bool {
		return ecs.Has[scene.Transform](w, e) && b.Type != BodyStatic
	})
	if r != nil && r.Simulation != nil {
		for _, b := range r.Simulation.bodies {
			if b.alive && !b.managed && b.bodyType != BodyStatic {
				count++
			}
		}
	}
	if pending {
		return max(count, 1)
	}
	return count
}
