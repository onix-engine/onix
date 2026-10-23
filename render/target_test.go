package render

import (
	"slices"
	"testing"
)

func TestRenderTargetDefersRealizationAndResize(t *testing.T) {
	driver := &renderTargetTestDriver{}
	installRenderTargetTestDriver(t, driver)
	target, err := NewRenderTarget(RenderTargetDescriptor{Width: 320, Height: 180, Depth: true})
	if err != nil {
		t.Fatal(err)
	}
	if driver.created != 0 {
		t.Fatal("new render target realized GPU resources eagerly")
	}
	first, err := target.ColorTexture()
	if err != nil {
		t.Fatal(err)
	}
	if driver.created != 1 || driver.resized != 0 || !first.Valid() {
		t.Fatalf("first realization = created %d resized %d color %s", driver.created, driver.resized, first)
	}
	if !target.Resize(640, 360) || driver.resized != 0 {
		t.Fatal("resize did not remain deferred")
	}
	second, err := target.ColorTexture()
	if err != nil {
		t.Fatal(err)
	}
	if driver.created != 1 || driver.resized != 1 || second == first {
		t.Fatalf("resize = created %d resized %d color %s, first %s", driver.created, driver.resized, second, first)
	}
	if target.Resize(640, 360) {
		t.Fatal("unchanged render target size was marked dirty")
	}
}

func TestRenderTargetBeginReadAndDispose(t *testing.T) {
	driver := &renderTargetTestDriver{}
	installRenderTargetTestDriver(t, driver)
	target, err := NewRenderTarget(RenderTargetDescriptor{Width: 8, Height: 4, ColorFormat: RenderTargetUint32, Depth: true})
	if err != nil {
		t.Fatal(err)
	}
	clear := RenderTargetClear{Color: Color{R: 0.25, A: 1}, Depth: 0, ClearColor: true, ClearDepth: true}
	if err := target.Begin(clear); err != nil {
		t.Fatal(err)
	}
	if err := target.End(); err != nil {
		t.Fatal(err)
	}
	if driver.begins != 1 || driver.ends != 1 || driver.clear != clear {
		t.Fatalf("target pass calls = begin %d end %d clear %+v", driver.begins, driver.ends, driver.clear)
	}
	pixels := make([]byte, 2*2*4)
	if err := target.Read(RenderTargetRegion{X: 1, Y: 1, Width: 2, Height: 2}, pixels); err != nil {
		t.Fatal(err)
	}
	for index, value := range pixels {
		if value != byte(index) {
			t.Fatalf("readback byte %d = %d", index, value)
		}
	}
	if err := target.Dispose(); err != nil {
		t.Fatal(err)
	}
	if driver.destroyed != 1 {
		t.Fatalf("destroy calls = %d, want 1", driver.destroyed)
	}
	if _, err := target.ColorTexture(); err == nil {
		t.Fatal("disposed render target was realized again")
	}
}

