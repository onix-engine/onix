package graphics

import (
	"errors"
	"image"
	"testing"
	"time"

	"github.com/diiyw/onix/render"
)

func TestUploadQueueDeduplicatesAndBudgetsExplicitDevice(t *testing.T) {
	firstDriver, secondDriver := &textureTestDriver{}, &textureTestDriver{}
	first, _ := render.NewDevice(firstDriver)
	second, _ := render.NewDevice(secondDriver)
	defer first.Destroy()
	defer second.Destroy()
	second.Activate()
	var queue UploadQueue
	for range 3 {
		texture := CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 2, 2)))
		defer texture.Dispose()
		queue.AddTexture(texture)
		queue.AddTexture(NewSubTexture(texture, 1, 1, 0, 0, .5, .5, 0))
	}
	if firstDriver.next != 0 || secondDriver.next != 0 || queue.Remaining() != 3 {
		t.Fatal("construction uploaded assets or queue did not deduplicate atlas views")
	}
	if ready, err := queue.Advance(nil, UploadBudget{Time: time.Second}); ready || err != nil || queue.Remaining() != 3 {
		t.Fatal("headless queue lost pending uploads")
	}
	// One texture is 16 bytes. Permit the oversized first unit, then stop.
	if ready, err := queue.Advance(first, UploadBudget{Time: time.Second, Bytes: 8}); ready || err != nil || firstDriver.next != 1 {
		t.Fatalf("oversized first unit: ready=%v error=%v uploads=%d", ready, err, firstDriver.next)
	}
	if ready, err := queue.Advance(first, UploadBudget{Time: time.Second, Resources: 1}); ready || err != nil || firstDriver.next != 2 {
		t.Fatal("resource budget was not respected")
	}
	if ready, err := queue.Advance(first, UploadBudget{}); !ready || err != nil || !queue.ReadyFor(first) || secondDriver.next != 0 {
		t.Fatal("upload used the active device instead of the explicit device")
	}
	if queue.ReadyFor(second) {
		t.Fatal("foreign device reported ready")
	}
	if ready, err := queue.Advance(second, UploadBudget{}); ready || err != nil || queue.Remaining() != 2 {
		t.Fatal("device switch did not restart budgeted uploads")
	}
	if ready, err := queue.Advance(second, UploadBudget{Time: time.Second, Bytes: 32}); !ready || err != nil {
		t.Fatal("exact byte budget failed", err)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if ready, err := queue.Advance(second, UploadBudget{}); !ready || err != nil {
			panic("ready queue changed")
		}
	}); allocations != 0 {
		t.Fatalf("ready upload queue allocated %g times", allocations)
	}
	second.Destroy()
	if queue.ReadyFor(second) {
		t.Fatal("destroyed device reported ready")
	}
	if ready, err := queue.Advance(second, UploadBudget{}); ready || err != nil {
		t.Fatal("advance accepted destroyed device residency", err)
	}
	queue.Reset()
	if queue.Remaining() != 0 || len(queue.seen) != 0 {
		t.Fatal("reset retained uploads")
	}
}

type failedUploadDriver struct {
	textureTestDriver
	err error
}

func (d *failedUploadDriver) CreateTexture(render.TextureDescriptor, []byte) (render.TextureHandle, error) {
	return render.TextureHandle{}, d.err
}

func TestUploadQueuePreservesFailureAndCanRetry(t *testing.T) {
	cause := errors.New("device out of memory")
	device, _ := render.NewDevice(&failedUploadDriver{err: cause})
	defer device.Destroy()
	texture := CreateTextureByColor(White)
	defer texture.Dispose()
	var queue UploadQueue
	queue.AddTexture(texture)
	if ready, err := queue.Advance(device, UploadBudget{}); ready || !errors.Is(err, cause) || !errors.Is(err, ErrUploadUnavailable) || queue.Remaining() != 1 {
		t.Fatalf("failed upload: ready=%v error=%v pending=%d", ready, err, queue.Remaining())
	}
	retry, _ := render.NewDevice(&textureTestDriver{})
	defer retry.Destroy()
	if ready, err := queue.Advance(retry, UploadBudget{}); !ready || err != nil {
		t.Fatal("CPU payload did not survive failed upload", err)
	}
}

func TestCPUPreparationDoesNotInvokeActiveDriver(t *testing.T) {
	driver := &uploadTestDriver{}
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	defer render.Uninstall()
	texture := CreateEmptyTexture(2, 2, render.TextureFormatR8, ResourceUsageCPU|ResourceUsageGPU)
	defer texture.Dispose()
	texture.UpdateRegions([]TextureRegionUpdate{{Width: 1, Height: 1, Pixels: []byte{17}}})
	shader, err := NewMaterial2DShader(render.ShaderDescriptor{WebGL2FragmentSource: "fragment"})
	if err != nil {
		t.Fatal(err)
	}
	defer shader.Dispose()
	meshShader, err := NewMeshShader(render.ShaderDescriptor{WebGL2FragmentSource: "fragment"})
	if err != nil {
		t.Fatal(err)
	}
	defer meshShader.Dispose()
	mesh := CubeMesh(1)
	defer mesh.Dispose()
	if !texture.Valid() || driver.nextID != 0 || driver.nextShaderID != 0 || driver.meshes != 0 || driver.meshShaders != 0 {
		t.Fatal("CPU preparation called the active render driver")
	}
	var queue UploadQueue
	queue.AddTexture(texture)
	queue.AddMaterial2DShader(shader)
	queue.AddMeshShader(meshShader)
	queue.AddMesh(mesh)
	if ready, err := queue.Advance(render.ActiveDevice(), UploadBudget{Time: time.Second}); !ready || err != nil {
		t.Fatal("prepared resource upload failed", err)
	}
}

type uploadTestDriver struct {
	canvasTestDriver
	meshes, meshShaders uint32
}

func (d *uploadTestDriver) CreateMesh([]render.Vertex3D, []uint32) (render.MeshHandle, error) {
	d.meshes++
	return render.NewHandle[render.MeshResource](d.meshes, 1), nil
}
func (d *uploadTestDriver) DestroyMesh(render.MeshHandle) error { return nil }
func (d *uploadTestDriver) DrawMeshes(render.MeshBatch3D) error { return nil }
func (d *uploadTestDriver) CreateMeshShader(render.ShaderDescriptor) (render.MeshShaderHandle, error) {
	d.meshShaders++
	return render.NewHandle[render.MeshShaderResource](d.meshShaders, 1), nil
}
func (d *uploadTestDriver) DestroyMeshShader(render.MeshShaderHandle) error { return nil }
