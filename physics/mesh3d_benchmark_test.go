package physics

import (
	"math"
	"testing"
)

func cookingGrid3D(grid int) ([]Vector3D, []uint32) {
	vertices := make([]Vector3D, 0, (grid+1)*(grid+1))
	for z := 0; z <= grid; z++ {
		for x := 0; x <= grid; x++ {
			vertices = append(vertices, V3(float64(x), 0, float64(z)))
		}
	}
	indices := make([]uint32, 0, grid*grid*6)
	for z := 0; z < grid; z++ {
		for x := 0; x < grid; x++ {
			a := uint32(z*(grid+1) + x)
			b, c, d := a+1, a+uint32(grid+1), a+uint32(grid+2)
			indices = append(indices, a, b, c, b, d, c)
		}
	}
	return vertices, indices
}

func cookingSphere3D(count int) []Vector3D {
	vertices := make([]Vector3D, count)
	for index := range vertices {
		y := 1 - 2*(float64(index)+.5)/float64(count)
		radius := math.Sqrt(1 - y*y)
		angle := float64(index) * math.Pi * (3 - math.Sqrt(5))
		vertices[index] = V3(math.Cos(angle)*radius, y, math.Sin(angle)*radius)
	}
	return vertices
}

func BenchmarkTriangleMeshCook(b *testing.B) {
	vertices, indices := cookingGrid3D(24)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := NewTriangleMesh(vertices, indices); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkConvexHullCook(b *testing.B) {
	vertices := cookingSphere3D(128)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := NewConvexHull(vertices); err != nil {
			b.Fatal(err)
		}
	}
}
