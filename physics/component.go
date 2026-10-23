package physics

import (
	"math"

	"github.com/diiyw/onix/ecs"
)

// RigidBody2D turns a world-space scene node into a simulated body. Dynamic
// bodies write their pose back to Transform; static and kinematic bodies read
// their pose from Transform before each fixed step.
type RigidBody2D struct {
	Type            BodyType
	Mass            float64
	Inertia         float64
	GravityScale    float64
	LinearDamping   float64
	AngularDamping  float64
	Velocity        Vector
	AngularVelocity float64
	Force           Vector
	Torque          float64
	Impulse         Vector
	FixedRotation   bool

	body         Body
	defaultsSet  bool
	initialized  bool
	lastPosition Vector
	lastRotation float64
}

func NewRigidBody2D(bodyType BodyType) RigidBody2D {
	return RigidBody2D{Type: bodyType, Mass: 1, GravityScale: 1, defaultsSet: true}
}

func (body *RigidBody2D) Handle() Body {
	if body == nil {
		return Body{}
	}
	return body.body
}

func (body *RigidBody2D) ApplyForce(force Vector) {
	if body != nil {
		body.Force = body.Force.Add(force)
	}
}

func (body *RigidBody2D) ApplyTorque(torque float64) {
	if body != nil {
		body.Torque += torque
	}
}

func (body *RigidBody2D) ApplyImpulse(impulse Vector) {
	if body != nil {
		body.Impulse = body.Impulse.Add(impulse)
	}
}

func (body *RigidBody2D) normalize() {
	if !body.defaultsSet {
		if body.GravityScale == 0 {
			body.GravityScale = 1
		}
		body.defaultsSet = true
	}
	if body.Mass <= 0 || math.IsNaN(body.Mass) || math.IsInf(body.Mass, 0) {
		body.Mass = 1
	}
	body.LinearDamping = max(body.LinearDamping, 0)
	body.AngularDamping = max(body.AngularDamping, 0)
}

type Collider2D[S Shape2D] struct {
	Shape         S
	Friction      float64
	Restitution   float64
	Sensor        bool
	Filter        Filter
	CollisionType uint32

	handle ColliderHandle[S]
}

type CircleCollider2D = Collider2D[Circle]
type BoxCollider2D = Collider2D[Box]
type SegmentCollider2D = Collider2D[Segment]

func NewCollider2D[S Shape2D](shape S) Collider2D[S] {
	return Collider2D[S]{Shape: shape, Friction: .5, Filter: FilterAll}
}

func (collider *Collider2D[S]) Handle() ColliderHandle[S] {
	if collider == nil {
		return ColliderHandle[S]{}
	}
	return collider.handle
}

type CollisionMessage2D struct {
	Phase         CollisionPhase
	Entity        ecs.Entity
	Other         ecs.Entity
	Collider      ColliderID
	OtherCollider ColliderID
	CollisionType uint32
	OtherType     uint32
	Normal        Vector
	Point         Vector
	Penetration   float64
}
