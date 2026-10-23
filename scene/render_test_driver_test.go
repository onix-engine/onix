package scene

import "github.com/diiyw/onix/render"

type canvasTestDriver struct {
	viewport render.Rect
	nextID   uint32
	sprites  int
	shapes   int
}

func (*canvasTestDriver) RendererInfo() render.RendererInfo { return render.RendererInfo{} }
func (*canvasTestDriver) BeginFrame(render.Color) error     { return nil }
func (d *canvasTestDriver) SetViewport(value render.Rect)   { d.viewport = value }
func (d *canvasTestDriver) Viewport() render.Rect           { return d.viewport }
func (*canvasTestDriver) SetBlend(render.BlendState) error  { return nil }
func (*canvasTestDriver) SetScissor(*render.Rect)           {}
func (d *canvasTestDriver) CreateTexture(descriptor render.TextureDescriptor, pixels []byte) (render.TextureHandle, error) {
	if err := descriptor.Validate(pixels); err != nil {
		return render.TextureHandle{}, err
	}
	d.nextID++
	return render.NewHandle[render.TextureResource](d.nextID, 1), nil
}
func (*canvasTestDriver) UpdateTextures([]render.TextureUpdate) error { return nil }
func (*canvasTestDriver) DestroyTexture(render.TextureHandle) error   { return nil }
func (d *canvasTestDriver) DrawSprites(batch render.SpriteBatch) error {
	d.sprites += len(batch.Instances)
	return nil
}
func (d *canvasTestDriver) DrawShapes(batch render.ShapeBatch) error {
	d.shapes += len(batch.Instances)
	return nil
}
func (*canvasTestDriver) Present() error               { return nil }
func (*canvasTestDriver) ResizeSurface(int, int) error { return nil }
func (*canvasTestDriver) SetVSync(bool)                {}
func (*canvasTestDriver) Destroy()                     {}
