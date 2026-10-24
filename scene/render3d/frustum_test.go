package render3d

import (
	"math/rand/v2"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// The previous per-object calculation is an independent boundary oracle.
func uncachedSphereVisible3D(matrix mgl32.Mat4, center mgl32.Vec3, radius float32) bool {
	rows := [4]mgl32.Vec4{
		{matrix[0], matrix[4], matrix[8], matrix[12]}, {matrix[1], matrix[5], matrix[9], matrix[13]},
		{matrix[2], matrix[6], matrix[10], matrix[14]}, {matrix[3], matrix[7], matrix[11], matrix[15]},
	}
	for _, plane := range [...]mgl32.Vec4{rows[3].Add(rows[0]), rows[3].Sub(rows[0]), rows[3].Add(rows[1]), rows[3].Sub(rows[1]), rows[2], rows[3].Sub(rows[2])} {
		if plane.Vec3().Dot(center)+plane.W() < -radius*plane.Vec3().Len() {
			return false
		}
	}
	return true
}

func TestFrustumMatchesPerSpherePlanes(t *testing.T) {
	random := rand.New(rand.NewPCG(23, 45))
	for _, matrix := range []mgl32.Mat4{{}, mgl32.Ident4(), mgl32.Ortho(-10, 10, -5, 5, .1, 100), mgl32.Perspective(1, 1.6, .1, 1000).Mul4(mgl32.Translate3D(10, -20, 30))} {
		frustum := newFrustum3D(matrix)
		for i := range 10000 {
			center := mgl32.Vec3{random.Float32()*200 - 100, random.Float32()*200 - 100, random.Float32()*200 - 100}
			radius := random.Float32() * 10
			if i < 100 {
				center, radius = mgl32.Vec3{float32(i%5) * .5, 0, 0}, float32(i/5)*.1
			}
			if frustum.containsSphere(center, radius) != uncachedSphereVisible3D(matrix, center, radius) {
				t.Fatalf("culling changed for center %v radius %g matrix %v", center, radius, matrix)
			}
		}
	}
}

var frustumBenchmarkHits int

func BenchmarkFrustumSphere(b *testing.B) {
	matrix := mgl32.Perspective(1, 1.6, .1, 1000)
	frustum := newFrustum3D(matrix)
	for _, cached := range []bool{false, true} {
		name := "per-object"
		if cached {
			name = "per-view"
		}
		b.Run(name, func(b *testing.B) {
			var centers [1000]mgl32.Vec3
			for i := range centers {
				centers[i] = mgl32.Vec3{float32(i%10) - 5, 0, -float32(i/10) - 5}
			}
			b.ReportAllocs()
			for b.Loop() {
				for _, center := range centers {
					visible := false
					if cached {
						visible = frustum.containsSphere(center, 1)
					} else {
						visible = uncachedSphereVisible3D(matrix, center, 1)
					}
					if visible {
						frustumBenchmarkHits++
					}
				}
			}
		})
	}
}
