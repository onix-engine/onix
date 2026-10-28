package scene

import (
	"image"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

func incrementalFrame(t testing.TB, world *ecs.World, canvas *graphics.Canvas2D) {
	t.Helper()
	context := &ecs.SystemContext{World: world}
	if err := extractRender2D(context); err != nil {
		t.Fatal(err)
	}
	if err := renderCameras(context); err != nil {
		t.Fatal(err)
	}
	if err := canvas.FinishFrame(); err != nil {
		t.Fatal(err)
	}
}

func TestRetainedRender2DSparseSnapshotsAndOrder(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 100, false)
	incrementalFrame(t, world, canvas)
	state := renderWorld(world)
	unchanged := state.Items2D[50].sprite
	ecs.GetMut[Sprite2D](world, entities[0]).Shape.Fill = graphics.Black
	incrementalFrame(t, world, canvas)
	if state.Preparation2D.Extracted != 1 || state.Preparation2D.Reused != 99 || state.Preparation2D.Sorted || state.Items2D[50].sprite != unchanged {
		t.Fatalf("sparse extraction rebuilt unrelated snapshots: %+v", state.Preparation2D)
	}
	if state.Items2D[0].sprite.Shape.Fill != graphics.Black || state.Items2D[50].sprite.Shape.Fill != graphics.White {
		t.Fatal("snapshot contents were overwritten")
	}
	incrementalFrame(t, world, canvas)
	if state.Preparation2D.Extracted != 0 || state.Composite2D.DirtyTiles != 0 {
		t.Fatal("stable frame did work")
	}
	ecs.GetMut[UIStackIndex](world, entities[0]) // Even a no-op write is safe.
	ecs.SetIfChanged(world, entities[0], UIStackIndex(150))
	incrementalFrame(t, world, canvas)
	if state.Items2D[len(state.Items2D)-1].entity != entities[0] {
		t.Fatal("retained order missed stacking change")
	}
	for range 3 {
		incrementalFrame(t, world, canvas)
	}
	if allocations := testing.AllocsPerRun(20, func() {
		sprite := ecs.GetMut[Sprite2D](world, entities[0])
		sprite.Shape.Fill.R = 1 - sprite.Shape.Fill.R
		incrementalFrame(t, world, canvas)
	}); allocations != 0 {
		t.Fatalf("sparse frames allocated %v times", allocations)
	}
}

func TestRetainedRender2DVisibilityStructureAndScopes(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 100, false)
	incrementalFrame(t, world, canvas)
	ecs.GetMut[Visibility](world, entities[0]).Visible = false
	incrementalFrame(t, world, canvas)
	if len(renderWorld(world).Items2D) != 99 {
		t.Fatal("hidden primitive retained")
	}
	ecs.GetMut[Visibility](world, entities[0]).Visible = true
	incrementalFrame(t, world, canvas)
	if len(renderWorld(world).Items2D) != 100 {
		t.Fatal("visible primitive not restored")
	}
	var scope ecs.QueryScope
	scope.Reset(entities[:1])
	world.SetQueryScope(&scope)
	incrementalFrame(t, world, canvas)
	if len(renderWorld(world).Items2D) != 1 {
		t.Fatal("scope ignored")
	}
	scope.Reset(entities[:2])
	incrementalFrame(t, world, canvas)
	if len(renderWorld(world).Items2D) != 2 {
		t.Fatal("scope revision ignored")
	}
	world.SetQueryScope(nil)
	incrementalFrame(t, world, canvas)
	if len(renderWorld(world).Items2D) != 100 {
		t.Fatal("scope switch lost previous cache")
	}
	world.Despawn(entities[0])
	incrementalFrame(t, world, canvas)
	if len(renderWorld(world).Items2D) != 99 {
		t.Fatal("deleted primitive retained")
	}
}

func TestComposite2DDamageMoveRemovalAndOcclusion(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 1000, false)
	incrementalFrame(t, world, canvas)
	state := renderWorld(world)
	ecs.GetMut[Sprite2D](world, entities[0]).Shape.Fill = graphics.Black
	incrementalFrame(t, world, canvas)
	if state.Composite2D.DirtyTiles != 1 || state.Composite2D.RasterPixels != 128*128 {
		t.Fatalf("local damage=%+v", state.Composite2D)
	}
	ecs.GetMut[UIGlobalTransform](world, entities[0]).Matrix[12] = 600
	incrementalFrame(t, world, canvas)
	if state.Composite2D.DirtyTiles != 2 {
		t.Fatalf("move must damage both old and new tiles: %+v", state.Composite2D)
	}
	world.Despawn(entities[0])
	incrementalFrame(t, world, canvas)
	if state.Composite2D.DirtyTiles+state.Composite2D.ReusedTiles != 1 {
		t.Fatalf("removal must restore background: %+v", state.Composite2D)
	}
	coveredWorld, coveredCanvas, driver, covered := incrementalRenderFixture(t, 1000, true)
	incrementalFrame(t, coveredWorld, coveredCanvas)
	driver.shapes, driver.sprites = 0, 0
	ecs.GetMut[Sprite2D](coveredWorld, covered[0]).Shape.Fill = graphics.Black
	incrementalFrame(t, coveredWorld, coveredCanvas)
	if stats := renderWorld(coveredWorld).Composite2D; stats.DirtyTiles != 0 || stats.Occluded != 1000 || driver.shapes != 0 || driver.sprites != 1 {
		t.Fatalf("covered mutation repainted: %+v, shapes=%d sprites=%d", stats, driver.shapes, driver.sprites)
	}
}

