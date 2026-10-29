package project

import (
	"fmt"
	"math"
	"strings"

	"github.com/diiyw/onix/ecs"
	enginephysics "github.com/diiyw/onix/physics"
)

type RigidBody2DSpec struct {
	Type            string     `json:"type,omitempty"`
	Mass            float64    `json:"mass,omitempty"`
	Inertia         float64    `json:"inertia,omitempty"`
	GravityScale    float64    `json:"gravityScale,omitempty"`
	LinearDamping   float64    `json:"linearDamping,omitempty"`
	AngularDamping  float64    `json:"angularDamping,omitempty"`
	Velocity        [2]float64 `json:"velocity,omitempty"`
	AngularVelocity float64    `json:"angularVelocity,omitempty"`
	FixedRotation   bool       `json:"fixedRotation,omitempty"`
}

func NewRigidBody2DSpec() RigidBody2DSpec {
	return RigidBody2DSpec{Type: "dynamic", Mass: 1, GravityScale: 1}
}

type Collider2DSettingsSpec struct {
	Friction      float64 `json:"friction,omitempty"`
	Restitution   float64 `json:"restitution,omitempty"`
	Sensor        bool    `json:"sensor,omitempty"`
	Group         int32   `json:"group,omitempty"`
	Categories    uint64  `json:"categories,omitempty"`
	Mask          uint64  `json:"mask,omitempty"`
	CollisionType uint32  `json:"collisionType,omitempty"`
}

func NewCollider2DSettingsSpec() Collider2DSettingsSpec {
	return Collider2DSettingsSpec{Friction: .5, Categories: math.MaxUint64, Mask: math.MaxUint64}
}

type CircleCollider2DSpec struct {
	Collider2DSettingsSpec
	Offset [2]float64 `json:"offset,omitempty"`
	Radius float64    `json:"radius,omitempty"`
}

func NewCircleCollider2DSpec() CircleCollider2DSpec {
	return CircleCollider2DSpec{Collider2DSettingsSpec: NewCollider2DSettingsSpec()}
}

type BoxCollider2DSpec struct {
	Collider2DSettingsSpec
	Offset   [2]float64 `json:"offset,omitempty"`
	Width    float64    `json:"width,omitempty"`
	Height   float64    `json:"height,omitempty"`
	Rotation float64    `json:"rotation,omitempty"`
}

func NewBoxCollider2DSpec() BoxCollider2DSpec {
	return BoxCollider2DSpec{Collider2DSettingsSpec: NewCollider2DSettingsSpec()}
}

type SegmentCollider2DSpec struct {
	Collider2DSettingsSpec
	Start  [2]float64 `json:"start"`
	End    [2]float64 `json:"end"`
	Radius float64    `json:"radius,omitempty"`
}

func NewSegmentCollider2DSpec() SegmentCollider2DSpec {
	return SegmentCollider2DSpec{Collider2DSettingsSpec: NewCollider2DSettingsSpec(), End: [2]float64{1, 0}}
}

func (spec RigidBody2DSpec) component() enginephysics.RigidBody2D {
	body := enginephysics.NewRigidBody2D(spec.bodyType())
	body.Mass, body.Inertia, body.GravityScale = spec.Mass, spec.Inertia, spec.GravityScale
	body.LinearDamping, body.AngularDamping = spec.LinearDamping, spec.AngularDamping
	body.Velocity = enginephysics.V(spec.Velocity[0], spec.Velocity[1])
	body.AngularVelocity, body.FixedRotation = spec.AngularVelocity, spec.FixedRotation
	return body
}

func (spec RigidBody2DSpec) bodyType() enginephysics.BodyType {
	switch strings.ToLower(strings.TrimSpace(spec.Type)) {
	case "static":
		return enginephysics.BodyStatic
	case "kinematic":
		return enginephysics.BodyKinematic
	default:
		return enginephysics.BodyDynamic
	}
}

func (spec Collider2DSettingsSpec) filter() enginephysics.Filter {
	return enginephysics.NewFilter(spec.Group, spec.Categories, spec.Mask)
}

