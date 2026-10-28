package physics

import (
	"math"
	"slices"
)

type shapeKind uint8

const (
	shapeCircle shapeKind = iota
	shapeBox
	shapeSegment
)

type bodyState struct {
	generation      uint32
	alive           bool
	bodyType        BodyType
	position        Vector
	rotation        float64
	velocity        Vector
	angularVelocity float64
	force           Vector
	torque          float64
	mass            float64
	inertia         float64
	inverseMass     float64
	inverseInertia  float64
	gravityScale    float64
	linearDamping   float64
	angularDamping  float64
	fixedRotation   bool
	userData        uint64
	managed         bool
	touched         uint64
	sleeping        bool
	quietTime       float64
	wakeContacts    bool
}

func (body *bodyState) updateInverseMass() {
	if body.fixedRotation {
		body.angularVelocity = 0
	}
	if body.bodyType != BodyDynamic {
		body.inverseMass, body.inverseInertia = 0, 0
		return
	}
	body.mass = max(body.mass, 1e-9)
	body.inverseMass = 1 / body.mass
	if body.fixedRotation || body.inertia <= 1e-9 || math.IsInf(body.inertia, 1) {
		body.inverseInertia = 0
	} else {
		body.inverseInertia = 1 / body.inertia
	}
}

type colliderState struct {
	generation    uint32
	alive         bool
	bodyIndex     uint32
	kind          shapeKind
	a             Vector
	b             Vector
	radius        float64
	rotation      float64
	friction      float64
	restitution   float64
	sensor        bool
	filter        Filter
	collisionType uint32
	managed       bool
	touched       uint64
}

type colliderPair struct {
	a int
	b int
}

type aabb struct {
	min Vector
	max Vector
}

type manifold struct {
	pair        colliderPair
	normal      Vector
	point       Vector
	penetration float64
	friction    float64
	restitution float64
	sensor      bool
	points      [2]contactPoint
	pointCount  int
	generations [2]uint32
}

type contactPoint struct {
	position       Vector
	penetration    float64
	normalImpulse  float64
	tangentImpulse float64
	bounceVelocity float64
}

type activeCollision struct {
	key         uint64
	colliderA   ColliderID
	colliderB   ColliderID
	bodyA       BodyID
	bodyB       BodyID
	userDataA   uint64
	userDataB   uint64
	typeA       uint32
	typeB       uint32
	normal      Vector
	point       Vector
	penetration float64
}

type World struct {
	Gravity       Vector
	Iterations    int
	Damping       float64
	CollisionSlop float64
	CellSize      float64
	Sleep         SleepConfig
	sleep         sleepState

	bodies        []bodyState
	freeBodies    []uint32
	colliders     []colliderState
	freeColliders []uint32

	aabbs            []aabb
	proxies          []broadphaseProxy
	staticGrid       spatialGrid
	dynamicGrid      spatialGrid
	gridCellSize     float64
	stats            StepStatistics
	large            []int
	staticSensors    []int
	pairs            []colliderPair
	contacts         []manifold
	velocityContacts []manifold
	contactDelta     float64
	active           []activeCollision
	nextActive       []activeCollision
	events           []CollisionEvent
}

func NewWorld() *World {
	return &World{Gravity: Vector{Y: -9.81}, Iterations: 8, Damping: 1, CollisionSlop: .01, CellSize: 64}
}

// Reserve preserves existing bodies and contacts while reserving pools and
// estimated scratch capacity. Dense contacts or small cells can require more
// scratch; once warmed up, Step reuses that storage without allocations.
func (world *World) Reserve(bodyCount, colliderCount int) {
	if world == nil {
		return
	}
	world.bodies = growCapacity(world.bodies, bodyCount)
	world.freeBodies = growCapacity(world.freeBodies, bodyCount)
	world.colliders = growCapacity(world.colliders, colliderCount)
	world.freeColliders = growCapacity(world.freeColliders, colliderCount)
	world.aabbs = growCapacity(world.aabbs, colliderCount)
	world.proxies = growCapacity(world.proxies, colliderCount)
	world.large = growCapacity(world.large, max(colliderCount/8, 8))
	world.pairs = growCapacity(world.pairs, colliderCount*8)
	world.contacts = growCapacity(world.contacts, colliderCount*4)
	world.velocityContacts = growCapacity(world.velocityContacts, colliderCount*4)
	world.active = growCapacity(world.active, colliderCount*4)
	world.nextActive = growCapacity(world.nextActive, colliderCount*4)
	world.events = growCapacity(world.events, colliderCount*4)
	world.staticGrid.reserve(colliderCount)
	world.dynamicGrid.reserve(colliderCount)
}

