package physics

import "testing"

func TestTriangleMeshCooksBVHAndDropsDegenerateTriangles(t *testing.T) {
	mesh, err := NewTriangleMesh(
		[]Vector3D{V3(-1, 0, -1), V3(-1, 0, 1), V3(1, 0, 1), V3(1, 0, -1)},
		[]uint32{0, 1, 2, 0, 2, 3, 0, 0, 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	if mesh.TriangleCount() != 2 || len(mesh.BVH) == 0 || !mesh.Cooked() {
		t.Fatalf("unexpected cooked mesh: triangles=%d nodes=%d cooked=%v", mesh.TriangleCount(), len(mesh.BVH), mesh.Cooked())
	}
	if mesh.Bounds.Min != (Vector3D{X: -1, Y: 0, Z: -1}) || mesh.Bounds.Max != (Vector3D{X: 1, Y: 0, Z: 1}) {
		t.Fatalf("unexpected bounds: %+v", mesh.Bounds)
	}
}

func TestTriangleMeshBVHAllocatesExactlyAndIndexesEveryTriangle(t *testing.T) {
	const grid = 24
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
	mesh, err := NewTriangleMesh(vertices, indices)
	if err != nil {
		t.Fatal(err)
	}
	if mesh.TriangleCount() <= triangleMeshLeafSize {
		t.Fatalf("fixture is too small to exercise the split: %d triangles", mesh.TriangleCount())
	}
	if len(mesh.BVH) != triangleMeshNodeCount(mesh.TriangleCount()) {
		t.Fatalf("BVH holds %d nodes, the balanced split needs %d", len(mesh.BVH), triangleMeshNodeCount(mesh.TriangleCount()))
	}
	if cap(mesh.BVH) != len(mesh.BVH) {
		t.Fatalf("BVH keeps %d nodes of growth slack", cap(mesh.BVH)-len(mesh.BVH))
	}
	seen := make([]int, mesh.TriangleCount())
	var walk func(node int32)
	walk = func(node int32) {
		entry := mesh.BVH[node]
		if entry.Count > 0 {
			for _, triangle := range mesh.TriangleIndices[entry.First : entry.First+entry.Count] {
				seen[triangle]++
			}
			return
		}
		walk(entry.Left)
		walk(entry.Right)
	}
	walk(0)
	for index, count := range seen {
		if count != 1 {
			t.Fatalf("triangle %d appears %d times in the BVH leaves", index, count)
		}
	}
}

func TestQuickHullBuildsCubeFaces(t *testing.T) {
	hull, err := NewConvexHull([]Vector3D{
		V3(-1, -1, -1), V3(-1, -1, 1), V3(-1, 1, -1), V3(-1, 1, 1),
		V3(1, -1, -1), V3(1, -1, 1), V3(1, 1, -1), V3(1, 1, 1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if hull.VertexCount() != 8 || hull.FaceCount() == 0 || !hull.Bounds.Valid() {
		t.Fatalf("unexpected hull: vertices=%d faces=%d bounds=%+v", hull.VertexCount(), hull.FaceCount(), hull.Bounds)
	}
	for _, face := range hull.Faces {
		if face.Normal.LengthSquared() < .9 {
			t.Fatalf("face normal was not normalized: %+v", face.Normal)
		}
		anchor := hull.Vertices[face.Indices[0]]
		for _, point := range hull.Vertices {
			if face.Normal.Dot(point.Sub(anchor)) > 1e-9 {
				t.Fatalf("face is not outward oriented: face=%+v point=%+v", face, point)
			}
		}
	}
}

func TestSphereSettlesOnTriangleMesh(t *testing.T) {
	mesh, err := NewTriangleMesh(
		[]Vector3D{V3(-10, 0, -10), V3(-10, 0, 10), V3(10, 0, 10), V3(10, 0, -10)},
		[]uint32{0, 1, 2, 0, 2, 3},
	)
	if err != nil {
		t.Fatal(err)
	}
	world := NewWorld3D()
	floor := world.CreateBody(NewBodyDef3D(BodyStatic))
	AddCollider3D(world, floor, NewColliderDef(mesh))
	def := NewBodyDef3D(BodyDynamic)
	def.Position = V3(0, 3, 0)
	ball := world.CreateBody(def)
	AddCollider3D(world, ball, NewColliderDef(Sphere{Radius: 1}))
	for range 120 {
		world.Step(1.0 / 60)
	}
	if y := ball.Position().Y; y < .9 || y > 1.2 {
		t.Fatalf("sphere did not settle on mesh: y=%f", y)
	}
}

func TestSphereSettlesOnConvexHull(t *testing.T) {
	hull, err := NewConvexHull([]Vector3D{
		V3(-10, 0, -10), V3(-10, 0, 10), V3(10, 0, 10), V3(10, 0, -10),
		V3(-10, 2, -10), V3(-10, 2, 10), V3(10, 2, 10), V3(10, 2, -10),
	})
	if err != nil {
		t.Fatal(err)
	}
	world := NewWorld3D()
	floor := world.CreateBody(NewBodyDef3D(BodyStatic))
	AddCollider3D(world, floor, NewColliderDef(hull))
	def := NewBodyDef3D(BodyDynamic)
	def.Position = V3(0, 4, 0)
	ball := world.CreateBody(def)
	AddCollider3D(world, ball, NewColliderDef(Sphere{Radius: 1}))
	for range 120 {
		world.Step(1.0 / 60)
	}
	if y := ball.Position().Y; y < 2.9 || y > 3.2 {
		t.Fatalf("sphere did not settle on convex hull: y=%f", y)
	}
}

func TestConvexHullsUseNarrowPhaseInsteadOfOnlyAABB(t *testing.T) {
	hull, err := NewConvexHull([]Vector3D{
		V3(-1, -1, -1), V3(-1, -1, 1), V3(-1, 1, -1), V3(-1, 1, 1),
		V3(1, -1, -1), V3(1, -1, 1), V3(1, 1, -1), V3(1, 1, 1),
	})
	if err != nil {
		t.Fatal(err)
	}
	world := NewWorld3D()
	first := world.CreateBody(NewBodyDef3D(BodyStatic))
	secondDef := NewBodyDef3D(BodyDynamic)
	secondDef.Position = V3(1.5, 0, 0)
	second := world.CreateBody(secondDef)
	AddCollider3D(world, first, NewColliderDef(hull))
	AddCollider3D(world, second, NewColliderDef(hull))
	world.Step(1.0 / 60)
	if len(world.Events()) == 0 {
		t.Fatal("overlapping convex hulls did not produce a contact")
	}
}

func TestCompoundStoresCookedConvexPieces(t *testing.T) {
	hull, err := NewConvexHull([]Vector3D{
		V3(-.5, -.5, -.5), V3(-.5, -.5, .5), V3(-.5, .5, -.5), V3(-.5, .5, .5),
		V3(.5, -.5, -.5), V3(.5, -.5, .5), V3(.5, .5, -.5), V3(.5, .5, .5),
	})
	if err != nil {
		t.Fatal(err)
	}
	compound, err := NewCompound([]CompoundChild{{Shape: hull}, {Shape: hull, Position: V3(2, 0, 0)}})
	if err != nil {
		t.Fatal(err)
	}
	if compound.ChildCount() != 2 || compound.Bounds.Max.X < 2.4 {
		t.Fatalf("unexpected compound: children=%d bounds=%+v", compound.ChildCount(), compound.Bounds)
	}
}

func TestCookCollisionSupportsImporterModes(t *testing.T) {
	vertices := []Vector3D{
		V3(-1, -1, -1), V3(-1, -1, 1), V3(-1, 1, -1), V3(-1, 1, 1),
		V3(1, -1, -1), V3(1, -1, 1), V3(1, 1, -1), V3(1, 1, 1),
	}
	indices := []uint32{0, 1, 3, 0, 3, 2, 4, 6, 7, 4, 7, 5, 0, 4, 5, 0, 5, 1, 2, 3, 7, 2, 7, 6, 0, 2, 6, 0, 6, 4, 1, 5, 7, 1, 7, 3}
	for _, mode := range []CollisionMode{CollisionModeTriangleMesh, CollisionModeSingleConvex, CollisionModeConvexDecomposition} {
		cooked, err := CookCollision(vertices, indices, mode)
		if err != nil {
			t.Fatalf("mode %q: %v", mode, err)
		}
		if cooked.Mode != mode {
			t.Fatalf("mode %q was normalized to %q", mode, cooked.Mode)
		}
	}
}

func TestCookedHullStepDoesNotAllocateAfterReserve(t *testing.T) {
	hull, err := NewConvexHull([]Vector3D{
		V3(-1, -1, -1), V3(-1, -1, 1), V3(-1, 1, -1), V3(-1, 1, 1),
		V3(1, -1, -1), V3(1, -1, 1), V3(1, 1, -1), V3(1, 1, 1),
	})
	if err != nil {
		t.Fatal(err)
	}
	world := NewWorld3D()
	world.Reserve(2, 2)
	first := world.CreateBody(NewBodyDef3D(BodyStatic))
	secondDef := NewBodyDef3D(BodyDynamic)
	secondDef.Position = V3(1.5, 0, 0)
	second := world.CreateBody(secondDef)
	AddCollider3D(world, first, NewColliderDef(hull))
	AddCollider3D(world, second, NewColliderDef(hull))
	world.Step(1.0 / 60)
	if allocations := testing.AllocsPerRun(100, func() { world.Step(1.0 / 60) }); allocations != 0 {
		t.Fatalf("cooked hull step allocated %f times", allocations)
	}
}
