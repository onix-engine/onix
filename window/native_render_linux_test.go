//go:build linux && cgo && !android && !onix_drm

package window

import (
	"os"
	"runtime"
	"testing"

	"github.com/diiyw/onix/render"
)

func TestNativeVulkanFrameDoesNotAllocate(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("requires an X display; run under xvfb-run")
	}
	context := NewContext(64, 64, "cgo allocation test", Options{})
	defer runtime.UnlockOSThread()
	defer context.Destroy()
	driver := context.Device.Driver()
	if driver.RendererInfo().Backend != "Vulkan" {
		t.Skip("requires Vulkan")
	}
	rgba := []byte{255, 0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255}
	mask := []byte{255, 255, 255}
	texture, err := driver.CreateTexture(render.TextureDescriptor{Width: 3, Height: 1}, rgba)
	if err != nil {
		t.Fatal(err)
	}
	r8, err := driver.CreateTexture(render.TextureDescriptor{Width: 3, Height: 1, Format: render.TextureFormatR8}, mask)
	if err != nil {
		t.Fatal(err)
	}
	updates := []render.TextureUpdate{
		{Texture: r8, Region: render.TextureRegion{Width: 3, Height: 1}, Pixels: mask},
		{Texture: texture, Region: render.TextureRegion{Width: 3, Height: 1}, Pixels: rgba},
	}
	targets := driver.(render.RenderTargetDriver)
	target, _, err := targets.CreateRenderTarget(render.RenderTargetDescriptor{Width: 64, Height: 64, Depth: true})
	if err != nil {
		t.Fatal(err)
	}
	identity := [16]float32{0: 1, 5: 1, 10: 1, 15: 1}
	batch := render.MeshBatch3D{
		ViewProjection: identity,
		Vertices: []render.Vertex3D{
			{Position: [3]float32{-1, -1, .5}},
			{Position: [3]float32{1, -1, .5}},
			{Position: [3]float32{0, 1, .5}},
		},
		Indices:   []uint32{0, 1, 2},
		Instances: []render.Instance3D{{Model: identity, BaseColor: [4]float32{1, 1, 1, 1}}},
		Draws:     []render.MeshDraw3D{{Texture: texture, IndexCount: 3, InstanceCount: 1}},
	}
	meshes := driver.(render.MeshDriver)
	buffers := driver.(interface {
		BufferMemoryStatistics() render.BufferMemoryStatistics
	})
	clear := render.RenderTargetClear{Color: render.Color{A: 1}, ClearColor: true, ClearDepth: true}
	frame := func() {
		if err := driver.BeginFrame(clear.Color); err != nil {
			t.Fatal(err)
		}
		if err := driver.UpdateTextures(updates[:1]); err != nil {
			t.Fatal(err)
		}
		if err := driver.UpdateTextures(updates); err != nil {
			t.Fatal(err)
		}
		if err := targets.BeginRenderTarget(target, clear); err != nil {
			t.Fatal(err)
		}
		if err := meshes.DrawMeshes(batch); err != nil {
			t.Fatal(err)
		}
		if err := targets.EndRenderTarget(); err != nil {
			t.Fatal(err)
		}
		if err := driver.Present(); err != nil {
			t.Fatal(err)
		}
		if memory := buffers.BufferMemoryStatistics(); memory.InstanceBytes == 0 || memory.StagingBytes == 0 {
			t.Fatalf("missing native buffer capacity: %+v", memory)
		}
	}
	for range 8 {
		frame()
	}
	if allocations := testing.AllocsPerRun(10, frame); allocations != 0 {
		t.Fatalf("Vulkan frame allocations = %v, want 0", allocations)
	}
	statistics := context.Device.Statistics()
	if statistics.TextureUploads != 3 || statistics.TextureUploadBytes != 18 {
		t.Fatalf("texture statistics include padding or miss updates: %+v", statistics)
	}
}