func TestComposite2DScrollReusesPropertiesAndPixels(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 1000, false)
	incrementalFrame(t, world, canvas)
	state := renderWorld(world)
	snapshot := state.Items2D[0].sprite
	for _, entity := range entities {
		ecs.GetMut[UIGlobalTransform](world, entity).Matrix[12] += 128
	}
	incrementalFrame(t, world, canvas)
	if state.Preparation2D.Extracted != 0 || state.Preparation2D.PropertiesUpdated != 1000 || state.Items2D[0].sprite != snapshot {
		t.Fatalf("translation rebuilt content: %+v", state.Preparation2D)
	}
	if state.Composite2D.ReusedTiles == 0 || state.Composite2D.DirtyTiles >= state.Composite2D.Tiles/2 || state.Composite2D.Direct {
		t.Fatalf("scroll did not reuse tiles: %+v", state.Composite2D)
	}
	for range 6 {
		for _, entity := range entities {
			ecs.GetMut[UIGlobalTransform](world, entity).Matrix[12] -= 128
		}
		incrementalFrame(t, world, canvas)
		for _, entity := range entities {
			ecs.GetMut[UIGlobalTransform](world, entity).Matrix[12] += 128
		}
		incrementalFrame(t, world, canvas)
	}
	if allocations := testing.AllocsPerRun(20, func() {
		for _, entity := range entities {
			x := &ecs.GetMut[UIGlobalTransform](world, entity).Matrix[12]
			*x += 128
			if *x >= 1280 {
				*x -= 1280
			}
		}
		incrementalFrame(t, world, canvas)
	}); allocations != 0 {
		t.Fatalf("warmed scroll allocated %v times", allocations)
	}
}

func TestRetainedRender2DClipTextureAndDenseRecovery(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 100, false)
	incrementalFrame(t, world, canvas)
	state := renderWorld(world)
	transform := ecs.GetMut[UIGlobalTransform](world, entities[0])
	transform.clip = clipRect2D{valid: true, minX: 0, minY: 0, maxX: 5, maxY: 5}
	incrementalFrame(t, world, canvas)
	if state.Items2D[0].paint.bounds.Max.X != 5 || state.Composite2D.DirtyTiles != 1 {
		t.Fatal("clip-only damage missed", state.Composite2D)
	}
	texture := graphics.CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 2, 2)))
	t.Cleanup(func() { _ = texture.Dispose() })
	sprite := ecs.GetMut[Sprite2D](world, entities[0])
	sprite.Shape = Shape2D{}
	sprite.Texture = texture
	sprite.Tint = graphics.White
	incrementalFrame(t, world, canvas)
	texture.UpdateRegions([]graphics.TextureRegionUpdate{{Width: 1, Height: 1, Pixels: []byte{255, 0, 0, 255}}})
	incrementalFrame(t, world, canvas)
	if state.Preparation2D.Extracted != 0 || state.Preparation2D.PaintUpdated != 1 || state.Composite2D.DirtyTiles != 1 {
		t.Fatal("texture mutation missed", state.Preparation2D, state.Composite2D)
	}
	if err := texture.Dispose(); err != nil {
		t.Fatal(err)
	}
	incrementalFrame(t, world, canvas)
	if state.Composite2D.DirtyTiles+state.Composite2D.ReusedTiles != 1 {
		t.Fatal("disposed texture retained", state.Composite2D)
	}
	for _, entity := range entities {
		ecs.GetMut[Sprite2D](world, entity).Alpha = .5
	}
	incrementalFrame(t, world, canvas)
	if !ecs.ScopeValue[retainedRender2D, retainedRender2D](world).direct {
		t.Fatal("dense frame retained")
	}
	incrementalFrame(t, world, canvas)
	if ecs.ScopeValue[retainedRender2D, retainedRender2D](world).direct {
		t.Fatal("quiet frame failed to recover")
	}
	incrementalFrame(t, world, canvas)
	if state.Preparation2D.Extracted != 0 {
		t.Fatal("quiet frame kept extracting")
	}
}

