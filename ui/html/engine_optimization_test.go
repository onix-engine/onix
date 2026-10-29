package html

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/layout"
	"github.com/diiyw/onix/html/style"
	"github.com/diiyw/onix/render"
)

func TestHTMLOpacityTextureReusesRasterAndTexture(t *testing.T) {
	if err := render.Install(&textureTestDriver{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	state := htmlRenderState{}
	root := &layout.Box{Node: &dom.Node{}, Content: layout.Rect{Width: 16, Height: 16}, Style: &style.ComputedStyle{}}
	root.Style.BackgroundColor = style.Value{Kind: style.ValueColor, Color: style.Color{R: 255, A: 255}}
	root.Style.Opacity = style.Value{Kind: style.ValueNumber, Number: 0.5}
	child := &layout.Box{Node: &dom.Node{}, Content: layout.Rect{Width: 8, Height: 8}, Style: &style.ComputedStyle{}}
	child.Style.BackgroundColor = style.Value{Kind: style.ValueColor, Color: style.Color{B: 255, A: 255}}
	child.Style.Opacity = style.Value{Kind: style.ValueNumber, Number: 1}
	root.Children = []*layout.Box{child}
	texture := htmlOpacityTexture(&state, root)
	state.Textures.bind(1, texture)
	state.Textures.sweep()
	for range 100 {
		state.RenderGeneration++
		if htmlOpacityTexture(&state, root) != texture {
			t.Fatal("unchanged content recreated texture")
		}
	}
	if allocations := testing.AllocsPerRun(100, func() { htmlOpacityTexture(&state, root) }); allocations != 0 {
		t.Fatalf("cache-hit allocations=%v", allocations)
	}
	child.Style.BackgroundColor.Color = style.Color{G: 255, A: 255}
	if htmlOpacityTexture(&state, root) != texture {
		t.Fatal("same-size repaint recreated texture")
	}
	pixel := state.Textures.rasters[root.Node].image.RGBAAt(1, 1)
	if pixel.G != 255 || pixel.A != 128 {
		t.Fatalf("updated composite=%+v", pixel)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		child.Style.BackgroundColor.Color.G ^= 1
		if htmlOpacityTexture(&state, root) != texture {
			t.Fatal("repaint recreated texture")
		}
	}); allocations != 0 {
		t.Fatalf("repaint allocations=%v", allocations)
	}
	root.Content.Width++
	replacement := htmlOpacityTexture(&state, root)
	if replacement == texture {
		t.Fatal("dimension change retained old texture")
	}
	state.Textures.bind(1, replacement)
	state.Textures.sweep()
	if texture.Valid() || !replacement.Valid() || len(state.Textures.entries) != 1 {
		t.Fatal("dimension replacement retained old texture or deleted new entry")
	}
	texture = replacement
	state.Textures.bind(1, nil)
	state.Textures.sweep()
	if texture.Valid() || len(state.Textures.entries) != 0 {
		t.Fatal("unused opacity texture retained")
	}
}

// textureTestDriver keeps the raster lifecycle tests independent of the scene renderer.
type textureTestDriver struct {
	viewport render.Rect
	nextID   uint32
}

func (*textureTestDriver) RendererInfo() render.RendererInfo { return render.RendererInfo{} }
func (*textureTestDriver) BeginFrame(render.Color) error     { return nil }
func (d *textureTestDriver) SetViewport(value render.Rect)   { d.viewport = value }
func (d *textureTestDriver) Viewport() render.Rect           { return d.viewport }
func (*textureTestDriver) SetBlend(render.BlendState) error  { return nil }
func (*textureTestDriver) SetScissor(*render.Rect)           {}
func (d *textureTestDriver) CreateTexture(descriptor render.TextureDescriptor, pixels []byte) (render.TextureHandle, error) {
	if err := descriptor.Validate(pixels); err != nil {
		return render.TextureHandle{}, err
	}
	d.nextID++
	return render.NewHandle[render.TextureResource](d.nextID, 1), nil
}
func (*textureTestDriver) UpdateTextures([]render.TextureUpdate) error { return nil }
func (*textureTestDriver) DestroyTexture(render.TextureHandle) error   { return nil }
func (*textureTestDriver) DrawSprites(render.SpriteBatch) error        { return nil }
func (*textureTestDriver) DrawShapes(render.ShapeBatch) error          { return nil }
func (*textureTestDriver) Present() error                              { return nil }
func (*textureTestDriver) ResizeSurface(int, int) error                { return nil }
func (*textureTestDriver) SetVSync(bool)                               {}
func (*textureTestDriver) Destroy()                                    {}

func TestHTMLTexturesReleasedOnOwnerRemoval(t *testing.T) {
	world := ecs.NewWorld()
	value := New(`<div style="width:16px;height:16px;background:linear-gradient(red,blue)"></div>`)
	value.Width, value.Height = 32, 32
	spawner := world.NewSpawner1[Surface]()
	owner := spawner.Spawn(value)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	state := world.GetRef[htmlRenderState](owner)
	var texture *htmlOwnedTexture
	for _, entry := range state.Textures.entries {
		texture = entry
		break
	}
	if texture == nil {
		t.Fatal("gradient texture missing")
	}
	world.Despawn(owner)
	if texture.texture.Valid() {
		t.Fatal("owner removal retained texture")
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	if world.EntityCount() != 0 {
		t.Fatalf("orphan primitives=%d", world.EntityCount())
	}
}

func TestHTMLRemovalAndReinsertRecreatesOwnedTextures(t *testing.T) {
	world := ecs.NewWorld()
	value := New(`<div style="width:16px;height:16px;background:linear-gradient(red,blue)"></div>`)
	value.Width, value.Height = 32, 32
	spawner := world.NewSpawner1[Surface]()
	owner := spawner.Spawn(value)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	state := world.GetRef[htmlRenderState](owner)
	var previous *htmlOwnedTexture
	for _, texture := range state.Textures.entries {
		previous = texture
		break
	}
	world.Remove[Surface](owner)
	if previous == nil || previous.texture.Valid() {
		t.Fatal("HTML removal retained owned texture")
	}
	if err := world.Insert(owner, value); err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	state = world.GetRef[htmlRenderState](owner)
	if len(state.Textures.entries) == 0 {
		t.Fatal("reinsertion retained disposed rendering cache")
	}
	world.Remove[Surface](owner)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	if world.Has[htmlRenderState](owner) || world.EntityCount() != 1 {
		t.Fatal("removed surface retained state or primitives")
	}
}
