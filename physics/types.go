package physics

import "math"

type BodyType uint8

const (
	BodyDynamic BodyType = iota
	BodyStatic
	BodyKinematic
)

type BodyID uint64
type ColliderID uint64

type BodyDef struct {
	Type            BodyType
	Position        Vector
	Rotation        float64
	Velocity        Vector
	AngularVelocity float64
	Mass            float64
	Inertia         float64
	GravityScale    float64
	LinearDamping   float64
	AngularDamping  float64
	FixedRotation   bool
	UserData        uint64
	gravitySet      bool
}

func NewBodyDef(bodyType BodyType) BodyDef {
	return BodyDef{Type: bodyType, Mass: 1, GravityScale: 1, gravitySet: true}
}

// Shape is closed to this package so every generic collider can be lowered to
// an allocation-free internal representation.
type Shape interface {
	physicsShape()
}

// Shape2D accepts shapes supported by the 2D solver.
type Shape2D interface {
	Shape
	physicsShape2D()
}

type Circle struct {
	Center Vector
	Radius float64
}

func (Circle) physicsShape()   {}
func (Circle) physicsShape2D() {}

type Box struct {
	Center      Vector
	HalfExtents Vector
	Rotation    float64
}

func (Box) physicsShape()   {}
func (Box) physicsShape2D() {}

func NewBox(width, height float64) Box {
	return Box{HalfExtents: Vector{X: width * .5, Y: height * .5}}
}

type Segment struct {
	Start  Vector
	End    Vector
	Radius float64
}

func (Segment) physicsShape()   {}
func (Segment) physicsShape2D() {}

type Filter struct {
	Group      int32
	Categories uint64
	Mask       uint64
}

var FilterAll = Filter{Categories: math.MaxUint64, Mask: math.MaxUint64}

func NewFilter(group int32, categories, mask uint64) Filter {
	return Filter{Group: group, Categories: categories, Mask: mask}
}

func (filter Filter) Reject(other Filter) bool {
	return filter.Group != 0 && filter.Group == other.Group ||
		filter.Categories&other.Mask == 0 || other.Categories&filter.Mask == 0
}

func (filter Filter) normalized() Filter {
	if filter == (Filter{}) {
		return FilterAll
	}
	return filter
}

type ColliderDef[S Shape] struct {
	Shape         S
	Friction      float64
	Restitution   float64
	Sensor        bool
	Filter        Filter
	CollisionType uint32
}

func NewColliderDef[S Shape](shape S) ColliderDef[S] {
	return ColliderDef[S]{Shape: shape, Friction: .5, Filter: FilterAll}
}

type CollisionPhase uint8

const (
	CollisionBegin CollisionPhase = iota
	CollisionStay
	CollisionEnd
)

type CollisionEvent struct {
	Phase          CollisionPhase
	BodyA          BodyID
	BodyB          BodyID
	ColliderA      ColliderID
	ColliderB      ColliderID
	CollisionTypeA uint32
	CollisionTypeB uint32
	UserDataA      uint64
	UserDataB      uint64
	Normal         Vector
	Point          Vector
	Penetration    float64
}

type Body struct {
	world *World
	id    BodyID
}

func (body Body) ID() BodyID { return body.id }
func (body Body) Valid() bool {
	return body.world != nil && body.world.bodyState(body.id) != nil
}

func (body Body) Type() BodyType {
	if state := body.state(); state != nil {
		return state.bodyType
	}
	return BodyStatic
}

func (body Body) SetType(value BodyType) {
	if state := body.state(); state != nil {
		state.bodyType = value
		state.updateInverseMass()
	}
}

func (body Body) Position() Vector {
	if state := body.state(); state != nil {
		return state.position
	}
	return Vector{}
}

func (body Body) SetPosition(value Vector) {
	if state := body.state(); state != nil {
		state.position = value
	}
}

func (body Body) Rotation() float64 {
	if state := body.state(); state != nil {
		return state.rotation
	}
	return 0
}

func (body Body) SetRotation(value float64) {
	if state := body.state(); state != nil {
		state.rotation = value
	}
}

func (body Body) Velocity() Vector {
	if state := body.state(); state != nil {
		return state.velocity
	}
	return Vector{}
}

