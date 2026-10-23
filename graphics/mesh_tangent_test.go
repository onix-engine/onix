package graphics

import (
	"math"
	"testing"

	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

func TestGenerateTangentsSharedVerticesAndMirroredUV(t *testing.T) {
	vertices := []render.Vertex3D{
		{Position: [3]float32{0, 0, 0}, Normal: [3]float32{0, 0, 1}, UV: [2]float32{0, 0}},
		{Position: [3]float32{1, 0, 0}, Normal: [3]float32{0, 0, 1}, UV: [2]float32{1, 0}},
		{Position: [3]float32{1, 1, 0}, Normal: [3]float32{0, 0, 1}, UV: [2]float32{1, 1}},
		{Position: [3]float32{0, 1, 0}, Normal: [3]float32{0, 0, 1}, UV: [2]float32{0, 1}},
	}
	for _, mirror := range []bool{false, true} {
		if mirror {
			for i := range vertices {
				vertices[i].UV[0] = -vertices[i].UV[0]
			}
		}
		GenerateTangents(vertices, []uint32{4, 5, 6, 4, 6, 7}, 4)
		want := [4]float32{1, 0, 0, 1}
		if mirror {
			want = [4]float32{-1, 0, 0, -1}
		}
		for _, vertex := range vertices {
			if vertex.Tangent != want {
				t.Fatalf("mirror=%t: tangent %v, want %v", mirror, vertex.Tangent, want)
			}
		}
	}
	for i := range vertices {
		vertices[i].UV = [2]float32{}
	}
	GenerateTangents(vertices, []uint32{0, 1, 2}, 0)
	for _, vertex := range vertices {
		if vertex.Tangent != [4]float32{} {
			t.Fatal("degenerate UV produced a tangent")
		}
	}
}

func TestTangentMorphSkinAndNonuniformMirrorReuseVertices(t *testing.T) {
	m := &Mesh{Vertices: []render.Vertex3D{{Normal: [3]float32{0, 1, 1}, Tangent: [4]float32{1, 0, 0, -1}}},
		MorphTargets: []MorphTarget{{PositionDelta: [][3]float32{{}}, TangentDelta: [][3]float32{{0, 1, 0}}}},
		Skin:         []SkinVertex{{Active: true, Joints: [4]uint32{0}, Weights: [4]float32{1}}},
	}
	joints := []mgl32.Mat4{mgl32.Scale3D(-2, 3, 4)}
	vertices, err := m.DeformVertices(nil, []float32{.5}, joints)
	if err != nil {
		t.Fatal(err)
	}
	if vertices[0].Tangent != [4]float32{-2, 1.5, 0, 1} {
		t.Fatal(vertices[0].Tangent)
	}
	// Normals use inverse transpose, while tangents use the linear transform.
	n := vertices[0].Normal
	if math.Abs(float64(n[1]/n[2]-4.0/3.0)) > 1e-6 {
		t.Fatal(n)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		vertices, err = m.DeformVertices(vertices, []float32{.5}, joints)
	}); allocations != 0 || err != nil {
		t.Fatalf("allocations=%v, err=%v", allocations, err)
	}
	m.MorphTargets[0].TangentDelta = [][3]float32{{}, {}}
	if _, err := m.DeformVertices(vertices, []float32{1}, joints); err == nil {
		t.Fatal("accepted invalid tangent delta count")
	}
}
