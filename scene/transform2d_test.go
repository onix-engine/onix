package scene

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func TestTransformPositiveRotationIsCounterClockwise(t *testing.T) {
	transform := NewTransform()
	transform.Rotation = mgl32.QuatRotate(float32(math.Pi/2), mgl32.Vec3{0, 0, 1})
	point := transform.LocalMatrix().Mul4x1(mgl32.Vec4{1, 0, 0, 1})

	if !near32(point.X(), 0) || !near32(point.Y(), 1) {
		t.Fatalf("rotated point = (%v, %v), want (0, 1)", point.X(), point.Y())
	}
}
