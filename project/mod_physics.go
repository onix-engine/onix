package project

import (
	"fmt"

	"github.com/diiyw/onix/physics"
	"github.com/go-gl/mathgl/mgl32"
)

func modWorldMatrix(t ModTransform) mgl32.Mat4 {
	rotation := mgl32.QuatRotate(t.Rotation[2], mgl32.Vec3{0, 0, 1}).Mul(mgl32.QuatRotate(t.Rotation[1], mgl32.Vec3{0, 1, 0})).Mul(mgl32.QuatRotate(t.Rotation[0], mgl32.Vec3{1, 0, 0}))
	return mgl32.Translate3D(t.Position[0], t.Position[1], t.Position[2]).Mul4(rotation.Mat4()).Mul4(mgl32.Scale3D(t.Scale[0], t.Scale[1], t.Scale[2]))
}

func validateModPhysicsTransforms(plan *ModPlan, placement mgl32.Mat4) error {
	root := placement.Mul4(modWorldMatrix(plan.Transform))
	validate := func(matrix mgl32.Mat4, content ModComponents) error {
		if content.RigidBody2D == nil && content.RigidBody3D == nil {
			return nil
		}
		return physics.ValidateBodyTransform(matrix, content.RigidBody2D != nil)
	}
	if err := validate(root, plan.Spec.Components); err != nil {
		return err
	}
	matrices := make([]mgl32.Mat4, len(plan.Elements))
	for i, element := range plan.Elements {
		parent := root
		if element.Parent >= 0 {
			parent = matrices[element.Parent]
		}
		matrices[i] = parent.Mul4(modWorldMatrix(element.Transform))
		if err := validate(matrices[i], element.Spec.Components); err != nil {
			return fmt.Errorf("element %s: %w", element.Spec.ID, err)
		}
	}
	return nil
}
