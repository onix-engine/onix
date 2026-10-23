package physics

import (
	"math"
	"slices"
	"strconv"
	"testing"
)

func TestCollisionCookingOwnsResults(t *testing.T) {
	for worker := range 4 {
		t.Run(strconv.Itoa(worker), func(t *testing.T) {
			t.Parallel()
			vertices, indices := cookingGrid3D(4)
			points := cookingSphere3D(32)
			mesh, err := NewTriangleMesh(vertices, indices)
			if err != nil {
				t.Fatal(err)
			}
			hull, err := NewConvexHull(points)
			if err != nil {
				t.Fatal(err)
			}
			meshVertices, meshIndices := slices.Clone(mesh.Vertices), slices.Clone(mesh.Indices)
			meshTriangles, meshNodes := slices.Clone(mesh.TriangleIndices), slices.Clone(mesh.BVH)
			hullVertices, hullFaces, hullEdges := slices.Clone(hull.Vertices), slices.Clone(hull.Faces), slices.Clone(hull.Edges)
			for round := range 8 {
				for index := range vertices {
					vertices[index].Y = float64(round + 1)
				}
				for index := range points {
					points[index] = points[index].Mul(1.1)
				}
				if _, err := NewTriangleMesh(vertices, indices); err != nil {
					t.Fatal(err)
				}
				if _, err := NewConvexHull(points); err != nil {
					t.Fatal(err)
				}
			}
			if !slices.Equal(mesh.Vertices, meshVertices) || !slices.Equal(mesh.Indices, meshIndices) ||
				!slices.Equal(mesh.TriangleIndices, meshTriangles) || !slices.Equal(mesh.BVH, meshNodes) {
				t.Fatal("later cooking or input edits overwrote a returned triangle mesh")
			}
			if !slices.Equal(hull.Vertices, hullVertices) || !slices.Equal(hull.Faces, hullFaces) || !slices.Equal(hull.Edges, hullEdges) {
				t.Fatal("later cooking or input edits overwrote a returned convex hull")
			}
			if hit, ok := mesh.Raycast(V3(1.25, 2, 1.25), V3(0, -1, 0), 10); !ok || hit.Distance != 2 {
				t.Fatalf("retained mesh query changed after later cooking: hit=%+v ok=%v", hit, ok)
			}
		})
	}
}

func TestTriangleMeshCookingRecoversAfterInvalidInput(t *testing.T) {
	vertices, indices := cookingGrid3D(4)
	badVertices := slices.Clone(vertices)
	badVertices[len(badVertices)-1].X = math.NaN()
	badIndices := slices.Clone(indices)
	badIndices[len(badIndices)-1] = uint32(len(vertices))
	for _, invalid := range []struct {
		vertices []Vector3D
		indices  []uint32
	}{
		{badVertices, indices},
		{vertices, badIndices},
		{vertices, []uint32{0, 0, 1}},
		{vertices, []uint32{0, 1}},
	} {
		if _, err := NewTriangleMesh(invalid.vertices, invalid.indices); err == nil {
			t.Fatal("invalid mesh was accepted")
		}
		// Duplicate the first vertex and refer to it from the first triangle.
		// Stale remaps or deduplication entries would corrupt this valid cook.
		validVertices := append(slices.Clone(vertices), vertices[0])
		validIndices := slices.Clone(indices)
		validIndices[0] = uint32(len(vertices))
		mesh, err := NewTriangleMesh(validVertices, validIndices)
		if err != nil {
			t.Fatal(err)
		}
		if len(mesh.Vertices) != len(vertices) || !slices.Equal(mesh.Indices, indices) || mesh.TriangleCount() != 32 {
			t.Fatal("valid cook was corrupted after an invalid input")
		}
	}
}

func TestConvexHullCookingKeepsSurfaceClosed(t *testing.T) {
	points := cookingSphere3D(128)
	// Interior points and near duplicates must not leave holes in the surface.
	points = append(points, Vector3D{}, V3(.1, .2, .3), points[0].Add(V3(1e-14, 0, 0)))
	hull, err := NewConvexHull(points)
	if err != nil {
		t.Fatal(err)
	}
	if len(hull.Faces) != 252 || len(hull.Edges) != 378 {
		t.Fatalf("unexpected sphere topology: faces=%d edges=%d", len(hull.Faces), len(hull.Edges))
	}
	edges := make(map[[2]uint32]int)
	for _, face := range hull.Faces {
		if math.Abs(face.Normal.LengthSquared()-1) > 1e-12 {
			t.Fatalf("face normal is not normalized: %+v", face)
		}
		anchor := hull.Vertices[face.Indices[0]]
		for _, point := range points {
			if distance := face.Normal.Dot(point.Sub(anchor)); distance > 1e-9 {
				t.Fatalf("point is outside the cooked hull: distance=%g face=%+v", distance, face)
			}
		}
		for _, edge := range [3][2]uint32{{face.Indices[0], face.Indices[1]}, {face.Indices[1], face.Indices[2]}, {face.Indices[2], face.Indices[0]}} {
			if edge[0] > edge[1] {
				edge[0], edge[1] = edge[1], edge[0]
			}
			edges[edge]++
		}
	}
	for _, edge := range hull.Edges {
		if count := edges[edge]; count != 2 {
			t.Fatalf("edge %v belongs to %d faces, want 2", edge, count)
		}
		delete(edges, edge)
	}
	if len(edges) != 0 {
		t.Fatalf("%d surface edges are missing from the cooked hull", len(edges))
	}
}

func TestCompoundCooksMissingEdgesForMultipleChildren(t *testing.T) {
	hull, err := NewConvexHull(cookingSphere3D(16))
	if err != nil {
		t.Fatal(err)
	}
	expected := slices.Clone(hull.Edges)
	hull.Edges = nil
	compound, err := NewCompound([]CompoundChild{{Shape: hull}, {Shape: hull, Position: V3(2, 0, 0)}})
	if err != nil {
		t.Fatal(err)
	}
	for index, child := range compound.Children {
		if !slices.Equal(child.Shape.Edges, expected) {
			t.Fatalf("child %d has incorrect edges after cooking", index)
		}
	}
}