func (spec Collider2DSettingsSpec) applyColliderSettings[S enginephysics.Shape2D](collider *enginephysics.Collider2D[S]) {
	collider.Friction, collider.Restitution, collider.Sensor = spec.Friction, spec.Restitution, spec.Sensor
	collider.Filter, collider.CollisionType = spec.filter(), spec.CollisionType
}

func (spec CircleCollider2DSpec) component(node SceneNodeSpec) enginephysics.CircleCollider2D {
	radius := spec.Radius
	if radius <= 0 {
		radius = float64(minPositiveSize(node.Width, node.Height)) * .5
	}
	collider := enginephysics.NewCollider2D(enginephysics.Circle{
		Center: enginephysics.V(spec.Offset[0], spec.Offset[1]), Radius: radius,
	})
	spec.Collider2DSettingsSpec.applyColliderSettings(&collider)
	return collider
}

func (spec BoxCollider2DSpec) component(node SceneNodeSpec) enginephysics.BoxCollider2D {
	width, height := spec.Width, spec.Height
	if width <= 0 {
		width = float64(node.Width)
	}
	if height <= 0 {
		height = float64(node.Height)
	}
	shape := enginephysics.NewBox(width, height)
	shape.Center, shape.Rotation = enginephysics.V(spec.Offset[0], spec.Offset[1]), spec.Rotation
	collider := enginephysics.NewCollider2D(shape)
	spec.Collider2DSettingsSpec.applyColliderSettings(&collider)
	return collider
}

func (spec SegmentCollider2DSpec) component() enginephysics.SegmentCollider2D {
	collider := enginephysics.NewCollider2D(enginephysics.Segment{
		Start: enginephysics.V(spec.Start[0], spec.Start[1]), End: enginephysics.V(spec.End[0], spec.End[1]), Radius: spec.Radius,
	})
	spec.Collider2DSettingsSpec.applyColliderSettings(&collider)
	return collider
}

func minPositiveSize(width, height int) int {
	if width <= 0 {
		return max(height, 1)
	}
	if height <= 0 {
		return max(width, 1)
	}
	return min(width, height)
}

func putPhysics2DComponents(bundle *ecs.Bundle, spec SceneNodeSpec) {
	components := spec.Components
	if components.RigidBody2D != nil {
		bundle.Put(components.RigidBody2D.component())
	}
	if components.CircleCollider2D != nil {
		bundle.Put(components.CircleCollider2D.component(spec))
	}
	if components.BoxCollider2D != nil {
		bundle.Put(components.BoxCollider2D.component(spec))
	}
	if components.SegmentCollider2D != nil {
		bundle.Put(components.SegmentCollider2D.component())
	}
}

func applyPhysics2DComponents(world *ecs.World, entity ecs.Entity, spec SceneNodeSpec) error {
	components := spec.Components
	if err := replacePhysicsComponent(world, entity, components.RigidBody2D, func(value RigidBody2DSpec) enginephysics.RigidBody2D { return value.component() }); err != nil {
		return err
	}
	if err := replacePhysicsComponent(world, entity, components.CircleCollider2D, func(value CircleCollider2DSpec) enginephysics.CircleCollider2D { return value.component(spec) }); err != nil {
		return err
	}
	if err := replacePhysicsComponent(world, entity, components.BoxCollider2D, func(value BoxCollider2DSpec) enginephysics.BoxCollider2D { return value.component(spec) }); err != nil {
		return err
	}
	return replacePhysicsComponent(world, entity, components.SegmentCollider2D, func(value SegmentCollider2DSpec) enginephysics.SegmentCollider2D { return value.component() })
}

func replacePhysicsComponent[Spec any, Component any](world *ecs.World, entity ecs.Entity, spec *Spec, build func(Spec) Component) error {
	if spec == nil {
		world.Remove[Component](entity)
		return nil
	}
	return world.Insert(entity, build(*spec))
}

