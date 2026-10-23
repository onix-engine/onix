package physics

import (
	"math"
	"testing"
)

func queryTestMesh(t testing.TB) TriangleMesh {
	t.Helper()
	mesh, err := NewTriangleMesh([]Vector3D{V3(-4, 0, -4), V3(-4, 0, 4), V3(4, 0, 4), V3(4, 0, -4), V3(1, 3, -4), V3(1, 3, 4), V3(1, 0, 4), V3(1, 0, -4)}, []uint32{0, 1, 2, 0, 2, 3, 4, 5, 6, 4, 6, 7})
	if err != nil {
		t.Fatal(err)
	}
	return mesh
}

func TestMeshRaycastAndCapsuleQueries(t *testing.T) {
	mesh := queryTestMesh(t)
	hit, ok := mesh.Raycast(V3(0, 2, 0), V3(0, -1, 0), 10)
	if !ok || hit.Distance != 2 || hit.Normal != V3(0, 1, 0) {
		t.Fatalf("floor ray: %+v %v", hit, ok)
	}
	if _, ok := mesh.Raycast(V3(0, 2, 0), V3(0, -1, 0), 1); ok {
		t.Fatal("ray exceeded its limit")
	}
	if _, ok := mesh.Raycast(V3(5, 2, 0), V3(0, -1, 0), 10); ok {
		t.Fatal("ray hit empty mesh bounds")
	}
	if mesh.OverlapsCapsule(V3(0, .4, 0), V3(0, 1.4, 0), .3) {
		t.Fatal("capsule in free space hit mesh bounds")
	}
	if !mesh.OverlapsCapsule(V3(.8, .4, 0), V3(.8, 1.4, 0), .3) {
		t.Fatal("capsule missed wall")
	}
	if !mesh.OverlapsCapsule(V3(0, .2, 0), V3(0, 1.2, 0), .3) {
		t.Fatal("capsule missed floor")
	}
	if !mesh.OverlapsCapsule(V3(4.1, .1, 4.1), V3(4.1, 1, 4.1), .3) {
		t.Fatal("capsule missed triangle vertex")
	}
	allocs := testing.AllocsPerRun(100, func() {
		mesh.Raycast(V3(0, 2, 0), V3(0, -1, 0), 10)
		mesh.OverlapsCapsule(V3(.8, .4, 0), V3(.8, 1.4, 0), .3)
	})
	if allocs != 0 {
		t.Fatalf("query allocations: %g", allocs)
	}
}

func TestCapsuleSettlesOnTransformedTriangleMesh(t *testing.T) {
	mesh := queryTestMesh(t)
	world := NewWorld3D()
	ground := NewBodyDef3D(BodyStatic)
	ground.Position = V3(3, 2, -5)
	ground.Rotation = QuaternionFromAxisAngle(V3(0, 1, 0), .6)
	groundBody := world.CreateBody(ground)
	AddCollider3D(world, groundBody, NewColliderDef(mesh))
	def := NewBodyDef3D(BodyDynamic)
	def.Position = V3(3, 4, -5)
	def.FixedRotation = true
	body := world.CreateBody(def)
	AddCollider3D(world, body, NewColliderDef(NewCapsule(1.75, .34)))
	for range 360 {
		world.Step(1.0 / 120)
	}
	if math.Abs(body.Position().Y-2.875) > .04 {
		t.Fatalf("capsule did not settle on triangles: %+v", body.Position())
	}
}