func growCapacity[T any](values []T, capacity int) []T {
	if cap(values) >= capacity {
		return values
	}
	return slices.Grow(values, capacity-len(values))
}

func (world *World) CreateBody(def BodyDef) Body {
	if world == nil {
		return Body{}
	}
	if def.Mass <= 0 || math.IsNaN(def.Mass) || math.IsInf(def.Mass, 0) {
		def.Mass = 1
	}
	if !def.gravitySet && def.GravityScale == 0 {
		def.GravityScale = 1
	}
	var index uint32
	if count := len(world.freeBodies); count != 0 {
		index = world.freeBodies[count-1]
		world.freeBodies = world.freeBodies[:count-1]
	} else {
		index = uint32(len(world.bodies))
		world.bodies = append(world.bodies, bodyState{})
	}
	state := &world.bodies[index]
	generation := state.generation
	if generation == 0 {
		generation = 1
	}
	*state = bodyState{
		generation: generation, alive: true, bodyType: def.Type,
		position: def.Position, rotation: def.Rotation, velocity: def.Velocity,
		angularVelocity: def.AngularVelocity, mass: def.Mass, inertia: def.Inertia,
		gravityScale: def.GravityScale, linearDamping: max(def.LinearDamping, 0),
		angularDamping: max(def.AngularDamping, 0), fixedRotation: def.FixedRotation,
		userData: def.UserData,
	}
	state.updateInverseMass()
	return Body{world: world, id: makeBodyID(index, generation)}
}

func (world *World) RemoveBody(body Body) bool {
	if world == nil || body.world != world {
		return false
	}
	index, state := world.resolveBody(body.id)
	if state == nil {
		return false
	}
	for colliderIndex := range world.colliders {
		collider := &world.colliders[colliderIndex]
		if collider.alive && collider.bodyIndex == index {
			world.removeColliderIndex(uint32(colliderIndex))
		}
	}
	state.alive = false
	state.generation++
	if state.generation == 0 {
		state.generation = 1
	}
	world.freeBodies = append(world.freeBodies, index)
	return true
}

func AddCollider[S Shape2D](world *World, body Body, def ColliderDef[S]) ColliderHandle[S] {
	if world == nil || body.world != world {
		return ColliderHandle[S]{}
	}
	bodyIndex, bodyState := world.resolveBody(body.id)
	if bodyState == nil {
		return ColliderHandle[S]{}
	}
	var index uint32
	if count := len(world.freeColliders); count != 0 {
		index = world.freeColliders[count-1]
		world.freeColliders = world.freeColliders[:count-1]
	} else {
		index = uint32(len(world.colliders))
		world.colliders = append(world.colliders, colliderState{})
	}
	state := &world.colliders[index]
	generation := state.generation
	if generation == 0 {
		generation = 1
	}
	*state = colliderState{
		generation: generation, alive: true, bodyIndex: bodyIndex,
		friction: max(def.Friction, 0), restitution: clamp(def.Restitution, 0, 1),
		sensor: def.Sensor, filter: def.Filter.normalized(), collisionType: def.CollisionType,
	}
	setColliderShape(state, def.Shape)
	if bodyState.inertia <= 0 {
		bodyState.inertia = shapeInertia(*state, bodyState.mass)
		bodyState.updateInverseMass()
	}
	return ColliderHandle[S]{world: world, id: makeColliderID(index, generation)}
}

func RemoveCollider[S Shape2D](world *World, collider ColliderHandle[S]) bool {
	if world == nil || collider.world != world {
		return false
	}
	index, state := world.resolveCollider(collider.id)
	if state == nil {
		return false
	}
	world.removeColliderIndex(index)
	return true
}

func (world *World) removeColliderIndex(index uint32) {
	state := &world.colliders[index]
	body := &world.bodies[state.bodyIndex]
	Body{world: world, id: makeBodyID(state.bodyIndex, body.generation)}.Wake()
	state.alive = false
	state.generation++
	if state.generation == 0 {
		state.generation = 1
	}
	world.freeColliders = append(world.freeColliders, index)
}

