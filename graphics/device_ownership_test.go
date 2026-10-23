package graphics

import (
	"image"
	"testing"

	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

func TestResourcesKeepResidencyAndDisposeOnTheirDevices(t *testing.T) {
	render.Uninstall()
	firstDriver, secondDriver := &meshLifecycleDriver{}, &meshLifecycleDriver{}
	first, err := render.NewDevice(firstDriver)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Destroy()
	second, err := render.NewDevice(secondDriver)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Destroy()
	first.Activate()
	texture := CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 2, 2)))
	mesh := CubeMesh(1)
	firstTexture, firstMesh := texture.RenderHandle(), mesh.RenderHandle()
	second.Activate()
	texture.RenderHandle()
	mesh.RenderHandle()
	if allocations := testing.AllocsPerRun(100, func() {
		first.Activate()
		if texture.RenderHandle() != firstTexture || mesh.RenderHandle() != firstMesh {
			t.Fatal("first residency lost")
		}
		second.Activate()
		texture.RenderHandle()
		mesh.RenderHandle()
	}); allocations != 0 {
		t.Fatalf("device switch allocations=%v", allocations)
	}
	if firstDriver.next != 1 || secondDriver.next != 1 || firstDriver.created != 1 || secondDriver.created != 1 {
		t.Fatal("device switches reuploaded resources")
	}
	texture.UpdateRegions([]TextureRegionUpdate{{Width: 1, Height: 1, Pixels: []byte{1, 2, 3, 4}}})
	if firstDriver.updateCalls != 1 || secondDriver.updateCalls != 1 {
		t.Fatal("texture update did not reach both residencies")
	}
	if err := texture.Dispose(); err != nil {
		t.Fatal(err)
	}
	if err := mesh.Dispose(); err != nil {
		t.Fatal(err)
	}
	if firstDriver.destroyedTextures != 1 || secondDriver.destroyedTextures != 1 || firstDriver.destroyed != 1 || secondDriver.destroyed != 1 {
		t.Fatal("dispose lost an inactive device's handles")
	}
	second.Destroy()
	if first.Driver() != firstDriver {
		t.Fatal("second device destruction destroyed first device")
	}
}

func TestTextureUpdateCreatesNewResidencyAndUpdatesExistingOne(t *testing.T) {
	firstDriver, secondDriver := &textureTestDriver{}, &textureTestDriver{}
	first, err := render.NewDevice(firstDriver)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Destroy()
	second, err := render.NewDevice(secondDriver)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Destroy()
	first.Activate()
	texture := CreateEmptyTexture(8, 8, render.TextureFormatR8, ResourceUsageDefault)
	defer texture.Dispose()
	texture.RenderHandle()
	second.Activate()
	texture.UpdateRegions([]TextureRegionUpdate{{X: 3, Y: 2, Width: 1, Height: 1, Pixels: []byte{173}}})
	if firstDriver.updateCalls != 1 || secondDriver.updateCalls != 0 || secondDriver.next != 1 {
		t.Fatal("update lost an existing residency or redundantly patched the newly uploaded one")
	}
	if secondDriver.createdPixels[2*8+3] != 173 {
		t.Fatal("new residency lost the region update")
	}
	texture.RenderHandleFor(first)
	texture.UpdateRegions([]TextureRegionUpdate{{Width: 1, Height: 1, Pixels: []byte{231}}})
	if firstDriver.updateCalls != 2 || secondDriver.updateCalls != 1 {
		t.Fatal("update lost an existing residency after using a different device's handle")
	}
}

func TestCanvasUsesOwningDeviceAfterAnotherIsActivated(t *testing.T) {
	render.Uninstall()
	firstDriver, secondDriver := &canvasTestDriver{viewport: render.Rect{Width: 20, Height: 20}}, &canvasTestDriver{}
	first, _ := render.NewDevice(firstDriver)
	second, _ := render.NewDevice(secondDriver)
	defer first.Destroy()
	defer second.Destroy()
	first.Activate()
	canvas := NewCanvas2D(first, 20, 20)
	texture := CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	shader, err := NewMaterial2DShader(render.ShaderDescriptor{WebGL2FragmentSource: "fragment"})
	if err != nil {
		t.Fatal(err)
	}
	second.Activate()
	canvas.BindMaterial2D(shader)
	canvas.BindTexture(texture)
	canvas.DrawMatrices(mgl32.Ident4(), mgl32.Ident3(), 1)
	if err := canvas.FinishFrame(); err != nil {
		t.Fatal(err)
	}
	if len(firstDriver.spriteBatches) != 1 || len(secondDriver.spriteBatches) != 0 || secondDriver.nextShaderID != 0 {
		t.Fatal("canvas submitted work to another device")
	}
	if err := shader.Dispose(); err != nil {
		t.Fatal(err)
	}
	if err := texture.Dispose(); err != nil {
		t.Fatal(err)
	}
}

func TestGPUOnlyAndExternalTexturesStayOnOwningDevice(t *testing.T) {
	firstDriver, secondDriver := &textureTestDriver{}, &textureTestDriver{}
	first, _ := render.NewDevice(firstDriver)
	second, _ := render.NewDevice(secondDriver)
	defer first.Destroy()
	defer second.Destroy()
	first.Activate()
	texture := CreateTextureByImageWithUsage(image.NewRGBA(image.Rect(0, 0, 2, 2)), false, ResourceUsageGPU)
	external, err := CreateTextureFromExternal(render.ExternalTextureDescriptor{Kind: render.ExternalTextureD3D11, Handle: 1, Width: 2, Height: 2, Format: render.ExternalTextureRGBA8})
	if err != nil {
		t.Fatal(err)
	}
	second.Activate()
	if texture.RenderHandle().Valid() || texture.UploadError() == nil || external.RenderHandle().Valid() {
		t.Fatal("GPU-only texture used on foreign device")
	}
	texture.UpdateRegions([]TextureRegionUpdate{{Width: 1, Height: 1, Pixels: []byte{1, 2, 3, 4}}})
	if texture.pix != nil || secondDriver.next != 0 {
		t.Fatal("partial update manufactured a foreign texture")
	}
	if err := external.RefreshExternal(); err != nil {
		t.Fatal(err)
	}
	if firstDriver.externalRefreshes != 1 || secondDriver.externalRefreshes != 0 {
		t.Fatal("external refresh used active device")
	}
	first.Activate()
	if !texture.Valid() || !external.Valid() {
		t.Fatal("foreign device check invalidated owner's texture")
	}
	second.Destroy()
	if !texture.Valid() {
		t.Fatal("destroying other device invalidated texture")
	}
	if err := texture.Dispose(); err != nil {
		t.Fatal(err)
	}
	if err := external.Dispose(); err != nil {
		t.Fatal(err)
	}
	if firstDriver.destroyedTextures != 2 {
		t.Fatal("texture disposal lost owner")
	}
}
