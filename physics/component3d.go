package physics

import (
	"math"

	"github.com/diiyw/onix/ecs"
)

// RigidBody3D binds a world-space transform to a simulated body.
type RigidBody3D struct {
	Type            BodyType
	Mass            float64
	Inertia         float64
	GravityScale    float64
	LinearDamping   float64
	AngularDamping  float64
	Velocity        Vector3D
	AngularVelocity Vector3D
	Force           Vector3D
	Torque          Vector3D
	Impulse         Vector3D
	FixedRotation   bool

	body         Body3D
	defaultsSet  bool
	lastPosition Vector3D
	lastRotation Quaternion
}

func NewRigidBody3D(bodyType BodyType) RigidBody3D {
	return RigidBody3D{Type: bodyType, Mass: 1, GravityScale: 1, defaultsSet: true}
}

func (body *RigidBody3D) Handle() Body3D {
	if body == nil {
		return Body3D{}
	}
	return body.body
}

func (body *RigidBody3D) ApplyForce(force Vector3D) {
	if body != nil {
		body.Force = body.Force.Add(force)
	}
}

func (body *RigidBody3D) ApplyTorque(torque Vector3D) {
	if body != nil {
		body.Torque = body.Torque.Add(torque)
	}
}

func (body *RigidBody3D) ApplyImpulse(impulse Vector3D) {
	if body != nil {
		body.Impulse = body.Impulse.Add(impulse)
	}
}

func (body *RigidBody3D) normalize() {
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

type Collider3D[S Shape3D] struct {
	Shape         S
	Friction      float64
	Restitution   float64
	Sensor        bool
	Filter        Filter
	CollisionType uint32

	handle ColliderHandle3D[S]
}

type SphereCollider3D = Collider3D[Sphere]
type BoxCollider3D = Collider3D[Box3D]
type CapsuleCollider3D = Collider3D[Capsule]
type TriangleMeshCollider3D = Collider3D[TriangleMesh]
type ConvexHullCollider3D = Collider3D[ConvexHull]
type CompoundCollider3D = Collider3D[Compound]

func NewCollider3D[S Shape3D](shape S) Collider3D[S] {
	return Collider3D[S]{Shape: shape, Friction: .5, Filter: FilterAll}
}

func (collider *Collider3D[S]) Handle() ColliderHandle3D[S] {
	if collider == nil {
		return ColliderHandle3D[S]{}
	}
	return collider.handle
}

type CollisionMessage3D struct {
	Phase         CollisionPhase
	Entity        ecs.Entity
	Other         ecs.Entity
	Collider      ColliderID3D
	OtherCollider ColliderID3D
	CollisionType uint32
	OtherType     uint32
	Normal        Vector3D
	Point         Vector3D
	Penetration   float64
}