func TestComposite2DConservativeOpaqueBounds(t *testing.T) {
	sprite := Sprite2D{Shape: RoundedRectShape2D(graphics.White, 0), Alpha: 1}
	item := newSpriteRenderItem2D(0, 1, &sprite, graphics.Material2DData{}, AnchorTopLeft, mgl32.Ident4(), clipRect2D{}, 256, 256, true, 0)
	if got := preparePaintItem2D(&item).opaque; got != image.Rect(0, 0, 256, 256) {
		t.Fatal("opaque rectangle not recognized", got)
	}
	for _, mutate := range []func(){
		func() { sprite.Shape.Radius = 1 },
		func() { sprite.Alpha = .5 },
		func() { sprite.Shape.Fill.A = .5 },
		func() { sprite.Shape.BorderWidth = 2 },
		func() { item.transform = mgl32.HomogRotate3DZ(.1) },
		func() { item.clip.hasRounded = true },
	} {
		sprite.Shape, sprite.Alpha = RoundedRectShape2D(graphics.White, 0), 1
		item.transform, item.clip = mgl32.Ident4(), clipRect2D{}
		mutate()
		if !preparePaintItem2D(&item).opaque.Empty() {
			t.Fatal("unsafe occlusion accepted")
		}
	}
	sprite.Shape, sprite.Alpha = RoundedRectShape2D(graphics.White, 0), 1
	item.transform = mgl32.Ident4()
	item.clip = clipRect2D{valid: true, minX: .25, minY: .25, maxX: 127.75, maxY: 127.75}
	paint := preparePaintItem2D(&item)
	if paint.opaque != image.Rect(1, 1, 127, 127) {
		t.Fatal("fractional clip expanded opaque coverage", paint.opaque)
	}
	item.clip.hasRounded = true
	paint.resolve(&item)
	if !paint.opaque.Empty() {
		t.Fatal("new rounded clip retained opaque coverage")
	}
	item.kind = renderItemTextLayer2D
	item.text = &Text2D{rasterScale: 1.1}
	if preparePaintItem2D(&item).cacheable {
		t.Fatal("unsafe glyph raster phase accepted")
	}
}

func TestComposite2DBudgetDenseChangesAndResize(t *testing.T) {
	world, canvas, driver, entities := incrementalRenderFixture(t, 100, false)
	incrementalFrame(t, world, canvas)
	state := renderWorld(world)
	ecs.InsertResource(world, CompositeSettings2D{MaxCacheBytes: -1})
	incrementalFrame(t, world, canvas)
	if state.Composite2D.CacheBytes != 0 {
		t.Fatal("disabled budget retained cache")
	}
	ecs.RemoveResource[CompositeSettings2D](world)
	for _, entity := range entities {
		ecs.GetMut[Sprite2D](world, entity).Shape.Fill = graphics.Black
	}
	incrementalFrame(t, world, canvas)
	if !ecs.ScopeValue[retainedRender2D, retainedRender2D](world).direct {
		t.Fatal("dense changes did not use sequential path")
	}
	incrementalFrame(t, world, canvas)
	canvas.Resize(640, 360)
	driver.viewport = render.Rect{Width: 640, Height: 360}
	incrementalFrame(t, world, canvas)
	if state.Composite2D.CacheBytes != 640*360*4 {
		t.Fatalf("resize did not rebuild target: %+v", state.Composite2D)
	}
}

func TestComposite2DIneligibleOutputsAndReleasedText(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 100, false)
	incrementalFrame(t, world, canvas)
	state := renderWorld(world)
	state.Cameras[0].Camera.ClearColor.A = .5
	incrementalFrame(t, world, canvas)
	if !state.Composite2D.Direct || state.Composite2D.CacheBytes != 0 {
		t.Fatal("transparent output cached")
	}
	state.Cameras[0].Camera.ClearColor.A = 1
	ecs.Insert(world, entities[0], Shader[graphics.Material2DShader]{Program: new(graphics.Material2DShader)})
	incrementalFrame(t, world, canvas)
	if !state.Composite2D.Direct || state.Composite2D.CacheBytes != 0 {
		t.Fatal("custom shader cached")
	}
	ecs.Remove[Shader[graphics.Material2DShader]](world, entities[0])
	RegisterTextRenderer2D(world)
	font := outputTestFont(t)
	pool := NewRenderOutputPool(1)
	handle, err := pool.Text(Text2D{Value: "Retained output", Font: font, FontSize: 18, DPI: 72, Color: graphics.White, Alpha: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ecs.Insert(world, entities[0], RenderOutput2D{Handle: handle}); err != nil {
		t.Fatal(err)
	}
	incrementalFrame(t, world, canvas)
	count := len(state.Items2D)
	if err := handle.Release(); err != nil {
		t.Fatal(err)
	}
	incrementalFrame(t, world, canvas)
	if len(state.Items2D) >= count || state.Preparation2D.Extracted == 0 {
		t.Fatal("released immutable text retained")
	}
}

func TestComposite2DOnlyFirstCameraOwnsOpaqueBackground(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 100, false)
	state := renderWorld(world)
	second := state.Cameras[0]
	second.Entity = 2
	state.Cameras = append(state.Cameras, second)
	if err := ecs.Insert(world, entities[0], UITargetCamera{Entity: 2}); err != nil {
		t.Fatal(err)
	}
	incrementalFrame(t, world, canvas)
	if len(state.compositors2D) != 1 {
		t.Fatal("later camera erased previous output with an opaque cache")
	}
}