func (world *World) Body(id BodyID) Body {
	if world == nil || world.bodyState(id) == nil {
		return Body{}
	}
	return Body{world: world, id: id}
}

func (world *World) Events() []CollisionEvent {
	if world == nil {
		return nil
	}
	return world.events
}

func (world *World) Step(delta float64) {
	if world == nil || delta <= 0 || math.IsNaN(delta) || math.IsInf(delta, 0) {
		return
	}
	world.stats = StepStatistics{}
	world.beginSleep()
	world.integrateVelocities(delta)
	world.buildContacts()
	world.solveContacts(delta)
	world.finishSleep(delta)
	world.buildEvents()
}

func (world *World) integrateVelocities(delta float64) {
	worldDamping := world.Damping
	if worldDamping <= 0 || worldDamping > 1 {
		worldDamping = 1
	}
	stepDamping := math.Pow(worldDamping, delta)
	for index := range world.bodies {
		body := &world.bodies[index]
		if !body.alive || body.sleeping {
			continue
		}
		switch body.bodyType {
		case BodyDynamic:
			acceleration := world.Gravity.Mul(body.gravityScale).Add(body.force.Mul(body.inverseMass))
			body.velocity = body.velocity.Add(acceleration.Mul(delta))
			body.angularVelocity += body.torque * body.inverseInertia * delta
			linearDamping := stepDamping / (1 + body.linearDamping*delta)
			angularDamping := stepDamping / (1 + body.angularDamping*delta)
			body.velocity = body.velocity.Mul(linearDamping)
			body.angularVelocity *= angularDamping
			body.force, body.torque = Vector{}, 0
		}
	}
}

func (world *World) integratePositions(delta float64) {
	for index := range world.bodies {
		body := &world.bodies[index]
		if !body.alive || body.sleeping || body.bodyType == BodyStatic {
			continue
		}
		body.position = body.position.Add(body.velocity.Mul(delta))
		if !body.fixedRotation {
			body.rotation += body.angularVelocity * delta
		}
	}
}

func (world *World) solveContacts(delta float64) {
	iterations := max(world.Iterations, 1)
	world.solveVelocities(delta, true)
	// Advance with constrained velocities so static friction also prevents
	// gravity from accumulating tangential displacement on resting slopes.
	world.integratePositions(delta)
	world.buildContacts()
	// Resolve new impacts before position correction changes their lever arms.
	world.solveVelocities(delta, false)
	for iteration := range iterations {
		// Position corrections can introduce a contact that was absent from the
		// initial broadphase (for example, a dense stack pushing into a wall).
		// Rebuild a bounded number of times so those constraints join this step.
		if iteration != 0 {
			world.buildContacts()
		}
		corrected := false
		for index := len(world.contacts) - 1; index >= 0; index-- {
			contact := &world.contacts[index]
			if !contact.sensor && world.contactAwake(contact) {
				corrected = world.solvePosition(contact) || corrected
			}
		}
		if !corrected {
			return
		}
	}
	// Finish with immovable constraints so a dense dynamic stack cannot leave
	// its last correction on the far side of a wall until the next tick.
	world.buildContacts()
	for index := len(world.contacts) - 1; index >= 0; index-- {
		contact := &world.contacts[index]
		if contact.sensor || !world.contactAwake(contact) {
			continue
		}
		bodyA := &world.bodies[world.colliders[contact.pair.a].bodyIndex]
		bodyB := &world.bodies[world.colliders[contact.pair.b].bodyIndex]
		if bodyA.bodyType != BodyDynamic || bodyB.bodyType != BodyDynamic {
			world.solvePosition(contact)
		}
	}
}