func validatePhysics2DComponents(name, id string, components SceneComponents) error {
	if !components.HasPhysics2D() {
		return nil
	}
	if components.RigidBody2D == nil && (components.CircleCollider2D != nil || components.BoxCollider2D != nil || components.SegmentCollider2D != nil) {
		return fmt.Errorf("%s game object %q collider requires rigidBody2D", name, id)
	}
	if body := components.RigidBody2D; body != nil {
		switch strings.ToLower(strings.TrimSpace(body.Type)) {
		case "", "dynamic", "static", "kinematic":
		default:
			return fmt.Errorf("%s game object %q has unsupported rigidBody2D type %q", name, id, body.Type)
		}
		if !finitePositive(body.Mass) || body.Inertia < 0 || !finite(body.Inertia) || !finite(body.GravityScale) || body.LinearDamping < 0 || !finite(body.LinearDamping) || body.AngularDamping < 0 || !finite(body.AngularDamping) || !finite(body.Velocity[0]) || !finite(body.Velocity[1]) || !finite(body.AngularVelocity) {
			return fmt.Errorf("%s game object %q has invalid rigidBody2D values", name, id)
		}
	}
	if collider := components.CircleCollider2D; collider != nil {
		if collider.Radius < 0 || !finite(collider.Radius) || !finite(collider.Offset[0]) || !finite(collider.Offset[1]) {
			return fmt.Errorf("%s game object %q has invalid circleCollider2D shape", name, id)
		}
		if err := validateColliderSettings(name, id, "circleCollider2D", collider.Collider2DSettingsSpec); err != nil {
			return err
		}
	}
	if collider := components.BoxCollider2D; collider != nil {
		if collider.Width < 0 || collider.Height < 0 || !finite(collider.Width) || !finite(collider.Height) || !finite(collider.Offset[0]) || !finite(collider.Offset[1]) || !finite(collider.Rotation) {
			return fmt.Errorf("%s game object %q has invalid boxCollider2D shape", name, id)
		}
		if err := validateColliderSettings(name, id, "boxCollider2D", collider.Collider2DSettingsSpec); err != nil {
			return err
		}
	}
	if collider := components.SegmentCollider2D; collider != nil {
		if collider.Start == collider.End || collider.Radius < 0 || !finite(collider.Start[0]) || !finite(collider.Start[1]) || !finite(collider.End[0]) || !finite(collider.End[1]) || !finite(collider.Radius) {
			return fmt.Errorf("%s game object %q has invalid segmentCollider2D shape", name, id)
		}
		if err := validateColliderSettings(name, id, "segmentCollider2D", collider.Collider2DSettingsSpec); err != nil {
			return err
		}
	}
	return nil
}

func validateColliderSettings(name, id, component string, spec Collider2DSettingsSpec) error {
	if spec.Friction < 0 || !finite(spec.Friction) || spec.Restitution < 0 || spec.Restitution > 1 || !finite(spec.Restitution) {
		return fmt.Errorf("%s game object %q has invalid %s material", name, id, component)
	}
	return nil
}

type RigidBody3DSpec struct {
	Type            string     `json:"type,omitempty"`
	Mass            float64    `json:"mass,omitempty"`
	Inertia         float64    `json:"inertia,omitempty"`
	GravityScale    float64    `json:"gravityScale,omitempty"`
	LinearDamping   float64    `json:"linearDamping,omitempty"`
	AngularDamping  float64    `json:"angularDamping,omitempty"`
	Velocity        [3]float64 `json:"velocity,omitempty"`
	AngularVelocity [3]float64 `json:"angularVelocity,omitempty"`
	FixedRotation   bool       `json:"fixedRotation,omitempty"`
}

func NewRigidBody3DSpec() RigidBody3DSpec {
	return RigidBody3DSpec{Type: "dynamic", Mass: 1, GravityScale: 1}
}

type Collider3DSettingsSpec struct {
	Friction      float64 `json:"friction,omitempty"`
	Restitution   float64 `json:"restitution,omitempty"`
	Sensor        bool    `json:"sensor,omitempty"`
	Group         int32   `json:"group,omitempty"`
	Categories    uint64  `json:"categories,omitempty"`
	Mask          uint64  `json:"mask,omitempty"`
	CollisionType uint32  `json:"collisionType,omitempty"`
}

