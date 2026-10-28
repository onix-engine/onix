package graphics

import (
	"bytes"
	"image"
	"testing"

	"github.com/diiyw/onix/render"
)

func TestEmptyTextureRestoresCompactShadow(t *testing.T) {
	for _, format := range []render.TextureFormat{render.TextureFormatR8, render.TextureFormatRGBA8} {
		t.Run(format.Name(), func(t *testing.T) {
			render.Uninstall()
			t.Cleanup(render.Uninstall)
			const size = 128
			texture := CreateEmptyTexture(size, size, format, ResourceUsageDefault)
			defer texture.Dispose()
			if !texture.Valid() || texture.RenderHandle().Valid() || cap(texture.pix) != 0 {
				t.Fatal("empty deferred texture allocated a CPU shadow or became invalid")
			}
			texelBytes := format.TexelBytes()
			want := make([]byte, size*size*texelBytes)
			updates := []TextureRegionUpdate{
				{X: 3, Y: 4, Width: 2, Height: 2, Pixels: bytes.Repeat([]byte{173}, 4*texelBytes)},
				{X: 2, Y: 24, Width: 3, Height: 1, Pixels: bytes.Repeat([]byte{231}, 3*texelBytes)},
			}
			for _, update := range updates {
				texture.UpdateRegions([]TextureRegionUpdate{update})
				for row := range update.Height {
					offset := ((update.Y+row)*size + update.X) * texelBytes
					copy(want[offset:], update.Pixels[row*update.Width*texelBytes:(row+1)*update.Width*texelBytes])
				}
			}
			if cap(texture.pix) >= len(want) {
				t.Fatal("partial updates retained a full CPU shadow")
			}
			texture.UpdateRegions([]TextureRegionUpdate{
				{Width: 1, Height: 1, Pixels: bytes.Repeat([]byte{255}, texelBytes)},
				{X: size, Width: 1, Height: 1, Pixels: make([]byte, texelBytes)},
			})
			for range 2 {
				driver := &textureTestDriver{}
				if err := render.Install(driver); err != nil {
					t.Fatal(err)
				}
				if !texture.RenderHandle().Valid() || !bytes.Equal(driver.createdPixels, want) {
					t.Fatal("device upload lost updated texels or changed untouched texels")
				}
				if cap(texture.pix) >= len(want) {
					t.Fatal("device upload expanded the retained CPU shadow")
				}
			}
			if allocations := testing.AllocsPerRun(100, func() { texture.UpdateRegions(updates) }); allocations != 0 {
				t.Fatalf("warm compact texture updates allocated %g objects", allocations)
			}
		})
	}
}

func TestEmptyGPUOnlyTextureStaysOnOwningDevice(t *testing.T) {
	if err := render.Install(&textureTestDriver{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	texture := CreateEmptyTexture(8, 8, render.TextureFormatR8, ResourceUsageGPU)
	if !texture.Valid() || !texture.RenderHandle().Valid() || len(texture.pix) != 0 || texture.zeroInitialized {
		t.Fatal("GPU-only empty texture retained a CPU payload")
	}
	if err := render.Install(&textureTestDriver{}); err != nil {
		t.Fatal(err)
	}
	if texture.RenderHandle().Valid() || texture.UploadError() != errTextureDevice {
		t.Fatal("GPU-only texture recreated empty pixels on a replacement device")
	}
}

func TestCreateTextureByImageNoMipmapStaysValidBeforeDeviceInit(t *testing.T) {
	render.Uninstall()
	texture := CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 8, 4)))
	if !texture.Valid() {
		t.Fatal("expected deferred texture to stay valid before device init")
	}
	if texture.RenderHandle().Valid() {
		t.Fatal("expected no GPU texture handle before device init")
	}
	if len(texture.pix) == 0 {
		t.Fatal("expected deferred texture pixels to be retained")
	}
}

func TestR8TextureRetainsSingleChannelShadowAndUpdates(t *testing.T) {
	driver := &textureTestDriver{}
	if err := render.Install(driver); err != nil {
		t.Fatalf("install test driver: %v", err)
	}
	t.Cleanup(render.Uninstall)
	pixels := make([]byte, 4*3)
	texture := CreateTextureR8(4, 3, pixels, false, ResourceUsageDefault)
	if texture.Format() != render.TextureFormatR8 {
		t.Fatalf("texture format = %v, want R8", texture.Format())
	}
	if len(texture.pix) != 12 {
		t.Fatalf("R8 CPU shadow length = %d, want 12", len(texture.pix))
	}
	texture.UpdateRegions([]TextureRegionUpdate{{X: 1, Y: 1, Width: 2, Height: 1, Pixels: []byte{17, 233}}})
	if got := texture.pix[5:7]; got[0] != 17 || got[1] != 233 {
		t.Fatalf("R8 region update = %v, want [17 233]", got)
	}
}