func (world *World) solveVelocities(delta float64, warmStart bool) {
	for index := range world.contacts {
		contact := &world.contacts[index]
		if contact.sensor || !world.contactAwake(contact) {
			continue
		}
		bodyA := &world.bodies[world.colliders[contact.pair.a].bodyIndex]
		bodyB := &world.bodies[world.colliders[contact.pair.b].bodyIndex]
		// Resting contacts must absorb the velocity gravity adds each tick.
		threshold := max(1, 2*world.Gravity.Length()*delta*max(math.Abs(bodyA.gravityScale), math.Abs(bodyB.gravityScale)))
		for pointIndex := range contact.pointCount {
			point := &contact.points[pointIndex]
			relative := bodyB.velocityAt(point.position.Sub(bodyB.position)).Sub(bodyA.velocityAt(point.position.Sub(bodyA.position)))
			if speed := relative.Dot(contact.normal); speed < -threshold {
				point.bounceVelocity = -contact.restitution * speed
			}
		}
	}
	world.restoreContactImpulses(delta, warmStart)
	for range max(world.Iterations, 1) {
		for index := range world.contacts {
			contact := &world.contacts[index]
			if contact.sensor || !world.contactAwake(contact) {
				continue
			}
			world.solveVelocity(contact)
		}
	}
	world.velocityContacts = append(world.velocityContacts[:0], world.contacts...)
	world.contactDelta = delta
}

func (world *World) restoreContactImpulses(delta float64, warmStart bool) {
	previousIndex := 0
	for index := range world.contacts {
		contact := &world.contacts[index]
		key := pairKey(contact.pair.a, contact.pair.b)
		for previousIndex < len(world.velocityContacts) && pairKey(world.velocityContacts[previousIndex].pair.a, world.velocityContacts[previousIndex].pair.b) < key {
			previousIndex++
		}
		if previousIndex == len(world.velocityContacts) {
			break
		}
		previous := &world.velocityContacts[previousIndex]
		if contact.sensor || !world.contactAwake(contact) || previous.sensor || contact.pair != previous.pair || contact.generations != previous.generations ||
			contact.pointCount != previous.pointCount || contact.normal.Dot(previous.normal) < .99 {
			continue
		}
		bodyA := &world.bodies[world.colliders[contact.pair.a].bodyIndex]
		bodyB := &world.bodies[world.colliders[contact.pair.b].bodyIndex]
		scale := 1.0
		if warmStart && world.contactDelta > 0 {
			scale = delta / world.contactDelta
		}
		reverse := contact.pointCount == 2 && contact.points[0].position.Sub(previous.points[1].position).LengthSquared() < contact.points[0].position.Sub(previous.points[0].position).LengthSquared()
		for pointIndex := range contact.pointCount {
			oldIndex := pointIndex
			if reverse {
				oldIndex = 1 - pointIndex
			}
			point, old := &contact.points[pointIndex], &previous.points[oldIndex]
			point.normalImpulse, point.tangentImpulse = scale*old.normalImpulse, scale*old.tangentImpulse
			if warmStart {
				impulse := contact.normal.Mul(point.normalImpulse).Add(contact.normal.Perpendicular().Mul(point.tangentImpulse))
				applyPairImpulse(bodyA, bodyB, point.position.Sub(bodyA.position), point.position.Sub(bodyB.position), impulse)
			} else {
				point.bounceVelocity = max(point.bounceVelocity, old.bounceVelocity)
			}
		}
	}
}

func (world *World) solveVelocity(contact *manifold) {
	bodyA := &world.bodies[world.colliders[contact.pair.a].bodyIndex]
	bodyB := &world.bodies[world.colliders[contact.pair.b].bodyIndex]
	tangent := contact.normal.Perpendicular()
	for index := range contact.pointCount {
		point := &contact.points[index]
		ra, rb := point.position.Sub(bodyA.position), point.position.Sub(bodyB.position)
		mass := contactInverseMass(bodyA, bodyB, ra, rb, tangent)
		if mass <= 1e-12 {
			continue
		}
		speed := bodyB.velocityAt(rb).Sub(bodyA.velocityAt(ra)).Dot(tangent)
		limit := contact.friction * point.normalImpulse
		impulse := clamp(point.tangentImpulse-speed/mass, -limit, limit)
		applyPairImpulse(bodyA, bodyB, ra, rb, tangent.Mul(impulse-point.tangentImpulse))
		point.tangentImpulse = impulse
	}
	// Finish with support constraints so friction cannot leave residual rocking.
	if contact.pointCount == 2 && solveNormalPair(bodyA, bodyB, contact) {
		return
	}
	for index := range contact.pointCount {
		point := &contact.points[index]
		ra, rb := point.position.Sub(bodyA.position), point.position.Sub(bodyB.position)
		mass := contactInverseMass(bodyA, bodyB, ra, rb, contact.normal)
		if mass > 1e-12 {
			speed := bodyB.velocityAt(rb).Sub(bodyA.velocityAt(ra)).Dot(contact.normal)
			impulse := max(point.normalImpulse+(point.bounceVelocity-speed)/mass, 0)
			applyPairImpulse(bodyA, bodyB, ra, rb, contact.normal.Mul(impulse-point.normalImpulse))
			point.normalImpulse = impulse
		}
	}
}

