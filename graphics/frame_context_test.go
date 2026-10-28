package graphics

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/render"
	"testing"
	"time"
)

func TestFrameContextUploadsStayWithOwningWorld(t *testing.T) {
	a, b := &meshLifecycleDriver{}, &meshLifecycleDriver{}
	first, err := render.NewDevice(a)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Destroy()
	second, err := render.NewDevice(b)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Destroy()
	wa, wb := ecs.NewWorld(), ecs.NewWorld()
	ca, cb := NewCanvas2D(first, 100, 60), NewCanvas2D(second, 200, 80)
	fa, fb := BindFrame(wa, ca), BindFrame(wb, cb)
	texture := CreateTextureByColor(ColorRGBA{R: .13, G: .27, B: .51, A: 1})
	defer texture.Dispose()
	fa.Uploads.AddTexture(texture)
	fb.Uploads.AddTexture(texture)
	second.Activate()
	defer SetCurrentContextForTesting(cb)()
	if ready, err := fa.AdvanceUploads(UploadBudget{Time: time.Second}); !ready || err != nil || a.next != 1 || b.next != 0 {
		t.Fatal("first-world upload leaked to global device", err)
	}
	first.Activate()
	if ready, err := fb.AdvanceUploads(UploadBudget{Time: time.Second}); !ready || err != nil || a.next != 1 || b.next != 1 {
		t.Fatal("second-world upload leaked to global device", err)
	}
	if DeviceFor(wa) != first || CanvasFor(wa) != ca || DeviceFor(wb) != second || CanvasFor(wb) != cb || CanvasFor(ecs.NewWorld()) != nil {
		t.Fatal("world frames share an implicit destination")
	}
	if n := testing.AllocsPerRun(100, func() { BindFrame(wa, ca); fa.AdvanceUploads(UploadBudget{Time: time.Second}) }); n != 0 {
		t.Fatalf("warm frame binding/upload allocations=%v", n)
	}
	// Rebind retains queued CPU assets and lets the upload queue realize them on
	// the new device instead of dropping work during window replacement.
	BindFrame(wa, cb)
	if ready, err := fa.AdvanceUploads(UploadBudget{Time: time.Second}); !ready || err != nil || b.next != 1 {
		t.Fatal("rebind lost upload ownership", err)
	}
}

func TestGPUOnlyTextureRegionBeforeFirstExplicitUpload(t *testing.T) {
	driver := &textureTestDriver{}
	device, err := render.NewDevice(driver)
	if err != nil {
		t.Fatal(err)
	}
	defer device.Destroy()
	texture := CreateEmptyTexture(2, 2, render.TextureFormatR8, ResourceUsageGPU)
	defer texture.Dispose()
	texture.UpdateRegions([]TextureRegionUpdate{{X: 1, Y: 1, Width: 1, Height: 1, Pixels: []byte{173}}})
	if driver.next != 0 {
		t.Fatal("CPU patch implicitly created GPU residency")
	}
	if !texture.RenderHandleFor(device).Valid() || len(driver.createdPixels) != 4 || driver.createdPixels[3] != 173 {
		t.Fatal("first explicit upload lost pending GPU-only texture patch")
	}
}