func (body Body) SetVelocity(value Vector) {
	if state := body.state(); state != nil {
		state.velocity = value
	}
}

func (body Body) AngularVelocity() float64 {
	if state := body.state(); state != nil {
		return state.angularVelocity
	}
	return 0
}

func (body Body) SetAngularVelocity(value float64) {
	if state := body.state(); state != nil {
		if !state.fixedRotation {
			state.angularVelocity = value
		}
	}
}

func (body Body) ApplyForce(force Vector) {
	if state := body.state(); state != nil && state.bodyType == BodyDynamic {
		state.force = state.force.Add(force)
	}
}

func (body Body) ApplyTorque(torque float64) {
	if state := body.state(); state != nil && state.bodyType == BodyDynamic {
		state.torque += torque
	}
}

func (body Body) ClearForces() {
	if state := body.state(); state != nil {
		state.force, state.torque = Vector{}, 0
	}
}

// IsSleeping is reserved for a future opt-in sleep policy. Onix currently
// keeps active bodies awake so deterministic fixed-step games never stall.
func (body Body) IsSleeping() bool { return false }

func (body Body) ApplyImpulse(impulse, worldPoint Vector) {
	state := body.state()
	if state == nil || state.bodyType != BodyDynamic {
		return
	}
	state.velocity = state.velocity.Add(impulse.Mul(state.inverseMass))
	if state.inverseInertia != 0 {
		state.angularVelocity += cross(worldPoint.Sub(state.position), impulse) * state.inverseInertia
	}
}

func (body Body) SetGravityScale(value float64) {
	if state := body.state(); state != nil {
		state.gravityScale = value
	}
}

func (body Body) UserData() uint64 {
	if state := body.state(); state != nil {
		return state.userData
	}
	return 0
}

func (body Body) state() *bodyState {
	if body.world == nil {
		return nil
	}
	return body.world.bodyState(body.id)
}

type ColliderHandle[S Shape2D] struct {
	world *World
	id    ColliderID
}

func (collider ColliderHandle[S]) ID() ColliderID { return collider.id }
func (collider ColliderHandle[S]) Valid() bool {
	return collider.world != nil && collider.world.colliderState(collider.id) != nil
}

func (collider ColliderHandle[S]) Shape() S {
	var zero S
	if collider.world == nil {
		return zero
	}
	state := collider.world.colliderState(collider.id)
	if state == nil {
		return zero
	}
	return shapeFromState[S](*state)
}

func (collider ColliderHandle[S]) SetShape(shape S) {
	if collider.world == nil {
		return
	}
	state := collider.world.colliderState(collider.id)
	if state != nil {
		setColliderShape(state, shape)
	}
}

func (collider ColliderHandle[S]) Filter() Filter {
	if collider.world != nil {
		if state := collider.world.colliderState(collider.id); state != nil {
			return state.filter
		}
	}
	return Filter{}
}

func (collider ColliderHandle[S]) SetFilter(value Filter) {
	if collider.world != nil {
		if state := collider.world.colliderState(collider.id); state != nil {
			state.filter = value.normalized()
		}
	}
}

func (collider ColliderHandle[S]) SetSensor(value bool) {
	if collider.world != nil {
		if state := collider.world.colliderState(collider.id); state != nil {
			state.sensor = value
		}
	}
}

func (collider ColliderHandle[S]) CollisionType() uint32 {
	if collider.world != nil {
		if state := collider.world.colliderState(collider.id); state != nil {
			return state.collisionType
		}
	}
	return 0
}

func (collider ColliderHandle[S]) SetCollisionType(value uint32) {
	if collider.world != nil {
		if state := collider.world.colliderState(collider.id); state != nil {
			state.collisionType = value
		}
	}
}

func (collider ColliderHandle[S]) Sensor() bool {
	if collider.world != nil {
		if state := collider.world.colliderState(collider.id); state != nil {
			return state.sensor
		}
	}
	return false
}

func (collider ColliderHandle[S]) SetMaterial(friction, restitution float64) {
	if collider.world != nil {
		if state := collider.world.colliderState(collider.id); state != nil {
			state.friction = max(friction, 0)
			state.restitution = clamp(restitution, 0, 1)
		}
	}
}