func contactInverseMass(a, b *bodyState, ra, rb, axis Vector) float64 {
	ca, cb := cross(ra, axis), cross(rb, axis)
	return a.inverseMass + b.inverseMass + ca*ca*a.inverseInertia + cb*cb*b.inverseInertia
}

// Solve both normal constraints together so a face supports the body without
// alternately applying torque at its two ends. Impulses may push, never pull.
func solveNormalPair(a, b *bodyState, contact *manifold) bool {
	p, q := &contact.points[0], &contact.points[1]
	ra, rb := p.position.Sub(a.position), p.position.Sub(b.position)
	sa, sb := q.position.Sub(a.position), q.position.Sub(b.position)
	n := contact.normal
	k11 := contactInverseMass(a, b, ra, rb, n)
	k22 := contactInverseMass(a, b, sa, sb, n)
	k12 := a.inverseMass + b.inverseMass + cross(ra, n)*cross(sa, n)*a.inverseInertia + cross(rb, n)*cross(sb, n)*b.inverseInertia
	b1 := p.bounceVelocity - b.velocityAt(rb).Sub(a.velocityAt(ra)).Dot(n) + k11*p.normalImpulse + k12*q.normalImpulse
	b2 := q.bounceVelocity - b.velocityAt(sb).Sub(a.velocityAt(sa)).Dot(n) + k12*p.normalImpulse + k22*q.normalImpulse
	impulses, ok := solveContactPair(k11, k12, k22, b1, b2)
	if !ok {
		return false
	}
	applyPairImpulse(a, b, ra, rb, n.Mul(impulses.X-p.normalImpulse))
	applyPairImpulse(a, b, sa, sb, n.Mul(impulses.Y-q.normalImpulse))
	p.normalImpulse, q.normalImpulse = impulses.X, impulses.Y
	return true
}

func solveContactPair(k11, k12, k22, b1, b2 float64) (Vector, bool) {
	determinant := k11*k22 - k12*k12
	if determinant <= 1e-6*k11*k22 || k11 <= 1e-12 || k22 <= 1e-12 {
		return Vector{}, false
	}
	x, y := (k22*b1-k12*b2)/determinant, (k11*b2-k12*b1)/determinant
	switch {
	case x >= 0 && y >= 0:
	case b2 >= 0 && k12*(b2/k22) >= b1:
		x, y = 0, b2/k22
	case b1 >= 0 && k12*(b1/k11) >= b2:
		x, y = b1/k11, 0
	case b1 <= 0 && b2 <= 0:
		x, y = 0, 0
	default:
		return Vector{}, false
	}
	return V(x, y), true
}

func applyPairImpulse(bodyA, bodyB *bodyState, ra, rb, impulse Vector) {
	bodyA.velocity = bodyA.velocity.Sub(impulse.Mul(bodyA.inverseMass))
	bodyA.angularVelocity -= cross(ra, impulse) * bodyA.inverseInertia
	bodyB.velocity = bodyB.velocity.Add(impulse.Mul(bodyB.inverseMass))
	bodyB.angularVelocity += cross(rb, impulse) * bodyB.inverseInertia
}

func (body *bodyState) velocityAt(offset Vector) Vector {
	if body.bodyType == BodyStatic {
		return Vector{}
	}
	angularVelocity := body.angularVelocity
	if body.fixedRotation {
		angularVelocity = 0
	}
	return body.velocity.Add(crossScalarVector(angularVelocity, offset))
}