func NewCollider3DSettingsSpec() Collider3DSettingsSpec {
	return Collider3DSettingsSpec{Friction: .5, Categories: math.MaxUint64, Mask: math.MaxUint64}
}

type TriangleMeshCollider3DSpec struct {
	Collider3DSettingsSpec
	Model string `json:"model,omitempty"`
}

func NewTriangleMeshCollider3DSpec() TriangleMeshCollider3DSpec {
	return TriangleMeshCollider3DSpec{Collider3DSettingsSpec: NewCollider3DSettingsSpec()}
}

type ConvexHullCollider3DSpec struct {
	Collider3DSettingsSpec
	Model string `json:"model,omitempty"`
}

func NewConvexHullCollider3DSpec() ConvexHullCollider3DSpec {
	return ConvexHullCollider3DSpec{Collider3DSettingsSpec: NewCollider3DSettingsSpec()}
}

type CompoundCollider3DSpec struct {
	Collider3DSettingsSpec
	Model string `json:"model,omitempty"`
}

func NewCompoundCollider3DSpec() CompoundCollider3DSpec {
	return CompoundCollider3DSpec{Collider3DSettingsSpec: NewCollider3DSettingsSpec()}
}

func (spec RigidBody3DSpec) component() enginephysics.RigidBody3D {
	body := enginephysics.NewRigidBody3D(spec.bodyType())
	body.Mass, body.Inertia, body.GravityScale = spec.Mass, spec.Inertia, spec.GravityScale
	body.LinearDamping, body.AngularDamping = spec.LinearDamping, spec.AngularDamping
	body.Velocity = enginephysics.V3(spec.Velocity[0], spec.Velocity[1], spec.Velocity[2])
	body.AngularVelocity = enginephysics.V3(spec.AngularVelocity[0], spec.AngularVelocity[1], spec.AngularVelocity[2])
	body.FixedRotation = spec.FixedRotation
	return body
}

func (spec RigidBody3DSpec) bodyType() enginephysics.BodyType {
	switch strings.ToLower(strings.TrimSpace(spec.Type)) {
	case "static":
		return enginephysics.BodyStatic
	case "kinematic":
		return enginephysics.BodyKinematic
	default:
		return enginephysics.BodyDynamic
	}
}

func (spec Collider3DSettingsSpec) filter() enginephysics.Filter {
	return enginephysics.NewFilter(spec.Group, spec.Categories, spec.Mask)
}

func (spec Collider3DSettingsSpec) applyColliderSettings3D[S enginephysics.Shape3D](collider *enginephysics.Collider3D[S]) {
	collider.Friction, collider.Restitution, collider.Sensor = spec.Friction, spec.Restitution, spec.Sensor
	collider.Filter, collider.CollisionType = spec.filter(), spec.CollisionType
}

func collisionModelID(node SceneNodeSpec, explicit string) string {
	if id := strings.TrimSpace(explicit); id != "" {
		return id
	}
	if node.Components.Model != nil {
		return strings.TrimSpace(node.Components.Model.Resource)
	}
	return ""
}

