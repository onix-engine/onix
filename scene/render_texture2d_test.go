package scene

import (
	"image"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
)

func TestRetainedTextureChangesStayLocal(t *testing.T) {
	w, canvas, _, entities := incrementalRenderFixture(t, 100, false)
	texture := graphics.CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 10, 5)))
	other := graphics.CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 2, 2)))
	t.Cleanup(func() { _ = texture.Dispose(); _ = other.Dispose() })
	sprite := ecs.GetMut[Sprite2D](w, entities[0])
	*sprite = Sprite2DFromTexture(texture)
	sprite.Width, sprite.Height = 0, 0
	incrementalFrame(t, w, canvas)
	state := renderWorld(w)
	snapshot := state.Items2D[0].sprite
	updates := []graphics.TextureRegionUpdate{{Width: 1, Height: 1, Pixels: []byte{255, 0, 0, 255}}}
	other.UpdateRegions(updates)
	incrementalFrame(t, w, canvas)
	if state.Preparation2D.Extracted != 0 || state.Preparation2D.PaintUpdated != 0 || state.Composite2D.PreparedTiles != 0 {
		t.Fatal("unrelated texture invalidated retained output", state.Preparation2D, state.Composite2D)
	}
	step := func() {
		updates[0].Pixels[0] ^= 255
		texture.UpdateRegions(updates)
		incrementalFrame(t, w, canvas)
	}
	step()
	if state.Preparation2D.Extracted != 0 || state.Preparation2D.PaintUpdated != 1 || state.Preparation2D.Sorted || state.Items2D[0].sprite != snapshot || state.Composite2D.DirtyTiles != 1 || state.Composite2D.PreparedTiles != 1 {
		t.Fatal("pixel update rebuilt geometry or unrelated tiles", state.Preparation2D, state.Composite2D)
	}
	if allocs := testing.AllocsPerRun(20, step); allocs != 0 {
		t.Fatalf("warm texture updates allocated %v times", allocs)
	}
	texture.UpdateRegions(updates)
	ecs.GetMut[UIGlobalTransform](w, entities[0]).Matrix[12] = 600
	incrementalFrame(t, w, canvas)
	cache := ecs.ScopeValue[retainedRender2D, retainedRender2D](w)
	if len(cache.damage) != 1 || state.Composite2D.DirtyTiles != 2 {
		t.Fatal("combined pixel and transform changes damaged an item twice", state.Composite2D)
	}
	texture.SetLogicalSize(20, 10)
	incrementalFrame(t, w, canvas)
	if state.Preparation2D.Extracted != 1 || state.Preparation2D.Sorted || state.Items2D[0].width != 20 || state.Items2D[0].height != 10 {
		t.Fatal("logical bounds did not update their owner", state.Preparation2D)
	}
}

func TestRetainedAtlasViewDependencyReplacement(t *testing.T) {
	w, canvas, _, entities := incrementalRenderFixture(t, 100, false)
	base := graphics.CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 8, 8)))
	left := graphics.NewSubTexture(base, 4, 4, 0, 0, .5, .5, 0)
	right := graphics.NewSubTexture(base, 4, 4, .5, .5, 1, 1, 0)
	t.Cleanup(func() { _ = left.Dispose(); _ = right.Dispose(); _ = base.Dispose() })
	*ecs.GetMut[Sprite2D](w, entities[0]) = Sprite2DFromTexture(left)
	incrementalFrame(t, w, canvas)
	ecs.GetMut[Sprite2D](w, entities[0]).Texture = right
	incrementalFrame(t, w, canvas)
	cache := ecs.ScopeValue[retainedRender2D, retainedRender2D](w)
	if cache.textures[left] != nil || cache.textures[right] == nil {
		t.Fatal("sparse replacement retained a stale atlas view dependency")
	}
	left.SetLogicalSize(20, 20)
	incrementalFrame(t, w, canvas)
	state := renderWorld(w)
	if state.Preparation2D.Extracted != 0 || state.Preparation2D.PaintUpdated != 0 {
		t.Fatal("unused atlas view invalidated the active view")
	}
	base.UpdateRegions([]graphics.TextureRegionUpdate{{Width: 1, Height: 1, Pixels: []byte{255, 0, 0, 255}}})
	incrementalFrame(t, w, canvas)
	if state.Preparation2D.Extracted != 0 || state.Preparation2D.PaintUpdated != 1 {
		t.Fatal("base texture update missed its atlas view")
	}
}

func TestInheritedRenderMetadataUpdatesOnlyItsSubtree(t *testing.T) {
	w, canvas, _, entities := incrementalRenderFixture(t, 100, false)
	spawner := ecs.NewSpawner1[RenderLayers](w)
	parent := spawner.Spawn(RenderLayers{})
	if err := ecs.Insert(w, entities[0], Parent{Entity: parent}); err != nil {
		t.Fatal(err)
	}
	incrementalFrame(t, w, canvas)
	// An equal write still takes the sparse path and preserves stable order.
	ecs.GetMut[RenderLayers](w, parent)
	incrementalFrame(t, w, canvas)
	state := renderWorld(w)
	if state.Preparation2D.Extracted != 1 || state.Preparation2D.Reused != 99 || state.Preparation2D.Sorted {
		t.Fatal("inherited metadata re-extracted unrelated entities", state.Preparation2D)
	}
}

func BenchmarkRetainedTexturePixelUpdate(b *testing.B) {
	w, canvas, _, entities := incrementalRenderFixture(b, 10000, false)
	texture := graphics.CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 10, 5)))
	b.Cleanup(func() { _ = texture.Dispose() })
	*ecs.GetMut[Sprite2D](w, entities[0]) = Sprite2DFromTexture(texture)
	updates := []graphics.TextureRegionUpdate{{Width: 1, Height: 1, Pixels: []byte{255, 0, 0, 255}}}
	step := func() {
		updates[0].Pixels[0] ^= 255
		texture.UpdateRegions(updates)
		incrementalFrame(b, w, canvas)
	}
	for range 3 {
		step()
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		step()
	}
}
