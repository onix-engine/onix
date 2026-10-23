package scene

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func TestUITransformSkewXShearsAlongY(t *testing.T) {
	transform := NewUITransform()
	transform.Skew[0] = math.Pi / 4
	point := transform.LocalMatrix(UIResolveContext{}).Mul4x1(mgl32.Vec4{0, 2, 0, 1})
	if abs := math.Abs(float64(point.X() - 2)); abs > 1e-5 || point.Y() != 2 {
		t.Fatalf("skewed point = %v, want (2, 2)", point)
	}
}

func TestUITransformRotationUsesPivot(t *testing.T) {
	transform := NewUITransform()
	transform.Rotation = float32(math.Pi / 2)
	transform.Pivot = mgl32.Vec2{5, 5}
	point := transform.LocalMatrix(UIResolveContext{}).Mul4x1(mgl32.Vec4{10, 5, 0, 1})
	if absX, absY := math.Abs(float64(point.X()-5)), math.Abs(float64(point.Y()-10)); absX > 1e-5 || absY > 1e-5 {
		t.Fatalf("rotated point = %v, want (5, 10)", point)
	}
}

func TestUITransformLinearMatrixPreservesSingularCompositionAndReset(t *testing.T) {
	transform := NewUITransform()
	transform.Translation = Px2(7, 11)
	transform.Pivot = mgl32.Vec2{3, 5}
	transform.Rotation = float32(math.Pi / 2)
	transform.LinearMatrix = mgl32.Scale3D(0, 2, 1)
	point := transform.LocalMatrix(UIResolveContext{}).Mul4x1(mgl32.Vec4{13, 8, 0, 1})
	if point != (mgl32.Vec4{10, 22, 0, 1}) {
		t.Fatalf("singular override lost pivot or translation: %v", point)
	}
	transform.LinearMatrix = mgl32.Mat4{}
	point = transform.LocalMatrix(UIResolveContext{}).Mul4x1(mgl32.Vec4{13, 8, 0, 1})
	if absX, absY := math.Abs(float64(point.X()-7)), math.Abs(float64(point.Y()-26)); absX > 1e-5 || absY > 1e-5 {
		t.Fatalf("reset did not restore authored rotation: %v", point)
	}
}

var benchmarkUITransformMatrix mgl32.Mat4

func BenchmarkUITransformLocalMatrix(b *testing.B) {
	for _, composed := range []bool{false, true} {
		name := "Authored"
		if composed {
			name = "Composed"
		}
		b.Run(name, func(b *testing.B) {
			transform := UITransformFromXY(7, 11)
			transform.Rotation, transform.Skew = 0.4, mgl32.Vec2{0.2, 0.1}
			transform.Pivot, transform.Scale = mgl32.Vec2{3, 5}, mgl32.Vec2{0.7, 2}
			if composed {
				transform.LinearMatrix = mgl32.Scale3D(0, 2, 1).Mul4(mgl32.HomogRotate3DZ(0.4))
			}
			b.ReportAllocs()
			for b.Loop() {
				benchmarkUITransformMatrix = transform.LocalMatrix(UIResolveContext{})
			}
		})
	}
}