func putPhysics3DComponents(bundle *Bundle, output *ecs.Bundle, spec SceneNodeSpec) error {
	components := spec.Components
	if components.RigidBody3D != nil {
		output.Put(components.RigidBody3D.component())
	}
	if components.TriangleMeshCollider3D != nil {
		cooked, err := bundle.collisionForModelID(collisionModelID(spec, components.TriangleMeshCollider3D.Model))
		if err != nil {
			return err
		}
		if cooked.Mode != enginephysics.CollisionModeTriangleMesh {
			return fmt.Errorf("game object %q triangleMeshCollider3D requires model collision mode %q", spec.ID, enginephysics.CollisionModeTriangleMesh)
		}
		collider := enginephysics.NewCollider3D(cooked.TriangleMesh)
		components.TriangleMeshCollider3D.Collider3DSettingsSpec.applyColliderSettings3D(&collider)
		output.Put(collider)
	}
	if components.ConvexHullCollider3D != nil {
		cooked, err := bundle.collisionForModelID(collisionModelID(spec, components.ConvexHullCollider3D.Model))
		if err != nil {
			return err
		}
		if cooked.Mode != enginephysics.CollisionModeSingleConvex {
			return fmt.Errorf("game object %q convexHullCollider3D requires model collision mode %q", spec.ID, enginephysics.CollisionModeSingleConvex)
		}
		collider := enginephysics.NewCollider3D(cooked.ConvexHull)
		components.ConvexHullCollider3D.Collider3DSettingsSpec.applyColliderSettings3D(&collider)
		output.Put(collider)
	}
	if components.CompoundCollider3D != nil {
		cooked, err := bundle.collisionForModelID(collisionModelID(spec, components.CompoundCollider3D.Model))
		if err != nil {
			return err
		}
		if cooked.Mode != enginephysics.CollisionModeConvexDecomposition {
			return fmt.Errorf("game object %q compoundCollider3D requires model collision mode %q", spec.ID, enginephysics.CollisionModeConvexDecomposition)
		}
		collider := enginephysics.NewCollider3D(cooked.Compound)
		components.CompoundCollider3D.Collider3DSettingsSpec.applyColliderSettings3D(&collider)
		output.Put(collider)
	}
	return nil
}

func applyPhysics3DComponents(bundle *Bundle, world *ecs.World, entity ecs.Entity, spec SceneNodeSpec) error {
	components := spec.Components
	if err := replacePhysicsComponent(world, entity, components.RigidBody3D, func(value RigidBody3DSpec) enginephysics.RigidBody3D { return value.component() }); err != nil {
		return err
	}
	remove3D := func() {
		world.Remove[enginephysics.TriangleMeshCollider3D](entity)
		world.Remove[enginephysics.ConvexHullCollider3D](entity)
		world.Remove[enginephysics.CompoundCollider3D](entity)
	}
	remove3D()
	if components.TriangleMeshCollider3D != nil || components.ConvexHullCollider3D != nil || components.CompoundCollider3D != nil {
		return insertPhysics3DCollider(world, entity, bundle, spec)
	}
	return nil
}

func insertPhysics3DCollider(world *ecs.World, entity ecs.Entity, bundle *Bundle, spec SceneNodeSpec) error {
	components := spec.Components
	if components.TriangleMeshCollider3D != nil {
		cooked, err := bundle.collisionForModelID(collisionModelID(spec, components.TriangleMeshCollider3D.Model))
		if err != nil {
			return err
		}
		if cooked.Mode != enginephysics.CollisionModeTriangleMesh {
			return fmt.Errorf("game object %q triangleMeshCollider3D requires model collision mode %q", spec.ID, enginephysics.CollisionModeTriangleMesh)
		}
		collider := enginephysics.NewCollider3D(cooked.TriangleMesh)
		components.TriangleMeshCollider3D.Collider3DSettingsSpec.applyColliderSettings3D(&collider)
		if err := world.Insert(entity, collider); err != nil {
			return err
		}
	}
	if components.ConvexHullCollider3D != nil {
		cooked, err := bundle.collisionForModelID(collisionModelID(spec, components.ConvexHullCollider3D.Model))
		if err != nil {
			return err
		}
		if cooked.Mode != enginephysics.CollisionModeSingleConvex {
			return fmt.Errorf("game object %q convexHullCollider3D requires model collision mode %q", spec.ID, enginephysics.CollisionModeSingleConvex)
		}
		collider := enginephysics.NewCollider3D(cooked.ConvexHull)
		components.ConvexHullCollider3D.Collider3DSettingsSpec.applyColliderSettings3D(&collider)
		if err := world.Insert(entity, collider); err != nil {
			return err
		}
	}
	if components.CompoundCollider3D != nil {
		cooked, err := bundle.collisionForModelID(collisionModelID(spec, components.CompoundCollider3D.Model))
		if err != nil {
			return err
		}
		if cooked.Mode != enginephysics.CollisionModeConvexDecomposition {
			return fmt.Errorf("game object %q compoundCollider3D requires model collision mode %q", spec.ID, enginephysics.CollisionModeConvexDecomposition)
		}
		collider := enginephysics.NewCollider3D(cooked.Compound)
		components.CompoundCollider3D.Collider3DSettingsSpec.applyColliderSettings3D(&collider)
		return world.Insert(entity, collider)
	}
	return nil
}

