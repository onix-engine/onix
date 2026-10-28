package editor

import (
	"testing"

	"github.com/diiyw/onix/render"
)

type viewport3DTestDriver struct {
	viewport  render.Rect
	nextID    uint32
	clears    []render.RenderTargetClear
	meshDraws int
	shapes    []render.ShapeInstance2D
}

func (*viewport3DTestDriver) RendererInfo() render.RendererInfo { return render.RendererInfo{} }
func (*viewport3DTestDriver) BeginFrame(render.Color) error     { return nil }
func (d *viewport3DTestDriver) SetViewport(r render.Rect)       { d.viewport = r }
func (d *viewport3DTestDriver) Viewport() render.Rect           { return d.viewport }
func (*viewport3DTestDriver) SetBlend(render.BlendState) error  { return nil }
func (*viewport3DTestDriver) SetScissor(*render.Rect)           {}
func (d *viewport3DTestDriver) CreateTexture(render.TextureDescriptor, []byte) (render.TextureHandle, error) {
	d.nextID++
	return render.NewHandle[render.TextureResource](d.nextID, 1), nil
}
func (*viewport3DTestDriver) UpdateTextures([]render.TextureUpdate) error { return nil }
func (*viewport3DTestDriver) DestroyTexture(render.TextureHandle) error   { return nil }
func (*viewport3DTestDriver) DrawSprites(render.SpriteBatch) error        { return nil }
func (d *viewport3DTestDriver) DrawShapes(batch render.ShapeBatch) error {
	d.shapes = append(d.shapes, batch.Instances...)
	return nil
}
func (*viewport3DTestDriver) Present() error               { return nil }
func (*viewport3DTestDriver) ResizeSurface(int, int) error { return nil }
func (*viewport3DTestDriver) SetVSync(bool)                {}
func (*viewport3DTestDriver) Destroy()                     {}
func (d *viewport3DTestDriver) CreateMesh([]render.Vertex3D, []uint32) (render.MeshHandle, error) {
	d.nextID++
	return render.NewHandle[render.MeshResource](d.nextID, 1), nil
}
func (*viewport3DTestDriver) DestroyMesh(render.MeshHandle) error { return nil }
func (d *viewport3DTestDriver) DrawMeshes(batch render.MeshBatch3D) error {
	for _, draw := range batch.Draws {
		if draw.Mesh.Valid() {
			d.meshDraws++
		}
	}
	return nil
}
func (d *viewport3DTestDriver) CreateRenderTarget(render.RenderTargetDescriptor) (render.RenderTargetHandle, render.TextureHandle, error) {
	d.nextID += 2
	return render.NewHandle[render.RenderTargetResource](d.nextID-1, 1), render.NewHandle[render.TextureResource](d.nextID, 1), nil
}
func (d *viewport3DTestDriver) ResizeRenderTarget(render.RenderTargetHandle, render.RenderTargetDescriptor) (render.TextureHandle, error) {
	d.nextID++
	return render.NewHandle[render.TextureResource](d.nextID, 1), nil
}
func (*viewport3DTestDriver) DestroyRenderTarget(render.RenderTargetHandle) error { return nil }
func (d *viewport3DTestDriver) BeginRenderTarget(_ render.RenderTargetHandle, clear render.RenderTargetClear) error {
	d.clears = append(d.clears, clear)
	return nil
}
func (*viewport3DTestDriver) EndRenderTarget() error { return nil }

func TestViewportTargetUsesExplicitDevice(t *testing.T) {
	first, second := &viewport3DTestDriver{}, &viewport3DTestDriver{}
	device, _ := render.NewDevice(first)
	other, _ := render.NewDevice(second)
	previous := render.ActiveDevice()
	defer previous.Activate()
	defer device.Destroy()
	defer other.Destroy()
	other.Activate()
	var target editorRenderTarget
	defer target.disposeTarget()
	if ok, err := target.ensureTarget(device, 100, 80, 1); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if first.nextID != 0 || second.nextID != 0 {
		t.Fatal("CPU target construction invoked a render driver")
	}
	if !target.colorTexture.RenderHandleFor(device).Valid() || first.nextID != 2 || second.nextID != 0 {
		t.Fatal("target was not realized exclusively on its explicit device")
	}
	if ok, err := target.ensureTarget(device, 200, 160, 1); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if !target.colorTexture.RenderHandleFor(device).Valid() || first.nextID != 3 || second.nextID != 0 {
		t.Fatal("target resize used the global device")
	}
}