func (world *World) solvePosition(contact *manifold) bool {
	colliderA := &world.colliders[contact.pair.a]
	colliderB := &world.colliders[contact.pair.b]
	bodyA := &world.bodies[colliderA.bodyIndex]
	bodyB := &world.bodies[colliderB.bodyIndex]
	current, ok := collideShapes(contact.pair, *colliderA, *bodyA, *colliderB, *bodyB)
	if !ok {
		return false
	}
	slop := max(world.CollisionSlop, 0)
	depth := current.penetration - slop
	if depth <= 1e-9 {
		return false
	}
	var ra, rb [2]Vector
	var mass [2]float64
	for index := range current.pointCount {
		ra[index] = current.points[index].position.Sub(bodyA.position)
		rb[index] = current.points[index].position.Sub(bodyB.position)
		mass[index] = contactInverseMass(bodyA, bodyB, ra[index], rb[index], current.normal)
	}
	var impulses Vector
	if current.pointCount == 2 {
		n := current.normal
		coupling := bodyA.inverseMass + bodyB.inverseMass + cross(ra[0], n)*cross(ra[1], n)*bodyA.inverseInertia + cross(rb[0], n)*cross(rb[1], n)*bodyB.inverseInertia
		impulses, ok = solveContactPair(mass[0], coupling, mass[1], current.points[0].penetration-slop+1e-6, current.points[1].penetration-slop+1e-6)
	} else {
		ok = false
	}
	if !ok {
		index := 0
		if current.pointCount == 2 && current.points[1].penetration > current.points[0].penetration {
			index = 1
		}
		if mass[index] <= 1e-12 {
			return false
		}
		if index == 0 {
			impulses.X = (depth + 1e-6) / mass[index]
		} else {
			impulses.Y = (depth + 1e-6) / mass[index]
		}
	}
	correction := current.normal.Mul(impulses.X + impulses.Y)
	angleA := (cross(ra[0], current.normal)*impulses.X + cross(ra[1], current.normal)*impulses.Y) * bodyA.inverseInertia
	angleB := (cross(rb[0], current.normal)*impulses.X + cross(rb[1], current.normal)*impulses.Y) * bodyB.inverseInertia
	// Limit angular corrections while the linearized contact geometry is valid.
	scale := 1.0
	if angle := max(math.Abs(angleA), math.Abs(angleB)); angle > .2 {
		scale = .2 / angle
	}
	bodyA.position = bodyA.position.Sub(correction.Mul(scale * bodyA.inverseMass))
	bodyB.position = bodyB.position.Add(correction.Mul(scale * bodyB.inverseMass))
	bodyA.rotation -= scale * angleA
	bodyB.rotation += scale * angleB
	return true
}

func (world *World) buildEvents() {
	world.events = world.events[:0]
	world.nextActive = world.nextActive[:0]
	for index := range world.contacts {
		contact := world.contacts[index]
		colliderA := &world.colliders[contact.pair.a]
		colliderB := &world.colliders[contact.pair.b]
		bodyA := &world.bodies[colliderA.bodyIndex]
		bodyB := &world.bodies[colliderB.bodyIndex]
		active := activeCollision{
			key:       pairKey(contact.pair.a, contact.pair.b),
			colliderA: makeColliderID(uint32(contact.pair.a), colliderA.generation),
			colliderB: makeColliderID(uint32(contact.pair.b), colliderB.generation),
			bodyA:     makeBodyID(colliderA.bodyIndex, bodyA.generation), bodyB: makeBodyID(colliderB.bodyIndex, bodyB.generation),
			userDataA: bodyA.userData, userDataB: bodyB.userData,
			typeA: colliderA.collisionType, typeB: colliderB.collisionType,
			normal: contact.normal, point: contact.point, penetration: contact.penetration,
		}
		world.nextActive = append(world.nextActive, active)
	}
	// Contacts inherit the broadphase's unique, sorted pair order.
	oldIndex, nextIndex := 0, 0
	for oldIndex < len(world.active) || nextIndex < len(world.nextActive) {
		switch {
		case nextIndex >= len(world.nextActive) || oldIndex < len(world.active) && world.active[oldIndex].key < world.nextActive[nextIndex].key:
			world.appendEvent(CollisionEnd, world.active[oldIndex])
			oldIndex++
		case oldIndex >= len(world.active) || world.nextActive[nextIndex].key < world.active[oldIndex].key:
			world.appendEvent(CollisionBegin, world.nextActive[nextIndex])
			nextIndex++
		default:
			previous, next := world.active[oldIndex], world.nextActive[nextIndex]
			if previous.colliderA != next.colliderA || previous.colliderB != next.colliderB {
				world.appendEvent(CollisionEnd, previous)
				world.appendEvent(CollisionBegin, next)
			} else {
				world.appendEvent(CollisionStay, next)
			}
			oldIndex++
			nextIndex++
		}
	}
	world.active, world.nextActive = world.nextActive, world.active[:0]
}

