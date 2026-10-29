package physics

import (
	"fmt"
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
	"math"
)

func matrixRotation(matrix mgl32.Mat4) mgl32.Quat {
	for column := range 3 {
		v := mgl32.Vec3{matrix[column*4], matrix[column*4+1], matrix[column*4+2]}
		if length := v.Len(); length > 0 {
			v = v.Mul(1 / length)
		}
		matrix[column*4], matrix[column*4+1], matrix[column*4+2] = v[0], v[1], v[2]
	}
	matrix[12], matrix[13], matrix[14] = 0, 0, 0
	return mgl32.Mat4ToQuat(matrix).Normalize()
}

// ValidateBodyTransform rejects transforms the rigid-body solvers cannot
// represent. Shape dimensions are authored in world units; scaling, reflection
// and shear must be baked into the collision resource instead.
func ValidateBodyTransform(matrix mgl32.Mat4, planar bool) error {
	for _, value := range matrix {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return fmt.Errorf("physics: body transform must be finite")
		}
	}
	const tolerance = 1e-4
	axes := [3]mgl32.Vec3{}
	for i := range axes {
		axes[i] = mgl32.Vec3{matrix[4*i], matrix[4*i+1], matrix[4*i+2]}
		if math.Abs(float64(axes[i].LenSqr()-1)) > tolerance {
			return fmt.Errorf("physics: body transform requires unit scale; bake size into the collider")
		}
	}
	if math.Abs(float64(axes[0].Dot(axes[1]))) > tolerance || math.Abs(float64(axes[0].Dot(axes[2]))) > tolerance || math.Abs(float64(axes[1].Dot(axes[2]))) > tolerance || matrix.Det() < 0 {
		return fmt.Errorf("physics: body transform cannot contain shear or reflection")
	}
	if planar && (math.Abs(float64(matrix[2])) > tolerance || math.Abs(float64(matrix[6])) > tolerance || math.Abs(float64(matrix[10]-1)) > tolerance) {
		return fmt.Errorf("physics: 2D body rotation must stay in the XY plane")
	}
	return nil
}

func ecsWorldPose(world *ecs.World, entity ecs.Entity, local scene.Transform, planar bool) (scene.Transform, error) {
	matrix := local.LocalMatrix()
	if parent, ok := world.Get[scene.Parent](entity); ok {
		matrix = ecsPhysicsParentMatrix(world, parent.Entity, false).Mul4(matrix)
	}
	if err := ValidateBodyTransform(matrix, planar); err != nil {
		return local, fmt.Errorf("entity %d: %w", entity, err)
	}
	local.Translation = mgl32.Vec3{matrix[12], matrix[13], matrix[14]}
	local.Rotation = matrixRotation(matrix)
	return local, nil
}

// During writeback a parent body may not have been visited yet. Resolve its
// solver pose directly so query order cannot change nested local coordinates.
func ecsPhysicsParentMatrix(world *ecs.World, entity ecs.Entity, simulated bool) mgl32.Mat4 {
	result := mgl32.Ident4()
	for entity != 0 {
		transform, ok := world.Get[scene.Transform](entity)
		if !ok {
			break
		}
		if simulated && scene.IsActive(world, entity) {
			pose := transform
			found := false
			if body, ok := world.Get[RigidBody2D](entity); ok && body.body.Valid() {
				state := body.body.state()
				pose.Translation[0], pose.Translation[1] = float32(state.position.X), float32(state.position.Y)
				pose.Rotation = mgl32.QuatRotate(float32(state.rotation), mgl32.Vec3{0, 0, 1})
				found = true
			} else if body, ok := world.Get[RigidBody3D](entity); ok && body.body.Valid() {
				state := body.body.state()
				pose.Translation = vectorToMGL(state.position)
				pose.Rotation = quaternionToMGL(state.rotation)
				found = true
			}
			if found {
				if global, ok := world.Get[scene.GlobalTransform](entity); ok {
					for column := range 3 {
						pose.Scale[column] = mgl32.Vec3{global.Matrix[column*4], global.Matrix[column*4+1], global.Matrix[column*4+2]}.Len()
					}
				}
				return pose.LocalMatrix().Mul4(result)
			}
		}
		result = transform.LocalMatrix().Mul4(result)
		parent, ok := world.Get[scene.Parent](entity)
		if !ok {
			break
		}
		entity = parent.Entity
	}
	return result
}

func ecsLocalPhysicsPose(world *ecs.World, entity ecs.Entity, pose scene.Transform) (scene.Transform, scene.Transform) {
	parent := mgl32.Ident4()
	if link, ok := world.Get[scene.Parent](entity); ok {
		parent = ecsPhysicsParentMatrix(world, link.Entity, true)
	}
	if parent != mgl32.Ident4() && parent.Det() != 0 {
		position := parent.Inv().Mul4x1(pose.Translation.Vec4(1))
		pose.Translation = position.Vec3()
		pose.Rotation = matrixRotation(parent).Conjugate().Mul(pose.Rotation).Normalize()
	}
	matrix := parent.Mul4(pose.LocalMatrix())
	rounded := pose
	rounded.Translation = mgl32.Vec3{matrix[12], matrix[13], matrix[14]}
	rounded.Rotation = matrixRotation(matrix)
	return pose, rounded
}