func validatePhysics3DComponents(name, id string, components SceneComponents) error {
	if !components.HasPhysics3D() {
		return nil
	}
	if components.RigidBody3D == nil && (components.TriangleMeshCollider3D != nil || components.ConvexHullCollider3D != nil || components.CompoundCollider3D != nil) {
		return fmt.Errorf("%s game object %q collider requires rigidBody3D", name, id)
	}
	if body := components.RigidBody3D; body != nil {
		switch strings.ToLower(strings.TrimSpace(body.Type)) {
		case "", "dynamic", "static", "kinematic":
		default:
			return fmt.Errorf("%s game object %q has unsupported rigidBody3D type %q", name, id, body.Type)
		}
		if !finitePositive(body.Mass) || body.Inertia < 0 || !finite(body.Inertia) || !finite(body.GravityScale) || body.LinearDamping < 0 || !finite(body.LinearDamping) || body.AngularDamping < 0 || !finite(body.AngularDamping) || !finite(body.Velocity[0]) || !finite(body.Velocity[1]) || !finite(body.Velocity[2]) || !finite(body.AngularVelocity[0]) || !finite(body.AngularVelocity[1]) || !finite(body.AngularVelocity[2]) {
			return fmt.Errorf("%s game object %q has invalid rigidBody3D values", name, id)
		}
		if components.TriangleMeshCollider3D != nil && body.bodyType() == enginephysics.BodyDynamic {
			return fmt.Errorf("%s game object %q triangleMeshCollider3D cannot be attached to a dynamic rigidBody3D", name, id)
		}
	}
	for component, model := range map[string]string{
		"triangleMeshCollider3D": func() string {
			if components.TriangleMeshCollider3D == nil {
				return ""
			}
			return components.TriangleMeshCollider3D.Model
		}(),
		"convexHullCollider3D": func() string {
			if components.ConvexHullCollider3D == nil {
				return ""
			}
			return components.ConvexHullCollider3D.Model
		}(),
		"compoundCollider3D": func() string {
			if components.CompoundCollider3D == nil {
				return ""
			}
			return components.CompoundCollider3D.Model
		}(),
	} {
		if model != "" && strings.TrimSpace(model) == "" {
			return fmt.Errorf("%s game object %q has invalid %s model", name, id, component)
		}
	}
	if collider := components.TriangleMeshCollider3D; collider != nil {
		if err := validateColliderSettings3D(name, id, "triangleMeshCollider3D", collider.Collider3DSettingsSpec); err != nil {
			return err
		}
	}
	if collider := components.ConvexHullCollider3D; collider != nil {
		if err := validateColliderSettings3D(name, id, "convexHullCollider3D", collider.Collider3DSettingsSpec); err != nil {
			return err
		}
	}
	if collider := components.CompoundCollider3D; collider != nil {
		if err := validateColliderSettings3D(name, id, "compoundCollider3D", collider.Collider3DSettingsSpec); err != nil {
			return err
		}
	}
	return nil
}

func validateColliderSettings3D(name, id, component string, spec Collider3DSettingsSpec) error {
	if spec.Friction < 0 || !finite(spec.Friction) || spec.Restitution < 0 || spec.Restitution > 1 || !finite(spec.Restitution) {
		return fmt.Errorf("%s game object %q has invalid %s material", name, id, component)
	}
	return nil
}

func finite(value float64) bool         { return !math.IsNaN(value) && !math.IsInf(value, 0) }
func finitePositive(value float64) bool { return value > 0 && finite(value) }