func (world *World) appendEvent(phase CollisionPhase, active activeCollision) {
	world.events = append(world.events, CollisionEvent{
		Phase: phase, BodyA: active.bodyA, BodyB: active.bodyB,
		ColliderA: active.colliderA, ColliderB: active.colliderB,
		CollisionTypeA: active.typeA, CollisionTypeB: active.typeB,
		UserDataA: active.userDataA, UserDataB: active.userDataB,
		Normal: active.normal, Point: active.point, Penetration: active.penetration,
	})
}

func (world *World) bodyState(id BodyID) *bodyState {
	_, state := world.resolveBody(id)
	return state
}

func (world *World) resolveBody(id BodyID) (uint32, *bodyState) {
	index, generation := splitID(uint64(id))
	if int(index) >= len(world.bodies) {
		return 0, nil
	}
	state := &world.bodies[index]
	if !state.alive || state.generation != generation {
		return 0, nil
	}
	return index, state
}

func (world *World) colliderState(id ColliderID) *colliderState {
	_, state := world.resolveCollider(id)
	return state
}

func (world *World) resolveCollider(id ColliderID) (uint32, *colliderState) {
	index, generation := splitID(uint64(id))
	if int(index) >= len(world.colliders) {
		return 0, nil
	}
	state := &world.colliders[index]
	if !state.alive || state.generation != generation {
		return 0, nil
	}
	return index, state
}

func makeBodyID(index, generation uint32) BodyID {
	return BodyID(uint64(generation)<<32 | uint64(index+1))
}

func makeColliderID(index, generation uint32) ColliderID {
	return ColliderID(uint64(generation)<<32 | uint64(index+1))
}

func splitID(value uint64) (uint32, uint32) {
	low := uint32(value)
	if low == 0 {
		return math.MaxUint32, 0
	}
	return low - 1, uint32(value >> 32)
}

func pairKey(a, b int) uint64 {
	if a > b {
		a, b = b, a
	}
	return uint64(uint32(a))<<32 | uint64(uint32(b))
}

func setColliderShape[S Shape2D](state *colliderState, shape S) {
	switch value := any(shape).(type) {
	case Circle:
		state.kind, state.a, state.radius = shapeCircle, value.Center, max(value.Radius, 0)
		state.b, state.rotation = Vector{}, 0
	case Box:
		state.kind, state.a = shapeBox, value.Center
		state.b = Vector{X: max(value.HalfExtents.X, 0), Y: max(value.HalfExtents.Y, 0)}
		state.radius, state.rotation = 0, value.Rotation
	case Segment:
		state.kind, state.a, state.b = shapeSegment, value.Start, value.End
		state.radius, state.rotation = max(value.Radius, 0), 0
	default:
		panic("physics: unsupported collider shape")
	}
}

func shapeFromState[S Shape2D](state colliderState) S {
	var value any
	switch state.kind {
	case shapeCircle:
		value = Circle{Center: state.a, Radius: state.radius}
	case shapeBox:
		value = Box{Center: state.a, HalfExtents: state.b, Rotation: state.rotation}
	case shapeSegment:
		value = Segment{Start: state.a, End: state.b, Radius: state.radius}
	}
	return value.(S)
}

func shapeInertia(shape colliderState, mass float64) float64 {
	switch shape.kind {
	case shapeCircle:
		return mass * (shape.radius*shape.radius*.5 + shape.a.LengthSquared())
	case shapeBox:
		width, height := shape.b.X*2, shape.b.Y*2
		return mass * ((width*width+height*height)/12 + shape.a.LengthSquared())
	case shapeSegment:
		lengthSquared := shape.b.Sub(shape.a).LengthSquared()
		center := shape.a.Add(shape.b).Mul(.5)
		return mass * (lengthSquared/12 + center.LengthSquared())
	default:
		return math.Inf(1)
	}
}
