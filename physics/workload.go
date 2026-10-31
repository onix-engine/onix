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
	versions         [11]uint64
	scope            *ecs.QueryScope
	scopeVersion     uint64
	bodyMembers      uint64
	transformMembers uint64
}

func (inputs physicsInputs) bodyChanged(previous physicsInputs) bool {
	if inputs.bodyTopologyChanged(previous) {
		return true
	}
	for index := range 5 {
		if inputs.versions[index] != previous.versions[index] {
			return true
		}
	}
	return false
}

// Structural changes still need an ownership sweep. Value edits can be
// synchronized locally, including transforms on ancestors without rigid bodies.
func (inputs physicsInputs) bodyTopologyChanged(previous physicsInputs) bool {
	return inputs.scope != previous.scope || inputs.scopeVersion != previous.scopeVersion ||
		inputs.bodyMembers != previous.bodyMembers || inputs.transformMembers != previous.transformMembers ||
		inputs.versions[2] != previous.versions[2] || inputs.versions[3] != previous.versions[3] || inputs.versions[4] != previous.versions[4]
}

type physicsBodyChanges struct {
	context         ecs.SystemContext
	entities, queue []ecs.Entity
	marks           []uint64
	epoch           uint64
}

func (dirty *physicsBodyChanges) collect[T any](world *ecs.World, since ecs.Tick, current, previous physicsInputs) []ecs.Entity {
	dirty.context = world.ChangesSince(since)
	dirty.entities, dirty.queue = dirty.entities[:0], dirty.queue[:0]
	dirty.epoch += 4
	if dirty.epoch == 0 {
		clear(dirty.marks)
		dirty.epoch = 4
	}
	if slots := world.EntitySlots(); len(dirty.marks) < slots {
		dirty.marks = append(dirty.marks, make([]uint64, slots-len(dirty.marks))...)
	}
	if current.versions[0] != previous.versions[0] {
		for entity := range world.Query2[T, scene.Transform](dirty.context.Changed[T]()).All {
			dirty.addBody[T](world, entity)
		}
	}
	if current.versions[1] != previous.versions[1] {
		hierarchy := scene.HierarchyFor(world)
		for entity := range world.Query[scene.Transform](ecs.Unscoped(), dirty.context.Changed[scene.Transform]()).All {
			dirty.enqueue(entity)
		}
		for index := 0; index < len(dirty.queue); index++ {
			entity := dirty.queue[index]
			dirty.addBody[T](world, entity)
			for child := hierarchy.FirstChild(entity); child != 0; child = hierarchy.NextSibling(child) {
				dirty.enqueue(child)
			}
		}
	}
	return dirty.entities
}

func (dirty *physicsBodyChanges) mark(entity ecs.Entity, bit uint64) bool {
	index, _ := entity.Index()
	mark := &dirty.marks[index]
	if *mark&^3 != dirty.epoch {
		*mark = dirty.epoch
	}
	if *mark&bit != 0 {
		return false
	}
	*mark |= bit
	return true
}

func (dirty *physicsBodyChanges) addBody[T any](world *ecs.World, entity ecs.Entity) {
	if world.Has[T](entity) && world.Has[scene.Transform](entity) && world.InQueryScope(entity) && scene.IsActive(world, entity) && dirty.mark(entity, 1) {
		dirty.entities = append(dirty.entities, entity)
	}
}

func (dirty *physicsBodyChanges) enqueue(entity ecs.Entity) {
	if dirty.mark(entity, 2) {
		dirty.queue = append(dirty.queue, entity)
	}
}

func (inputs physicsInputs) collidersChanged(previous physicsInputs) bool {
	for index := 5; index < len(inputs.versions); index++ {
		if inputs.versions[index] != previous.versions[index] {
			return true
		}
	}
	return false
}

func valueVersion[T any](world *ecs.World) uint64 {
	version, _ := world.ComponentValueRevision[T]()
	return version
}

func bodyInputs[T any](world *ecs.World) physicsInputs {
	scope, version := world.QueryScopeKey()
	bodyValues, bodyMembers := world.ComponentValueRevision[T]()
	_, transformMembers := world.ComponentValueRevision[scene.Transform]()
	return physicsInputs{scope: scope, scopeVersion: version, bodyMembers: bodyMembers, transformMembers: transformMembers, versions: [11]uint64{
		bodyValues, valueVersion[scene.Transform](world),
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
	_, membership := world.ComponentValueRevision[scene.Transform]()
	return scene.ActiveTaskCount[pendingBodies[T], T](world, membership, func(w *ecs.World, e ecs.Entity, _ *T) bool {
		return w.Has[scene.Transform](e)
	}) != 0
}

// TaskCount2D includes active ECS simulation bodies, manually created bodies,
// and pending synchronization. Idle requires all external inputs to be consumed.
func TaskCount2D(world *ecs.World) int {
	r, _ := world.GetResource[*Runtime2D]()
	previous, managed := physicsInputs{}, 0
	if r != nil {
		previous, managed = r.syncedInputs, r.managedBodies
	}
	pending := pendingBodySync[RigidBody2D](world, previous, physicsInputs2D(world), managed)
	if r != nil && r.Simulation != nil && r.Simulation.Sleep.Enabled {
		sim := r.Simulation
		pending = pending || sim.sleepInputsChanged()
		count := 0
		for e, a := range world.Query[RigidBody2D]().All {
			b := a.Ref()
			if scene.IsActive(world, e) && world.Has[scene.Transform](e) && b.Type != BodyStatic && (pending || !b.body.IsSleeping()) {
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

	_, membership := world.ComponentValueRevision[scene.Transform]()
	count := scene.ActiveTaskCount[tasks2D, RigidBody2D](world, membership, func(w *ecs.World, e ecs.Entity, b *RigidBody2D) bool {
		return w.Has[scene.Transform](e) && b.Type != BodyStatic
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
	r, _ := world.GetResource[*Runtime3D]()
	previous, managed := physicsInputs{}, 0
	if r != nil {
		previous, managed = r.syncedInputs, r.managedBodies
	}
	pending := pendingBodySync[RigidBody3D](world, previous, physicsInputs3D(world), managed)
	_, membership := world.ComponentValueRevision[scene.Transform]()
	count := scene.ActiveTaskCount[tasks3D, RigidBody3D](world, membership, func(w *ecs.World, e ecs.Entity, b *RigidBody3D) bool {
		return w.Has[scene.Transform](e) && b.Type != BodyStatic
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
