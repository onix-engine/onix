package graphics

import (
	"fmt"
	"testing"

	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

func TestPrimitiveMeshesHaveValidBoundsAndIndices(t *testing.T) {
	for name, mesh := range map[string]*Mesh{
		"cube": CubeMesh(2), "plane": PlaneMesh(4), "sphere": UVSphereMesh(1, 16, 8),
	} {
		if mesh == nil || len(mesh.Vertices) == 0 || len(mesh.Indices) == 0 {
			t.Fatalf("%s mesh is empty", name)
		}
		if mesh.Bounds.Radius <= 0 {
			t.Fatalf("%s mesh has invalid bounds: %+v", name, mesh.Bounds)
		}
		for _, index := range mesh.Indices {
			if int(index) >= len(mesh.Vertices) {
				t.Fatalf("%s index %d exceeds vertex count %d", name, index, len(mesh.Vertices))
			}
		}
	}
}

func TestMeshDeformationReusesVerticesForMorphAndSkin(t *testing.T) {
	mesh, err := NewMesh([]render.Vertex3D{
		{Position: [3]float32{0, 0, 0}, Normal: [3]float32{0, 1, 0}},
		{Position: [3]float32{1, 0, 0}, Normal: [3]float32{0, 1, 0}},
		{Position: [3]float32{0, 0, 1}, Normal: [3]float32{0, 1, 0}},
	}, []uint32{0, 1, 2})
	if err != nil {
		t.Fatal(err)
	}
	mesh.MorphTargets = []MorphTarget{{PositionDelta: [][3]float32{{0, 1, 0}, {0, 1, 0}, {0, 1, 0}}}}
	mesh.Skin = []SkinVertex{
		{Joints: [4]uint32{0}, Weights: [4]float32{1}, Active: true},
		{Joints: [4]uint32{0}, Weights: [4]float32{1}, Active: true},
		{Joints: [4]uint32{0}, Weights: [4]float32{1}, Active: true},
	}
	joints := []mgl32.Mat4{mgl32.Translate3D(2, 0, 0)}
	vertices, err := mesh.DeformVertices(nil, []float32{0.5}, joints)
	if err != nil {
		t.Fatal(err)
	}
	if got := vertices[0].Position; got != [3]float32{2, 0.5, 0} {
		t.Fatalf("deformed position = %v", got)
	}
	allocations := testing.AllocsPerRun(100, func() {
		vertices, err = mesh.DeformVertices(vertices, []float32{0.5}, joints)
	})
	if err != nil || allocations != 0 {
		t.Fatalf("deformation allocations = %v, err=%v", allocations, err)
	}
}

func TestMeshGPUDeformationKeepsInactiveSkinVerticesUnskinned(t *testing.T) {
	mesh, err := NewMesh([]render.Vertex3D{
		{Position: [3]float32{0, 0, 0}},
		{Position: [3]float32{1, 0, 0}},
		{Position: [3]float32{0, 1, 0}},
	}, []uint32{0, 1, 2})
	if err != nil {
		t.Fatal(err)
	}
	mesh.Skin = []SkinVertex{
		{Joints: [4]uint32{0}, Weights: [4]float32{1}, Active: true},
		{Joints: [4]uint32{3}, Weights: [4]float32{1}},
		{Joints: [4]uint32{0}, Weights: [4]float32{1}, Active: true},
	}
	skin, _, err := mesh.GPUDeformationData()
	if err != nil {
		t.Fatal(err)
	}
	if skin[0].Weights[0] != 1 || skin[1] != (render.MeshSkinVertex{}) || skin[2].Weights[0] != 1 {
		t.Fatalf("GPU skin data = %+v", skin)
	}
	allocations := testing.AllocsPerRun(100, func() {
		skin, _, err = mesh.GPUDeformationData()
	})
	if err != nil || allocations != 0 {
		t.Fatalf("cached GPU deformation allocations = %v, err=%v", allocations, err)
	}
}

func TestNewMeshRejectsInvalidIndices(t *testing.T) {
	_, err := NewMesh(CubeMesh(1).Vertices[:3], []uint32{0, 1, 3})
	if err == nil {
		t.Fatal("expected an out-of-range index error")
	}
}

func TestNewMeshOwnedTakesSliceOwnership(t *testing.T) {
	vertices := CubeMesh(1).Vertices[:3]
	indices := []uint32{0, 1, 2}
	mesh, err := NewMeshOwned(vertices, indices)
	if err != nil {
		t.Fatal(err)
	}
	if &mesh.Vertices[0] != &vertices[0] || &mesh.Indices[0] != &indices[0] {
		t.Fatal("owned mesh copied geometry slices")
	}
}

func TestMeshUploadsLazilyOncePerDeviceAndDisposes(t *testing.T) {
	render.Uninstall()
	t.Cleanup(render.Uninstall)
	mesh := CubeMesh(1)
	first := &meshLifecycleDriver{}
	if err := render.Install(first); err != nil {
		t.Fatal(err)
	}
	firstHandle := mesh.RenderHandle()
	if !firstHandle.Valid() || mesh.UploadError() != nil || first.created != 1 {
		t.Fatalf("first upload = handle %s, error %v, creates %d", firstHandle, mesh.UploadError(), first.created)
	}
	if handle := mesh.RenderHandle(); handle != firstHandle || first.created != 1 {
		t.Fatalf("same-device upload repeated: handle %s, creates %d", handle, first.created)
	}

	second := &meshLifecycleDriver{}
	if err := render.Install(second); err != nil {
		t.Fatal(err)
	}
	secondHandle := mesh.RenderHandle()
	if !secondHandle.Valid() || mesh.UploadError() != nil || second.created != 1 {
		t.Fatalf("replacement-device upload = handle %s, error %v, creates %d", secondHandle, mesh.UploadError(), second.created)
	}
	if err := mesh.Dispose(); err != nil {
		t.Fatal(err)
	}
	if second.destroyed != 1 || mesh.RenderHandle().Valid() || mesh.UploadError() == nil {
		t.Fatalf("disposed mesh = destroys %d, handle %s, error %v", second.destroyed, mesh.RenderHandle(), mesh.UploadError())
	}
}

type meshLifecycleDriver struct {
	textureTestDriver
	created   int
	destroyed int
}

func (d *meshLifecycleDriver) CreateMesh([]render.Vertex3D, []uint32) (render.MeshHandle, error) {
	d.created++
	return render.NewHandle[render.MeshResource](uint32(d.created), 1), nil
}

func (d *meshLifecycleDriver) DestroyMesh(render.MeshHandle) error {
	d.destroyed++
	return nil
}

func (d *meshLifecycleDriver) DrawMeshes(render.MeshBatch3D) error { return nil }

type deformationLifecycleDriver struct {
	meshLifecycleDriver
	uploaded int
	fail     bool
}

func (d *deformationLifecycleDriver) SetMeshDeformation(_ render.MeshHandle, descriptor render.MeshDeformationDescriptor) error {
	d.uploaded++
	if d.fail {
		return fmt.Errorf("static upload failed")
	}
	return nil
}
func (d *deformationLifecycleDriver) DrawMeshesDeformed(render.MeshBatch3D) error { return nil }

func TestMeshStaticDeformationUploadsOncePerDeviceAndCleansFailedUploads(t *testing.T) {
	mesh := PlaneMesh(1)
	mesh.MorphTargets = []MorphTarget{{PositionDelta: make([][3]float32, len(mesh.Vertices))}}
	first, second := &deformationLifecycleDriver{}, &deformationLifecycleDriver{fail: true}
	firstDevice, err := render.NewDevice(first)
	if err != nil {
		t.Fatal(err)
	}
	defer firstDevice.Destroy()
	secondDevice, err := render.NewDevice(second)
	if err != nil {
		t.Fatal(err)
	}
	defer secondDevice.Destroy()
	handle := mesh.RenderHandleFor(firstDevice)
	if !handle.Valid() || first.uploaded != 1 {
		t.Fatalf("first static upload: %v %d", mesh.UploadError(), first.uploaded)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if mesh.RenderHandleFor(firstDevice) != handle {
			panic("mesh residency changed")
		}
	}); allocations != 0 || first.uploaded != 1 {
		t.Fatalf("stable residency allocates or reuploads: %v %d", allocations, first.uploaded)
	}
	if mesh.RenderHandleFor(secondDevice).Valid() || mesh.UploadError() == nil || second.destroyed != 1 {
		t.Fatal("failed upload leaked its mesh")
	}
	second.fail = false
	if !mesh.RenderHandleFor(secondDevice).Valid() || second.uploaded != 2 {
		t.Fatal("failed static upload did not recover")
	}
	if err := mesh.Dispose(); err != nil {
		t.Fatal(err)
	}
	if first.destroyed != 1 || second.destroyed != 2 {
		t.Fatalf("mesh disposal: %d/%d", first.destroyed, second.destroyed)
	}
}

func TestMeshStaticDeformationRejectsMalformedMorphOnRetry(t *testing.T) {
	mesh := PlaneMesh(1)
	mesh.MorphTargets = []MorphTarget{{PositionDelta: make([][3]float32, len(mesh.Vertices)-1)}}
	driver := &deformationLifecycleDriver{}
	device, err := render.NewDevice(driver)
	if err != nil {
		t.Fatal(err)
	}
	defer device.Destroy()
	defer mesh.Dispose()
	for range 2 {
		if mesh.RenderHandleFor(device).Valid() || mesh.UploadError() == nil {
			t.Fatal("malformed morph data accepted on retry")
		}
	}
	if driver.uploaded != 0 || driver.destroyed != 2 {
		t.Fatal("malformed morph data was uploaded or leaked geometry")
	}
	mesh.MorphTargets[0].PositionDelta = make([][3]float32, len(mesh.Vertices))
	if !mesh.RenderHandleFor(device).Valid() || driver.uploaded != 1 {
		t.Fatal("corrected morph data did not upload")
	}
}