func TestSubTextureUsesParentHandle(t *testing.T) {
	driver := &textureTestDriver{}
	if err := render.Install(driver); err != nil {
		t.Fatalf("install test driver: %v", err)
	}
	t.Cleanup(render.Uninstall)
	parent := CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 32, 32)))
	child := NewSubTexture(parent, 16, 16, 0, 0, 0.5, 0.5, 0)
	if !child.Valid() {
		t.Fatal("expected sub texture to be valid when parent texture is valid")
	}
	if child.RenderHandle() != parent.RenderHandle() {
		t.Fatalf("expected sub texture to reuse parent handle: child=%s parent=%s", child.RenderHandle(), parent.RenderHandle())
	}
}

func TestDeferredUpdateRegionsMutatesPendingPixels(t *testing.T) {
	render.Uninstall()
	texture := CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 2, 2)))
	texture.UpdateRegions([]TextureRegionUpdate{{
		X: 1, Y: 0, Width: 1, Height: 1,
		Pixels: []byte{255, 0, 0, 255},
	}})
	if got := texture.pix[4:8]; got[0] != 255 || got[1] != 0 || got[2] != 0 || got[3] != 255 {
		t.Fatalf("expected deferred sub image update to patch pending pixels, got %v", got)
	}
}

func TestDisposeReleasesTextureAndCPUShadow(t *testing.T) {
	driver := &textureTestDriver{}
	if err := render.Install(driver); err != nil {
		t.Fatalf("install test driver: %v", err)
	}
	t.Cleanup(render.Uninstall)

	texture := CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 8, 8)))
	if !texture.RenderHandle().Valid() {
		t.Fatal("initial texture upload failed")
	}
	if err := texture.Dispose(); err != nil {
		t.Fatalf("dispose texture: %v", err)
	}
	if texture.Valid() || texture.RenderHandle().Valid() || len(texture.pix) != 0 {
		t.Fatal("disposed texture retained render or CPU resources")
	}
}

func TestDisposeSubTextureKeepsParentAlive(t *testing.T) {
	driver := &textureTestDriver{}
	if err := render.Install(driver); err != nil {
		t.Fatalf("install test driver: %v", err)
	}
	t.Cleanup(render.Uninstall)

	parent := CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 16, 16)))
	child := NewSubTexture(parent, 8, 8, 0, 0, 0.5, 0.5, 0)
	if err := child.Dispose(); err != nil {
		t.Fatalf("dispose sub texture: %v", err)
	}
	if !parent.Valid() || !parent.RenderHandle().Valid() {
		t.Fatal("disposing a sub texture invalidated its parent")
	}
	if child.Valid() || driver.destroyedTextures != 0 {
		t.Fatal("disposing a sub texture released the shared GPU texture")
	}
}

func TestGPUOnlyTextureReleasesCPUShadowAfterUpload(t *testing.T) {
	driver := &textureTestDriver{}
	if err := render.Install(driver); err != nil {
		t.Fatalf("install test driver: %v", err)
	}
	t.Cleanup(render.Uninstall)

	texture := CreateTextureByImageWithUsage(
		image.NewRGBA(image.Rect(0, 0, 16, 8)),
		false,
		ResourceUsageGPU,
	)
	if !texture.Valid() || !texture.RenderHandle().Valid() {
		t.Fatal("GPU-only texture did not remain valid after upload")
	}
	if texture.pix != nil {
		t.Fatalf("GPU-only texture retained %d CPU bytes", len(texture.pix))
	}
}

func TestCreateTextureFromExternalSurface(t *testing.T) {
	driver := &textureTestDriver{}
	if err := render.Install(driver); err != nil {
		t.Fatalf("install test driver: %v", err)
	}
	t.Cleanup(render.Uninstall)
	texture, err := CreateTextureFromExternal(render.ExternalTextureDescriptor{
		Kind: render.ExternalTextureD3D11, Handle: 17, Width: 64, Height: 32,
		Format: render.ExternalTextureBGRA8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !texture.Valid() || !texture.RenderHandle().Valid() || texture.pix != nil {
		t.Fatal("external texture retained an invalid or CPU-backed representation")
	}
	if driver.externalDescriptor.Handle != 17 {
		t.Fatalf("external handle = %d", driver.externalDescriptor.Handle)
	}
	content, geometry := texture.ContentRevision(), texture.GeometryRevision()
	if err := texture.RefreshExternal(); err != nil {
		t.Fatal(err)
	}
	if texture.ContentRevision() == content || texture.GeometryRevision() != geometry {
		t.Fatal("external pixel refresh did not preserve geometry revision")
	}
	if driver.externalRefreshes != 1 {
		t.Fatalf("external refreshes = %d, want 1", driver.externalRefreshes)
	}
}

func TestCreateTextureFromExternalSurfaceRejectsUnsupportedDriver(t *testing.T) {
	render.Uninstall()
	if _, err := CreateTextureFromExternal(render.ExternalTextureDescriptor{
		Kind: render.ExternalTextureD3D11, Handle: 1, Width: 1, Height: 1,
		Format: render.ExternalTextureBGRA8,
	}); err == nil {
		t.Fatal("expected unsupported external texture driver error")
	}
}

func TestUpdateRegionsUsesOneDriverBatch(t *testing.T) {
	driver := &textureTestDriver{}
	if err := render.Install(driver); err != nil {
		t.Fatalf("install test driver: %v", err)
	}
	t.Cleanup(render.Uninstall)
	texture := CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 8, 8)))
	if !texture.RenderHandle().Valid() {
		t.Fatal("initial texture upload failed")
	}

	texture.UpdateRegions([]TextureRegionUpdate{
		{X: 0, Y: 0, Width: 2, Height: 2, Pixels: make([]byte, 16)},
		{X: 4, Y: 4, Width: 2, Height: 2, Pixels: make([]byte, 16)},
	})

	if driver.updateCalls != 1 || driver.updateCount != 2 {
		t.Fatalf("texture updates = %d calls / %d regions, want 1 / 2", driver.updateCalls, driver.updateCount)
	}
}

