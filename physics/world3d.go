package physics

import (
	"math"
	"slices"
)

type shapeKind3D uint8

const (
	shapeSphere3D shapeKind3D = iota
	shapeBox3D
	shapeCapsule3D
	shapeTriangleMesh3D
	shapeConvexHull3D
	shapeCompound3D
)

type bodyState3D struct {
	generation      uint32
	alive           bool
	bodyType        BodyType
	position        Vector3D
	rotation        Quaternion
	velocity        Vector3D
	angularVelocity Vector3D
	force           Vector3D
	torque          Vector3D
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
}

func (body *bodyState3D) updateInverseMass() {
	if body.fixedRotation {
		body.angularVelocity = Vector3D{}
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

type colliderState3D struct {
	generation    uint32
	alive         bool
	bodyIndex     uint32
	kind          shapeKind3D
	center        Vector3D
	half          Vector3D
	rotation      Quaternion
	radius        float64
	halfHeight    float64
	mesh          TriangleMesh
	hull          ConvexHull
	compound      Compound
	friction      float64
	restitution   float64
	sensor        bool
	filter        Filter
	collisionType uint32
	managed       bool
	touched       uint64
}

type aabb3D struct {
	min Vector3D
	max Vector3D
}

type manifold3D struct {
	pair        colliderPair
	normal      Vector3D
	point       Vector3D
	penetration float64
	friction    float64
	restitution float64
	sensor      bool
	points      [4]contactPoint3D
	pointCount  int
	generations [2]uint32
}

type contactPoint3D struct {
	position       Vector3D
	penetration    float64
	normalImpulse  float64
	tangentImpulse Vector3D
	bounceVelocity float64
}

func (contact *manifold3D) initializePoints() {
	if contact.pointCount == 0 {
		contact.points[0] = contactPoint3D{position: contact.point, penetration: contact.penetration}
		contact.pointCount = 1
	}
}

type activeCollision3D struct {
	key         uint64
	colliderA   ColliderID3D
	colliderB   ColliderID3D
	bodyA       BodyID3D
	bodyB       BodyID3D
	userDataA   uint64
	userDataB   uint64
	typeA       uint32
	typeB       uint32
	normal      Vector3D
	point       Vector3D
	penetration float64
}

type broadphaseEntry3D struct {
	collider, next int
}

type World3D struct {
	Gravity       Vector3D
	Iterations    int
	Damping       float64
	CollisionSlop float64
	CellSize      float64

	bodies        []bodyState3D
	freeBodies    []uint32
	colliders     []colliderState3D
	freeColliders []uint32

	aabbs            []aabb3D
	bucketHeads      []int
	gridEntries      []broadphaseEntry3D
	large            []int
	pairs            []colliderPair
	contacts         []manifold3D
	velocityContacts []manifold3D
	contactDelta     float64
	active           []activeCollision3D
	nextActive       []activeCollision3D
	events           []CollisionEvent3D
}

func NewWorld3D() *World3D {
	return &World3D{Gravity: V3(0, -9.81, 0), Iterations: 8, Damping: 1, CollisionSlop: .005, CellSize: 4}
}

// Reserve moves pool and scratch growth to setup time.
func (world *World3D) Reserve(bodyCount, colliderCount int) {
	if world == nil {
		return
	}
	world.bodies = growCapacity(world.bodies, bodyCount)
	world.freeBodies = growCapacity(world.freeBodies, bodyCount)
	world.colliders = growCapacity(world.colliders, colliderCount)
	world.freeColliders = growCapacity(world.freeColliders, colliderCount)
	world.aabbs = growCapacity(world.aabbs, colliderCount)
	world.gridEntries = growCapacity(world.gridEntries, colliderCount*8)
	world.large = growCapacity(world.large, max(colliderCount/8, 8))
	world.pairs = growCapacity(world.pairs, colliderCount*8)
	world.contacts = growCapacity(world.contacts, colliderCount*4)
	world.velocityContacts = growCapacity(world.velocityContacts, colliderCount*4)
	world.active = growCapacity(world.active, colliderCount*4)
	world.nextActive = growCapacity(world.nextActive, colliderCount*4)
	world.events = growCapacity(world.events, colliderCount*4)
	world.ensureBuckets(colliderCount)
}

func (world *World3D) CreateBody(def BodyDef3D) Body3D {
	if world == nil {
		return Body3D{}
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
		world.bodies = append(world.bodies, bodyState3D{})
	}
	state := &world.bodies[index]
	generation := state.generation
	if generation == 0 {
		generation = 1
	}
	*state = bodyState3D{
		generation: generation, alive: true, bodyType: def.Type,
		position: def.Position, rotation: def.Rotation.Normalize(), velocity: def.Velocity,
		angularVelocity: def.AngularVelocity, mass: def.Mass, inertia: def.Inertia,
		gravityScale: def.GravityScale, linearDamping: max(def.LinearDamping, 0),
		angularDamping: max(def.AngularDamping, 0), fixedRotation: def.FixedRotation,
		userData: def.UserData,
	}
	state.updateInverseMass()
	return Body3D{world: world, id: makeBodyID3D(index, generation)}
}

func (world *World3D) RemoveBody(body Body3D) bool {
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

func (world *World3D) AddCollider[S Shape3D](body Body3D, def ColliderDef[S]) ColliderHandle3D[S] {
	if world == nil || body.world != world {
		return ColliderHandle3D[S]{}
	}
	bodyIndex, bodyState3D := world.resolveBody(body.id)
	if bodyState3D == nil {
		return ColliderHandle3D[S]{}
	}
	var index uint32
	if count := len(world.freeColliders); count != 0 {
		index = world.freeColliders[count-1]
		world.freeColliders = world.freeColliders[:count-1]
	} else {
		index = uint32(len(world.colliders))
		world.colliders = append(world.colliders, colliderState3D{})
	}
	state := &world.colliders[index]
	generation := state.generation
	if generation == 0 {
		generation = 1
	}
	*state = colliderState3D{
		generation: generation, alive: true, bodyIndex: bodyIndex,
		friction: max(def.Friction, 0), restitution: clamp(def.Restitution, 0, 1),
		sensor: def.Sensor, filter: def.Filter.normalized(), collisionType: def.CollisionType,
	}
	state.setColliderShape3D(def.Shape)
	if bodyState3D.inertia <= 0 {
		bodyState3D.inertia = shapeInertia3D(*state, bodyState3D.mass)
		bodyState3D.updateInverseMass()
	}
	return ColliderHandle3D[S]{world: world, id: makeColliderID3D(index, generation)}
}

func (world *World3D) RemoveCollider[S Shape3D](collider ColliderHandle3D[S]) bool {
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

func (world *World3D) removeColliderIndex(index uint32) {
	state := &world.colliders[index]
	state.alive = false
	state.generation++
	if state.generation == 0 {
		state.generation = 1
	}
	world.freeColliders = append(world.freeColliders, index)
}

func (world *World3D) Body3D(id BodyID3D) Body3D {
	if world == nil || world.bodyState3D(id) == nil {
		return Body3D{}
	}
	return Body3D{world: world, id: id}
}

func (world *World3D) Events() []CollisionEvent3D {
	if world == nil {
		return nil
	}
	return world.events
}

func (world *World3D) Step(delta float64) {
	if world == nil || delta <= 0 || math.IsNaN(delta) || math.IsInf(delta, 0) {
		return
	}
	world.integrateVelocities(delta)
	world.buildContacts()
	world.solveContacts(delta)
	world.buildEvents()
}

func (world *World3D) integrateVelocities(delta float64) {
	worldDamping := world.Damping
	if worldDamping <= 0 || worldDamping > 1 {
		worldDamping = 1
	}
	stepDamping := math.Pow(worldDamping, delta)
	for index := range world.bodies {
		body := &world.bodies[index]
		if !body.alive {
			continue
		}
		switch body.bodyType {
		case BodyDynamic:
			acceleration := world.Gravity.Mul(body.gravityScale).Add(body.force.Mul(body.inverseMass))
			body.velocity = body.velocity.Add(acceleration.Mul(delta))
			body.angularVelocity = body.angularVelocity.Add(body.torque.Mul(body.inverseInertia * delta))
			body.velocity = body.velocity.Mul(stepDamping / (1 + body.linearDamping*delta))
			body.angularVelocity = body.angularVelocity.Mul(stepDamping / (1 + body.angularDamping*delta))
			body.force, body.torque = Vector3D{}, Vector3D{}
		}
	}
}

func (world *World3D) integratePositions(delta float64) {
	for index := range world.bodies {
		body := &world.bodies[index]
		if !body.alive || body.bodyType == BodyStatic {
			continue
		}
		body.position = body.position.Add(body.velocity.Mul(delta))
		if !body.fixedRotation {
			body.rotation = integrateRotation(body.rotation, body.angularVelocity, delta)
		}
	}
}

func (world *World3D) buildEvents() {
	world.events = world.events[:0]
	world.nextActive = world.nextActive[:0]
	for index := range world.contacts {
		contact := world.contacts[index]
		colliderA := &world.colliders[contact.pair.a]
		colliderB := &world.colliders[contact.pair.b]
		bodyA := &world.bodies[colliderA.bodyIndex]
		bodyB := &world.bodies[colliderB.bodyIndex]
		world.nextActive = append(world.nextActive, activeCollision3D{
			key:       pairKey(contact.pair.a, contact.pair.b),
			colliderA: makeColliderID3D(uint32(contact.pair.a), colliderA.generation),
			colliderB: makeColliderID3D(uint32(contact.pair.b), colliderB.generation),
			bodyA:     makeBodyID3D(colliderA.bodyIndex, bodyA.generation),
			bodyB:     makeBodyID3D(colliderB.bodyIndex, bodyB.generation),
			userDataA: bodyA.userData, userDataB: bodyB.userData,
			typeA: colliderA.collisionType, typeB: colliderB.collisionType,
			normal: contact.normal, point: contact.point, penetration: contact.penetration,
		})
	}
	slices.SortFunc(world.nextActive, func(a, b activeCollision3D) int {
		return int64Compare(a.key, b.key)
	})
	oldIndex, nextIndex := 0, 0
	for oldIndex < len(world.active) || nextIndex < len(world.nextActive) {
		switch {
		case nextIndex == len(world.nextActive) || oldIndex < len(world.active) && world.active[oldIndex].key < world.nextActive[nextIndex].key:
			world.appendEvent(CollisionEnd, world.active[oldIndex])
			oldIndex++
		case oldIndex == len(world.active) || world.nextActive[nextIndex].key < world.active[oldIndex].key:
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

func int64Compare(a, b uint64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func (world *World3D) appendEvent(phase CollisionPhase, active activeCollision3D) {
	world.events = append(world.events, CollisionEvent3D{
		Phase: phase, BodyA: active.bodyA, BodyB: active.bodyB,
		ColliderA: active.colliderA, ColliderB: active.colliderB,
		CollisionTypeA: active.typeA, CollisionTypeB: active.typeB,
		UserDataA: active.userDataA, UserDataB: active.userDataB,
		Normal: active.normal, Point: active.point, Penetration: active.penetration,
	})
}

func (world *World3D) bodyState3D(id BodyID3D) *bodyState3D {
	_, state := world.resolveBody(id)
	return state
}

func (world *World3D) resolveBody(id BodyID3D) (uint32, *bodyState3D) {
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

func (world *World3D) colliderState3D(id ColliderID3D) *colliderState3D {
	_, state := world.resolveCollider(id)
	return state
}

func (world *World3D) resolveCollider(id ColliderID3D) (uint32, *colliderState3D) {
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

func makeBodyID3D(index, generation uint32) BodyID3D {
	return BodyID3D(uint64(generation)<<32 | uint64(index+1))
}

func makeColliderID3D(index, generation uint32) ColliderID3D {
	return ColliderID3D(uint64(generation)<<32 | uint64(index+1))
}

func (state *colliderState3D) setColliderShape3D[S Shape3D](shape S) {
	state.mesh, state.hull, state.compound = TriangleMesh{}, ConvexHull{}, Compound{}
	switch value := any(shape).(type) {
	case Sphere:
		state.kind, state.center, state.radius = shapeSphere3D, value.Center, max(value.Radius, 0)
		state.half, state.rotation, state.halfHeight = Vector3D{}, QuaternionIdentity(), 0
	case Box3D:
		state.kind, state.center = shapeBox3D, value.Center
		state.half = V3(max(value.HalfExtents.X, 0), max(value.HalfExtents.Y, 0), max(value.HalfExtents.Z, 0))
		state.rotation, state.radius, state.halfHeight = value.Rotation.Normalize(), 0, 0
	case Capsule:
		state.kind, state.center = shapeCapsule3D, value.Center
		state.rotation = value.Rotation.Normalize()
		state.radius, state.halfHeight = max(value.Radius, 0), max(value.HalfHeight, 0)
		state.half = Vector3D{}
	case TriangleMesh:
		state.kind, state.center, state.rotation = shapeTriangleMesh3D, Vector3D{}, QuaternionIdentity()
		state.mesh = value
	case *TriangleMesh:
		if value == nil {
			panic("physics: nil triangle mesh")
		}
		state.kind, state.center, state.rotation = shapeTriangleMesh3D, Vector3D{}, QuaternionIdentity()
		state.mesh = *value
	case ConvexHull:
		state.kind, state.center, state.rotation = shapeConvexHull3D, Vector3D{}, QuaternionIdentity()
		state.hull = value
	case *ConvexHull:
		if value == nil {
			panic("physics: nil convex hull")
		}
		state.kind, state.center, state.rotation = shapeConvexHull3D, Vector3D{}, QuaternionIdentity()
		state.hull = *value
	case Compound:
		state.kind, state.center, state.rotation = shapeCompound3D, Vector3D{}, QuaternionIdentity()
		state.compound = value
	case *Compound:
		if value == nil {
			panic("physics: nil compound")
		}
		state.kind, state.center, state.rotation = shapeCompound3D, Vector3D{}, QuaternionIdentity()
		state.compound = *value
	default:
		panic("physics: unsupported 3D collider shape")
	}
}

func (state colliderState3D) shapeFromState3D[S Shape3D]() S {
	var zero S
	var value any
	switch state.kind {
	case shapeSphere3D:
		value = Sphere{Center: state.center, Radius: state.radius}
	case shapeBox3D:
		value = Box3D{Center: state.center, HalfExtents: state.half, Rotation: state.rotation}
	case shapeCapsule3D:
		value = Capsule{
			Center: state.center, Rotation: state.rotation,
			HalfHeight: state.halfHeight, Radius: state.radius,
		}
	case shapeTriangleMesh3D:
		value = state.mesh
		if _, pointer := any(zero).(*TriangleMesh); pointer {
			value = &state.mesh
		}
	case shapeConvexHull3D:
		value = state.hull
		if _, pointer := any(zero).(*ConvexHull); pointer {
			value = &state.hull
		}
	case shapeCompound3D:
		value = state.compound
		if _, pointer := any(zero).(*Compound); pointer {
			value = &state.compound
		}
	}
	return value.(S)
}

func shapeInertia3D(shape colliderState3D, mass float64) float64 {
	switch shape.kind {
	case shapeSphere3D:
		return mass * (.4*shape.radius*shape.radius + shape.center.LengthSquared())
	case shapeBox3D:
		width, height, depth := shape.half.X*2, shape.half.Y*2, shape.half.Z*2
		return mass * ((width*width+height*height+depth*depth)/18 + shape.center.LengthSquared())
	case shapeCapsule3D:
		height := shape.halfHeight*2 + shape.radius*2
		return mass * ((height*height+3*shape.radius*shape.radius)/12 + shape.center.LengthSquared())
	case shapeConvexHull3D:
		size := shape.hull.Bounds.Size()
		return mass * ((size.X*size.X+size.Y*size.Y+size.Z*size.Z)/18 + shape.center.LengthSquared())
	case shapeCompound3D:
		size := shape.compound.Bounds.Size()
		return mass * ((size.X*size.X+size.Y*size.Y+size.Z*size.Z)/18 + shape.center.LengthSquared())
	case shapeTriangleMesh3D:
		return math.Inf(1)
	default:
		return math.Inf(1)
	}
}
