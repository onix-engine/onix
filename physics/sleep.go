package physics

import "math"

// SleepConfig enables whole contact-island sleeping. The zero value disables
// sleeping; nonpositive thresholds use the defaults below (world units/second,
// radians/second, and seconds respectively).
type SleepConfig struct {
	Enabled                                          bool
	LinearThreshold, AngularThreshold, TimeThreshold float64
}

func (config SleepConfig) normalized() SleepConfig {
	if !(config.LinearThreshold > 0) || math.IsInf(config.LinearThreshold, 0) {
		config.LinearThreshold = .05
	}
	if !(config.AngularThreshold > 0) || math.IsInf(config.AngularThreshold, 0) {
		config.AngularThreshold = .05
	}
	if !(config.TimeThreshold > 0) || math.IsInf(config.TimeThreshold, 0) {
		config.TimeThreshold = .5
	}
	return config
}

type sleepBodyInput struct {
	generation                                                                                    uint32
	alive                                                                                         bool
	bodyType                                                                                      BodyType
	position, velocity, force                                                                     Vector
	rotation, angularVelocity, torque, mass, inertia, gravityScale, linearDamping, angularDamping float64
	fixedRotation                                                                                 bool
}

func sleepInput(body *bodyState) sleepBodyInput {
	return sleepBodyInput{body.generation, body.alive, body.bodyType, body.position, body.velocity, body.force, body.rotation, body.angularVelocity, body.torque, body.mass, body.inertia, body.gravityScale, body.linearDamping, body.angularDamping, body.fixedRotation}
}

type sleepWakeNode struct {
	generation uint32
	first      int // one-based edge index
}

type sleepWakeEdge struct {
	body, next int
}

type sleepState struct {
	wakeNodes   []sleepWakeNode
	wakeEdges   []sleepWakeEdge
	wakeQueue   []int
	inputs      []sleepBodyInput
	colliders   []colliderState
	parents     []int
	flags       []bool
	config      SleepConfig
	gravity     Vector
	damping     float64
	initialized bool
}

func (world *World) islandRoot(i int) int {
	for world.sleep.parents[i] != i {
		world.sleep.parents[i] = world.sleep.parents[world.sleep.parents[i]]
		i = world.sleep.parents[i]
	}
	return i
}
func (world *World) contactIslands() {
	n := len(world.bodies)
	world.sleep.parents = growCapacity(world.sleep.parents, n)[:n]
	world.sleep.flags = growCapacity(world.sleep.flags, n)[:n]
	for i := range world.bodies {
		world.sleep.parents[i] = i
		world.sleep.flags[i] = false
	}
	for _, contact := range world.contacts {
		if contact.sensor {
			continue
		}
		a, b := int(world.colliders[contact.pair.a].bodyIndex), int(world.colliders[contact.pair.b].bodyIndex)
		if world.bodies[a].bodyType == BodyDynamic && world.bodies[b].bodyType == BodyDynamic {
			a, b = world.islandRoot(a), world.islandRoot(b)
			world.sleep.parents[b] = a
		}
	}
}
func (world *World) markWake(index int) {
	if index >= len(world.bodies) {
		return
	}
	world.bodies[index].wakeContacts = true
	world.sleep.flags[world.islandRoot(index)] = true
}