type textureTestDriver struct {
	descriptor         render.TextureDescriptor
	createdPixels      []byte
	next               uint32
	destroyedTextures  int
	updateCalls        int
	updateCount        int
	externalDescriptor render.ExternalTextureDescriptor
	externalRefreshes  int
}

func (d *textureTestDriver) RendererInfo() render.RendererInfo { return render.RendererInfo{} }
func (d *textureTestDriver) BeginFrame(render.Color) error     { return nil }
func (d *textureTestDriver) SetViewport(render.Rect)           {}
func (d *textureTestDriver) Viewport() render.Rect             { return render.Rect{} }
func (d *textureTestDriver) SetBlend(render.BlendState) error  { return nil }
func (d *textureTestDriver) SetScissor(*render.Rect)           {}
func (d *textureTestDriver) CreateTexture(descriptor render.TextureDescriptor, pixels []byte) (render.TextureHandle, error) {
	if err := descriptor.Validate(pixels); err != nil {
		return render.TextureHandle{}, err
	}
	d.descriptor = descriptor
	d.createdPixels = append(d.createdPixels[:0], pixels...)
	d.next++
	return render.NewHandle[render.TextureResource](d.next, 1), nil
}
func (d *textureTestDriver) CreateExternalTexture(descriptor render.ExternalTextureDescriptor) (render.TextureHandle, error) {
	if err := descriptor.Validate(); err != nil {
		return render.TextureHandle{}, err
	}
	d.externalDescriptor = descriptor
	d.next++
	return render.NewHandle[render.TextureResource](d.next, 1), nil
}
func (d *textureTestDriver) RefreshExternalTexture(render.TextureHandle) error {
	d.externalRefreshes++
	return nil
}
func (d *textureTestDriver) UpdateTextures(updates []render.TextureUpdate) error {
	d.updateCalls++
	d.updateCount += len(updates)
	return nil
}
func (d *textureTestDriver) DestroyTexture(render.TextureHandle) error {
	d.destroyedTextures++
	return nil
}
func (d *textureTestDriver) DrawSprites(render.SpriteBatch) error { return nil }
func (d *textureTestDriver) DrawShapes(render.ShapeBatch) error   { return nil }
func (d *textureTestDriver) Present() error                       { return nil }
func (d *textureTestDriver) ResizeSurface(int, int) error         { return nil }
func (d *textureTestDriver) SetVSync(bool)                        {}
func (d *textureTestDriver) Destroy()                             {}

func TestDecodedTextureAddressingReachesDriverBeforeUpload(t *testing.T) {
	driver := &textureTestDriver{}
	render.Install(driver)
	t.Cleanup(render.Uninstall)
	texture := TextureFromOwnedNRGBA(image.NewNRGBA(image.Rect(0, 0, 2, 2)), TextureOptions{Usage: ResourceUsageGPU, Mipmap: true, WrapU: render.TextureAddressRepeat, WrapV: render.TextureAddressMirror})
	defer texture.Dispose()
	if driver.next != 0 || !texture.Valid() {
		t.Fatal("CPU construction or validation invoked the driver")
	}
	texture.RenderHandle()
	if driver.next != 1 || driver.descriptor.WrapU != render.TextureAddressRepeat || driver.descriptor.WrapV != render.TextureAddressMirror || driver.descriptor.Filter != render.TextureFilterLinearMipmapLinear {
		t.Fatalf("sampler upload: count=%d descriptor=%+v", driver.next, driver.descriptor)
	}
	u, v := texture.AddressModes()
	if u != driver.descriptor.WrapU || v != driver.descriptor.WrapV {
		t.Fatal("texture lost addressing")
	}
}
