package physics

type BodyID3D uint64
type ColliderID3D uint64

type BodyDef3D struct {
	Type            BodyType
	Position        Vector3D
	Rotation        Quaternion
	Velocity        Vector3D
	AngularVelocity Vector3D
	Mass            float64
	Inertia         float64
	GravityScale    float64
	LinearDamping   float64
	AngularDamping  float64
	FixedRotation   bool
	UserData        uint64
	gravitySet      bool
}

func NewBodyDef3D(bodyType BodyType) BodyDef3D {
	return BodyDef3D{
		Type: bodyType, Rotation: QuaternionIdentity(), Mass: 1,
		GravityScale: 1, gravitySet: true,
	}
}

// Shape3D is closed so generic colliders lower to an allocation-free internal value.
type Shape3D interface {
	Shape
	physicsShape3D()
}

type Sphere struct {
	Center Vector3D
	Radius float64
}

func (Sphere) physicsShape()   {}
func (Sphere) physicsShape3D() {}

type Box3D struct {
	Center      Vector3D
	HalfExtents Vector3D
	Rotation    Quaternion
}

func (Box3D) physicsShape()   {}
func (Box3D) physicsShape3D() {}

func NewBox3D(width, height, depth float64) Box3D {
	return Box3D{HalfExtents: V3(width*.5, height*.5, depth*.5), Rotation: QuaternionIdentity()}
}

// Capsule is aligned to its local Y axis. HalfHeight excludes the round caps.
type Capsule struct {
	Center     Vector3D
	Rotation   Quaternion
	HalfHeight float64
	Radius     float64
}

func (Capsule) physicsShape()   {}
func (Capsule) physicsShape3D() {}

func NewCapsule(height, radius float64) Capsule {
	return Capsule{HalfHeight: max(height*.5-radius, 0), Radius: radius, Rotation: QuaternionIdentity()}
}

type CollisionEvent3D struct {
	Phase          CollisionPhase
	BodyA          BodyID3D
	BodyB          BodyID3D
	ColliderA      ColliderID3D
	ColliderB      ColliderID3D
	CollisionTypeA uint32
	CollisionTypeB uint32
	UserDataA      uint64
	UserDataB      uint64
	Normal         Vector3D
	Point          Vector3D
	Penetration    float64
}

type Body3D struct {
	world *World3D
	id    BodyID3D
}

func (body Body3D) ID() BodyID3D { return body.id }

func (body Body3D) Valid() bool {
	return body.world != nil && body.world.bodyState3D(body.id) != nil
}

func (body Body3D) Type() BodyType {
	if state := body.state(); state != nil {
		return state.bodyType
	}
	return BodyStatic
}

func (body Body3D) SetType(value BodyType) {
	if state := body.state(); state != nil {
		state.bodyType = value
		state.updateInverseMass()
	}
}

func (body Body3D) Position() Vector3D {
	if state := body.state(); state != nil {
		return state.position
	}
	return Vector3D{}
}

func (body Body3D) SetPosition(value Vector3D) {
	if state := body.state(); state != nil {
		state.position = value
	}
}

func (body Body3D) Rotation() Quaternion {
	if state := body.state(); state != nil {
		return state.rotation
	}
	return QuaternionIdentity()
}

func (body Body3D) SetRotation(value Quaternion) {
	if state := body.state(); state != nil {
		state.rotation = value.Normalize()
	}
}

func (body Body3D) Velocity() Vector3D {
	if state := body.state(); state != nil {
		return state.velocity
	}
	return Vector3D{}
}

func (body Body3D) SetVelocity(value Vector3D) {
	if state := body.state(); state != nil {
		state.velocity = value
	}
}

func (body Body3D) AngularVelocity() Vector3D {
	if state := body.state(); state != nil {
		return state.angularVelocity
	}
	return Vector3D{}
}