func TestRealizedRenderTargetBeginDoesNotAllocate(t *testing.T) {
	driver := &renderTargetTestDriver{}
	installRenderTargetTestDriver(t, driver)
	target, err := NewRenderTarget(RenderTargetDescriptor{Width: 64, Height: 64, Depth: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := target.ColorTexture(); err != nil {
		t.Fatal(err)
	}
	allocations := testing.AllocsPerRun(100, func() {
		if err := target.Begin(RenderTargetClear{ClearDepth: true}); err != nil {
			t.Fatal(err)
		}
		if err := target.End(); err != nil {
			t.Fatal(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("render target pass allocations = %v, want 0", allocations)
	}
}

func TestRenderTargetRecreatesForNewDeviceGeneration(t *testing.T) {
	firstDriver := &renderTargetTestDriver{}
	installRenderTargetTestDriver(t, firstDriver)
	target, err := NewRenderTarget(RenderTargetDescriptor{Width: 64, Height: 64, Depth: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := target.ColorTexture(); err != nil {
		t.Fatal(err)
	}

	secondDriver := &renderTargetTestDriver{}
	if err := Install(secondDriver); err != nil {
		t.Fatal(err)
	}
	if _, err := target.ColorTexture(); err != nil {
		t.Fatal(err)
	}
	if firstDriver.created != 1 || secondDriver.created != 1 || secondDriver.resized != 0 {
		t.Fatalf("generation recreation = first created %d, second created %d resized %d", firstDriver.created, secondDriver.created, secondDriver.resized)
	}
	if err := target.Dispose(); err != nil {
		t.Fatal(err)
	}
	if firstDriver.destroyed != 0 || secondDriver.destroyed != 1 {
		t.Fatalf("generation destruction = first %d, second %d", firstDriver.destroyed, secondDriver.destroyed)
	}
}

func TestRenderViewKeepsTypedCameraAndTargetSize(t *testing.T) {
	type camera struct{ Entity uint32 }
	target, err := NewRenderTarget(RenderTargetDescriptor{Width: 512, Height: 256})
	if err != nil {
		t.Fatal(err)
	}
	view := NewRenderView(camera{Entity: 7})
	view.Target = target
	view.Viewport = Rect{Width: 100, Height: 50}
	if view.Camera.Entity != 7 || !view.Enabled || view.Mode != RenderViewLit {
		t.Fatalf("unexpected typed render view: %+v", view)
	}
	if width, height := view.Size(Rect{Width: 800, Height: 600}); width != 512 || height != 256 {
		t.Fatalf("target view size = %dx%d", width, height)
	}
}

func TestFlipRenderTargetRowsUsesTopLeftOrder(t *testing.T) {
	pixels := []byte{
		1, 2, 3, 4, 5, 6, 7, 8,
		9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24,
	}
	flipRenderTargetRows(pixels, 8, 3)
	want := [...]byte{
		17, 18, 19, 20, 21, 22, 23, 24,
		9, 10, 11, 12, 13, 14, 15, 16,
		1, 2, 3, 4, 5, 6, 7, 8,
	}
	if !slices.Equal(pixels, want[:]) {
		t.Fatalf("flipped rows = %v, want %v", pixels, want)
	}
}

func installRenderTargetTestDriver(t *testing.T, driver *renderTargetTestDriver) {
	t.Helper()
	Uninstall()
	if err := Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(Uninstall)
}

type renderTargetTestDriver struct {
	viewport  Rect
	next      uint32
	created   int
	resized   int
	destroyed int
	begins    int
	ends      int
	clear     RenderTargetClear
}

func (driver *renderTargetTestDriver) RendererInfo() RendererInfo { return RendererInfo{} }
func (driver *renderTargetTestDriver) BeginFrame(Color) error     { return nil }
func (driver *renderTargetTestDriver) SetViewport(viewport Rect)  { driver.viewport = viewport }
func (driver *renderTargetTestDriver) Viewport() Rect             { return driver.viewport }
func (driver *renderTargetTestDriver) SetBlend(BlendState) error  { return nil }
func (*renderTargetTestDriver) SetScissor(*Rect)                  {}
func (driver *renderTargetTestDriver) CreateTexture(descriptor TextureDescriptor, pixels []byte) (TextureHandle, error) {
	if err := descriptor.Validate(pixels); err != nil {
		return TextureHandle{}, err
	}
	driver.next++
	return NewHandle[TextureResource](driver.next, 1), nil
}
func (*renderTargetTestDriver) UpdateTextures([]TextureUpdate) error { return nil }
func (*renderTargetTestDriver) DestroyTexture(TextureHandle) error   { return nil }
func (*renderTargetTestDriver) DrawSprites(SpriteBatch) error        { return nil }
func (*renderTargetTestDriver) DrawShapes(ShapeBatch) error          { return nil }
func (*renderTargetTestDriver) Present() error                       { return nil }
func (*renderTargetTestDriver) ResizeSurface(int, int) error         { return nil }
func (*renderTargetTestDriver) SetVSync(bool)                        {}
func (*renderTargetTestDriver) Destroy()                             {}

func (driver *renderTargetTestDriver) CreateRenderTarget(RenderTargetDescriptor) (RenderTargetHandle, TextureHandle, error) {
	driver.created++
	driver.next++
	target := NewHandle[RenderTargetResource](driver.next, 1)
	driver.next++
	return target, NewHandle[TextureResource](driver.next, 1), nil
}

func (driver *renderTargetTestDriver) ResizeRenderTarget(RenderTargetHandle, RenderTargetDescriptor) (TextureHandle, error) {
	driver.resized++
	driver.next++
	return NewHandle[TextureResource](driver.next, 1), nil
}

func (driver *renderTargetTestDriver) DestroyRenderTarget(RenderTargetHandle) error {
	driver.destroyed++
	return nil
}

func (driver *renderTargetTestDriver) BeginRenderTarget(_ RenderTargetHandle, clear RenderTargetClear) error {
	driver.begins++
	driver.clear = clear
	return nil
}

func (driver *renderTargetTestDriver) EndRenderTarget() error {
	driver.ends++
	return nil
}

func (*renderTargetTestDriver) ReadRenderTarget(_ RenderTargetHandle, _ RenderTargetRegion, destination []byte) error {
	for index := range destination {
		destination[index] = byte(index)
	}
	return nil
}
