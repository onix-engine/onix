package project

import (
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/scene/render3d"
)

type preparationMaterial2D struct{ shader *graphics.Material2DShader }

func (m preparationMaterial2D) Material2D() graphics.Material2DData {
	return graphics.Material2DData{Shader: m.shader}
}

type preparationMaterial3D struct{ data graphics.Material3DData }

func (m preparationMaterial3D) Material3D() graphics.Material3DData { return m.data }

type materialPreloadDriver struct {
	preloadDriver
	meshes, shaders2D, shaders3D uint32
}

func (d *materialPreloadDriver) CreateMesh([]render.Vertex3D, []uint32) (render.MeshHandle, error) {
	d.meshes++
	return render.NewHandle[render.MeshResource](d.meshes, 1), nil
}
func (*materialPreloadDriver) DestroyMesh(render.MeshHandle) error { return nil }
func (*materialPreloadDriver) DrawMeshes(render.MeshBatch3D) error { return nil }
func (d *materialPreloadDriver) CreateMaterial2DShader(render.ShaderDescriptor) (render.Material2DShaderHandle, error) {
	d.shaders2D++
	return render.NewHandle[render.Material2DShaderResource](d.shaders2D, 1), nil
}
func (*materialPreloadDriver) DestroyMaterial2DShader(render.Material2DShaderHandle) error {
	return nil
}
func (d *materialPreloadDriver) CreateMeshShader(render.ShaderDescriptor) (render.MeshShaderHandle, error) {
	d.shaders3D++
	return render.NewHandle[render.MeshShaderResource](d.shaders3D, 1), nil
}
func (*materialPreloadDriver) DestroyMeshShader(render.MeshShaderHandle) error { return nil }

func TestQueueEntityUploadsUsesRegisteredGenericMaterialsAndSubmeshes(t *testing.T) {
	w := ecs.NewWorld()
	scene.RegisterMaterial2D[preparationMaterial2D](w)
	render3d.RegisterMaterial[preparationMaterial3D](w)
	shader2D, err := graphics.NewMaterial2DShader(render.ShaderDescriptor{WebGL2FragmentSource: "fragment"})
	if err != nil {
		t.Fatal(err)
	}
	defer shader2D.Dispose()
	shader3D, err := graphics.NewMeshShader(render.ShaderDescriptor{WebGL2FragmentSource: "fragment"})
	if err != nil {
		t.Fatal(err)
	}
	defer shader3D.Dispose()
	first, second := graphics.CreateTextureByColor(graphics.White), graphics.CreateTextureByColor(graphics.Black)
	defer first.Dispose()
	defer second.Dispose()
	mesh := graphics.CubeMesh(1)
	mesh.Submeshes = []graphics.Submesh{{Material: 0}}
	defer mesh.Dispose()
	defaultMaterial := preparationMaterial3D{graphics.Material3DData{Shader: shader3D, BaseColorTexture: first}}
	components := ecs.NewBundle(3)
	ecs.Put(&components, scene.MeshMaterial2D[preparationMaterial2D]{Material: &preparationMaterial2D{shader2D}})
	ecs.Put(&components, render3d.Material[preparationMaterial3D]{Material: &defaultMaterial, Materials: []preparationMaterial3D{{graphics.Material3DData{Shader: shader3D, NormalTexture: second}}}})
	ecs.Put(&components, render3d.Mesh{Mesh: mesh})
	entity := w.Spawn(components)
	var queue graphics.UploadQueue
	QueueEntityUploads(&queue, w, entity)
	if queue.Remaining() != 5 {
		t.Fatalf("queued resources=%d want=5", queue.Remaining())
	}
	if allocations := testing.AllocsPerRun(100, func() { QueueEntityUploads(&queue, w, entity) }); allocations != 0 {
		t.Fatalf("repeated resource discovery allocated %g times", allocations)
	}
	driver := &materialPreloadDriver{}
	device, _ := render.NewDevice(driver)
	defer device.Destroy()
	if ready, err := queue.Advance(device, graphics.UploadBudget{Time: time.Second}); !ready || err != nil {
		t.Fatal("generic material upload failed", err)
	}
	if driver.uploads.Load() != 2 || driver.shaders2D != 1 || driver.shaders3D != 1 || driver.meshes != 1 {
		t.Fatal("material textures, shaders or mesh were omitted")
	}
}