func (world *World) wakeMarked() {
	// Propagate all changed supports in one contact pass, rather than scanning
	// every contact for each modified static/kinematic body.
	for _, contact := range world.contacts {
		if contact.sensor {
			continue
		}
		a, b := int(world.colliders[contact.pair.a].bodyIndex), int(world.colliders[contact.pair.b].bodyIndex)
		if world.bodies[a].wakeContacts && world.bodies[a].bodyType != BodyDynamic {
			world.sleep.flags[world.islandRoot(b)] = true
		}
		if world.bodies[b].wakeContacts && world.bodies[b].bodyType != BodyDynamic {
			world.sleep.flags[world.islandRoot(a)] = true
		}
	}
	for i := range world.bodies {
		if world.sleep.flags[world.islandRoot(i)] {
			world.bodies[i].sleeping = false
			world.bodies[i].quietTime = 0
		}
	}
}
func (world *World) beginSleep() {
	if !world.Sleep.Enabled && !world.sleep.initialized {
		return
	}
	world.contactIslands()
	all := world.Sleep != world.sleep.config || world.Gravity != world.sleep.gravity || world.Damping != world.sleep.damping
	for i := range world.bodies {
		if all || i >= len(world.sleep.inputs) || sleepInput(&world.bodies[i]) != world.sleep.inputs[i] {
			world.markWake(i)
		}
	}
	for i := range world.colliders {
		current := world.colliders[i]
		current.touched = 0
		if i >= len(world.sleep.colliders) || current != world.sleep.colliders[i] {
			world.markWake(int(current.bodyIndex))
			if i < len(world.sleep.colliders) {
				world.markWake(int(world.sleep.colliders[i].bodyIndex))
			}
		}
	}
	world.wakeMarked()
}
func (world *World) wakeContactIslands() {
	if !world.Sleep.Enabled {
		return
	}
	world.contactIslands()
	for i := range world.bodies {
		body := &world.bodies[i]
		if body.alive && body.bodyType == BodyDynamic && !body.sleeping {
			world.sleep.flags[world.islandRoot(i)] = true
		}
	}
	for _, contact := range world.contacts {
		if contact.sensor {
			continue
		}
		a, b := int(world.colliders[contact.pair.a].bodyIndex), int(world.colliders[contact.pair.b].bodyIndex)
		if world.bodies[a].wakeContacts {
			world.sleep.flags[world.islandRoot(b)] = true
		}
		if world.bodies[b].wakeContacts {
			world.sleep.flags[world.islandRoot(a)] = true
		}
		if world.bodies[a].bodyType == BodyKinematic && (world.bodies[a].velocity != (Vector{}) || world.bodies[a].angularVelocity != 0) {
			world.sleep.flags[world.islandRoot(b)] = true
		}
		if world.bodies[b].bodyType == BodyKinematic && (world.bodies[b].velocity != (Vector{}) || world.bodies[b].angularVelocity != 0) {
			world.sleep.flags[world.islandRoot(a)] = true
		}
	}
	for i := range world.bodies {
		if world.bodies[i].sleeping && world.sleep.flags[world.islandRoot(i)] {
			world.bodies[i].sleeping = false
			world.bodies[i].quietTime = 0
		}
	}
}
func (world *World) finishSleep(delta float64) {
	if !world.Sleep.Enabled {
		world.sleep.initialized = false
		return
	}
	config := world.Sleep.normalized()
	world.contactIslands()
	for i := range world.bodies {
		body := &world.bodies[i]
		if !body.alive || body.bodyType != BodyDynamic {
			continue
		}
		if body.velocity.LengthSquared() > config.LinearThreshold*config.LinearThreshold || math.Abs(body.angularVelocity) > config.AngularThreshold {
			body.quietTime = 0
		} else {
			body.quietTime += delta
		}
		if body.quietTime < config.TimeThreshold {
			world.sleep.flags[world.islandRoot(i)] = true
		}
	}
	for _, contact := range world.contacts {
		if contact.sensor {
			continue
		}
		a, b := int(world.colliders[contact.pair.a].bodyIndex), int(world.colliders[contact.pair.b].bodyIndex)
		if contact.penetration > max(world.CollisionSlop*2, .02) {
			world.sleep.flags[world.islandRoot(a)] = true
			world.sleep.flags[world.islandRoot(b)] = true
		}
		if world.bodies[a].bodyType == BodyKinematic && (world.bodies[a].velocity != (Vector{}) || world.bodies[a].angularVelocity != 0) {
			world.sleep.flags[world.islandRoot(b)] = true
		}
		if world.bodies[b].bodyType == BodyKinematic && (world.bodies[b].velocity != (Vector{}) || world.bodies[b].angularVelocity != 0) {
			world.sleep.flags[world.islandRoot(a)] = true
		}
	}
	world.sleep.inputs = growCapacity(world.sleep.inputs, len(world.bodies))[:len(world.bodies)]
	for i := range world.bodies {
		body := &world.bodies[i]
		if body.alive && body.bodyType == BodyDynamic && !world.sleep.flags[world.islandRoot(i)] {
			body.sleeping = true
			body.velocity = Vector{}
			body.angularVelocity = 0
		}
		if body.alive && body.sleeping {
			world.stats.SleepingBodies++
		}
		body.wakeContacts = false
		world.sleep.inputs[i] = sleepInput(body)
	}
	world.sleep.colliders = growCapacity(world.sleep.colliders, len(world.colliders))[:len(world.colliders)]
	for i, c := range world.colliders {
		c.touched = 0
		world.sleep.colliders[i] = c
	}
	world.sleep.config, world.sleep.gravity, world.sleep.damping, world.sleep.initialized = world.Sleep, world.Gravity, world.Damping, true
	world.cacheWakeContacts()
}
func (world *World) contactAwake(contact *manifold) bool {
	a, b := &world.bodies[world.colliders[contact.pair.a].bodyIndex], &world.bodies[world.colliders[contact.pair.b].bodyIndex]
	return a.bodyType == BodyDynamic && !a.sleeping || b.bodyType == BodyDynamic && !b.sleeping
}