func (body Body3D) SetAngularVelocity(value Vector3D) {
	if state := body.state(); state != nil {
		if state.fixedRotation {
			value = Vector3D{}
		}
		state.angularVelocity = value
	}
}

func (body Body3D) ApplyForce(force Vector3D) {
	if state := body.state(); state != nil && state.bodyType == BodyDynamic {
		state.force = state.force.Add(force)
	}
}

func (body Body3D) ApplyTorque(torque Vector3D) {
	if state := body.state(); state != nil && state.bodyType == BodyDynamic {
		state.torque = state.torque.Add(torque)
	}
}

func (body Body3D) ApplyImpulse(impulse, worldPoint Vector3D) {
	state := body.state()
	if state == nil || state.bodyType != BodyDynamic {
		return
	}
	state.velocity = state.velocity.Add(impulse.Mul(state.inverseMass))
	state.angularVelocity = state.angularVelocity.Add(
		worldPoint.Sub(state.position).Cross(impulse).Mul(state.inverseInertia),
	)
}

func (body Body3D) ClearForces() {
	if state := body.state(); state != nil {
		state.force, state.torque = Vector3D{}, Vector3D{}
	}
}

func (body Body3D) SetGravityScale(value float64) {
	if state := body.state(); state != nil {
		state.gravityScale = value
	}
}

func (body Body3D) UserData() uint64 {
	if state := body.state(); state != nil {
		return state.userData
	}
	return 0
}

func (body Body3D) state() *bodyState3D {
	if body.world == nil {
		return nil
	}
	return body.world.bodyState3D(body.id)
}

type ColliderHandle3D[S Shape3D] struct {
	world *World3D
	id    ColliderID3D
}

func (collider ColliderHandle3D[S]) ID() ColliderID3D { return collider.id }

func (collider ColliderHandle3D[S]) Valid() bool {
	return collider.world != nil && collider.world.colliderState3D(collider.id) != nil
}

func (collider ColliderHandle3D[S]) Shape() S {
	var zero S
	if collider.world == nil {
		return zero
	}
	state := collider.world.colliderState3D(collider.id)
	if state == nil {
		return zero
	}
	return (*state).shapeFromState3D[S]()
}

func (collider ColliderHandle3D[S]) SetShape(shape S) {
	if collider.world != nil {
		if state := collider.world.colliderState3D(collider.id); state != nil {
			state.setColliderShape3D(shape)
		}
	}
}

func (collider ColliderHandle3D[S]) Filter() Filter {
	if collider.world != nil {
		if state := collider.world.colliderState3D(collider.id); state != nil {
			return state.filter
		}
	}
	return Filter{}
}

func (collider ColliderHandle3D[S]) SetFilter(value Filter) {
	if collider.world != nil {
		if state := collider.world.colliderState3D(collider.id); state != nil {
			state.filter = value.normalized()
		}
	}
}

func (collider ColliderHandle3D[S]) Sensor() bool {
	if collider.world != nil {
		if state := collider.world.colliderState3D(collider.id); state != nil {
			return state.sensor
		}
	}
	return false
}

func (collider ColliderHandle3D[S]) SetSensor(value bool) {
	if collider.world != nil {
		if state := collider.world.colliderState3D(collider.id); state != nil {
			state.sensor = value
		}
	}
}

func (collider ColliderHandle3D[S]) CollisionType() uint32 {
	if collider.world != nil {
		if state := collider.world.colliderState3D(collider.id); state != nil {
			return state.collisionType
		}
	}
	return 0
}

func (collider ColliderHandle3D[S]) SetCollisionType(value uint32) {
	if collider.world != nil {
		if state := collider.world.colliderState3D(collider.id); state != nil {
			state.collisionType = value
		}
	}
}

func (collider ColliderHandle3D[S]) SetMaterial(friction, restitution float64) {
	if collider.world != nil {
		if state := collider.world.colliderState3D(collider.id); state != nil {
			state.friction = max(friction, 0)
			state.restitution = clamp(restitution, 0, 1)
		}
	}
}