// Retain the final solid-contact graph for immediate wake requests between
// steps. Body generations keep reused slots from inheriting old neighbours.
func (world *World) cacheWakeContacts() {
	s := &world.sleep
	s.wakeNodes = growCapacity(s.wakeNodes, len(world.bodies))[:len(world.bodies)]
	s.wakeQueue = growCapacity(s.wakeQueue, len(world.bodies))[:0]
	s.wakeEdges = s.wakeEdges[:0]
	for i, body := range world.bodies {
		s.wakeNodes[i] = sleepWakeNode{generation: body.generation}
	}
	for _, contact := range world.contacts {
		if contact.sensor {
			continue
		}
		a, b := int(world.colliders[contact.pair.a].bodyIndex), int(world.colliders[contact.pair.b].bodyIndex)
		s.wakeEdges = append(s.wakeEdges, sleepWakeEdge{body: b, next: s.wakeNodes[a].first})
		s.wakeNodes[a].first = len(s.wakeEdges)
		s.wakeEdges = append(s.wakeEdges, sleepWakeEdge{body: a, next: s.wakeNodes[b].first})
		s.wakeNodes[b].first = len(s.wakeEdges)
	}
}

// Wake immediately wakes this body's contact island. A changed support wakes
// its neighbours, without joining separate islands across static supports.
// Repeated requests share the traversal until the next physics step.
func (body Body) Wake() {
	state := body.state()
	if state == nil {
		return
	}
	world := body.world
	if !world.Sleep.Enabled && !world.sleep.initialized {
		return
	}
	state.sleeping, state.quietTime = false, 0
	if state.wakeContacts {
		return
	}
	state.wakeContacts = true
	index, _ := splitID(uint64(body.id))
	s := &world.sleep
	if int(index) >= len(s.wakeNodes) || s.wakeNodes[index].generation != state.generation {
		return
	}
	s.wakeQueue = append(s.wakeQueue[:0], int(index))
	for i := 0; i < len(s.wakeQueue); i++ {
		for edge := s.wakeNodes[s.wakeQueue[i]].first; edge != 0; {
			link := s.wakeEdges[edge-1]
			edge = link.next
			other := &world.bodies[link.body]
			if !other.alive || other.bodyType != BodyDynamic || other.wakeContacts || s.wakeNodes[link.body].generation != other.generation {
				continue
			}
			other.sleeping, other.quietTime, other.wakeContacts = false, 0, true
			s.wakeQueue = append(s.wakeQueue, link.body)
		}
	}
}

func (world *World) sleepInputsChanged() bool {
	if !world.sleep.initialized || world.Sleep != world.sleep.config || world.Gravity != world.sleep.gravity || world.Damping != world.sleep.damping || len(world.bodies) != len(world.sleep.inputs) || len(world.colliders) != len(world.sleep.colliders) {
		return true
	}
	for i := range world.bodies {
		if sleepInput(&world.bodies[i]) != world.sleep.inputs[i] {
			return true
		}
	}
	for i, c := range world.colliders {
		c.touched = 0
		if c != world.sleep.colliders[i] {
			return true
		}
	}
	return false
}
