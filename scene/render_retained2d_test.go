package scene

import (
	"image"
	"testing"
	"unsafe"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/internal/reuse"
	"github.com/go-gl/mathgl/mgl32"
)

func TestRetainedRender2DUnusedDynamicInputs(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 100, false)
	RenderWorldFor(world).RegisterMaterial2D[customMaterial2DTest]()
	RegisterParticleRenderer2D(world)
	RegisterTilemapRenderer2D(world)
	RegisterSpineRenderer2D(world)
	RegisterSpriteLayersRenderer2D(world)
	RegisterSpriteTrailRenderer2D(world)
	bundle := ecs.NewBundle(7)
	bundle.Put(ParticleEmitter2D{})
	bundle.Put(Tilemap2D{})
	bundle.Put(Spine2D{})
	bundle.Put(SpriteLayers2D{})
	bundle.Put(SpriteTrail2D{})
	bundle.Put(UIGlobalTransform{Matrix: mgl32.Ident4()})
	bundle.Put(Visibility{Visible: true})
	empty := world.Spawn(bundle)
	if err := world.Insert(entities[0], ParticleEmitter2D{}); err != nil {
		t.Fatal(err)
	}
	for _, remove := range []bool{false, true} {
		if remove {
			world.Despawn(empty)
		}
		incrementalFrame(t, world, canvas)
		incrementalFrame(t, world, canvas)
		state := RenderWorldFor(world)
		if state.Preparation2D.Extracted != 0 || state.Preparation2D.Reused != 100 || state.Composite2D.Direct {
			t.Fatalf("unused dynamic inputs disabled caching: %+v / %+v", state.Preparation2D, state.Composite2D)
		}
	}
	material := customMaterial2DTest{value: .25}
	if err := world.Insert(entities[0], MeshMaterial2D[customMaterial2DTest]{Material: &material}); err != nil {
		t.Fatal(err)
	}
	incrementalFrame(t, world, canvas)
	unchanged := RenderWorldFor(world).Items2D[50].sprite
	material.value = .75 // Edits through the material pointer still update.
	incrementalFrame(t, world, canvas)
	state := RenderWorldFor(world)
	if state.Preparation2D.Extracted != 1 || state.Items2D[0].material2D.Custom[0][0] != .75 || state.Items2D[50].sprite != unchanged {
		t.Fatalf("material edit rebuilt unrelated content: %+v", state.Preparation2D)
	}
	incrementalFrame(t, world, canvas)
	if state.Preparation2D.Extracted != 0 {
		t.Fatal("unchanged material kept extracting")
	}
	world.Remove[MeshMaterial2D[customMaterial2DTest]](entities[0])
	incrementalFrame(t, world, canvas)
	if state.Items2D[0].material2D != (graphics.Material2DData{}) {
		t.Fatal("removed material retained")
	}
}

func TestRetainedRender2DMixedTilemapSnapshots(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 100, false)
	RegisterTilemapRenderer2D(world)
	texture := graphics.CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 4, 2)))
	t.Cleanup(func() { _ = texture.Dispose() })
	tilemap := Tilemap2D{Texture: texture, Tiles: []int{0, 1}, Width: 2, Height: 1, TileWidth: 2, TileHeight: 2, Columns: 2, Alpha: 1}
	if err := world.Insert(entities[0], tilemap); err != nil {
		t.Fatal(err)
	}
	incrementalFrame(t, world, canvas)
	state := RenderWorldFor(world)
	unchanged := state.Items2D[50].sprite
	tilemap.Tiles[0] = 1 // Revision zero allows direct tile edits.
	incrementalFrame(t, world, canvas)
	if state.Preparation2D.Extracted != 2 || state.Items2D[50].sprite != unchanged {
		t.Fatalf("dynamic entity invalidated scene: %+v", state.Preparation2D)
	}
	var snapshot *Tilemap2D
	for i := range state.Items2D {
		if state.Items2D[i].tilemap != nil {
			snapshot = state.Items2D[i].tilemap
		}
	}
	if snapshot == nil || snapshot.Tiles[0] != 1 {
		t.Fatal("tile edit missed")
	}
	tilemap.Tiles[0] = 0
	if snapshot.Tiles[0] != 1 {
		t.Fatal("retained snapshot aliases source tiles")
	}
	incrementalFrame(t, world, canvas)
	if snapshot.Tiles[0] != 0 {
		t.Fatal("reused tile snapshot did not update")
	}
	if allocations := testing.AllocsPerRun(20, func() { incrementalFrame(t, world, canvas) }); allocations != 0 {
		t.Fatalf("mixed dynamic frames allocated %v times", allocations)
	}
}

func TestRetainedRender2DNonRenderingDynamicCandidates(t *testing.T) {
	world, canvas, _, _ := incrementalRenderFixture(t, 100, false)
	RegisterSpriteLayersRenderer2D(world)
	texture := graphics.CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 2, 2)))
	t.Cleanup(func() { _ = texture.Dispose() })
	spawner := world.NewSpawner3[SpriteLayers2D, UIGlobalTransform, Visibility]()
	RegisterTilemapRenderer2D(world)
	tiles := world.NewSpawner3[Tilemap2D, UIGlobalTransform, Visibility]()
	tiles.Spawn(Tilemap2D{Texture: texture, Tiles: []int{0}, Width: 1, Height: 1, TileWidth: 2, TileHeight: 2, Columns: 1, Alpha: 1}, UIGlobalTransform{Matrix: mgl32.Ident4(), clip: clipRect2D{valid: true}}, Visibility{Visible: true})
	layer := SpriteLayer2D{Sprite: Sprite2D{Texture: texture, Width: 2, Height: 2, Alpha: 1, Tint: graphics.White}}
	var entity ecs.Entity
	for range 100 {
		entity = spawner.Spawn(SpriteLayers2D{Layers: []SpriteLayer2D{layer}}, UIGlobalTransform{Matrix: mgl32.Ident4()}, Visibility{Visible: true})
	}
	incrementalFrame(t, world, canvas)
	cache := world.ScopeValue[retainedRender2D, retainedRender2D]()
	generation := cache.generation
	for range 3 {
		incrementalFrame(t, world, canvas)
		if cache.stats.Extracted != 0 || cache.stats.Reused != 100 || cache.generation != generation {
			t.Fatalf("hidden layers invalidated static content: %+v", cache.stats)
		}
	}
	if allocations := testing.AllocsPerRun(20, func() { incrementalFrame(t, world, canvas) }); allocations != 0 {
		t.Fatalf("non-rendering candidates allocated %v times", allocations)
	}
	// The empty snapshot must still notice when a layer becomes drawable.
	world.GetRef[SpriteLayers2D](entity).Layers[0].Visible = true
	incrementalFrame(t, world, canvas)
	if len(RenderWorldFor(world).Items2D) != 101 {
		t.Fatal("newly visible layer missed")
	}
	world.GetRef[SpriteLayers2D](entity).Layers[0].Visible = false
	incrementalFrame(t, world, canvas)
	incrementalFrame(t, world, canvas)
	if cache.stats.Extracted != 0 || cache.stats.Reused != 100 {
		t.Fatal("hidden layer retained output or kept invalidating content")
	}
}

func TestRetainedRender2DDenseDynamicReturnsToStatic(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 100, false)
	RegisterSpriteLayersRenderer2D(world)
	texture := graphics.CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 2, 2)))
	t.Cleanup(func() { _ = texture.Dispose() })
	for _, entity := range entities {
		layer := SpriteLayer2D{Visible: true, Sprite: Sprite2D{Texture: texture, Width: 2, Height: 2, Alpha: 1, Tint: graphics.White}}
		if err := world.Insert(entity, SpriteLayers2D{Layers: []SpriteLayer2D{layer}}); err != nil {
			t.Fatal(err)
		}
	}
	incrementalFrame(t, world, canvas)
	cache := world.ScopeValue[retainedRender2D, retainedRender2D]()
	for frame := range reuse.RetentionFrames * 2 {
		incrementalFrame(t, world, canvas)
		if !cache.direct || len(RenderWorldFor(world).Items2D) != 200 {
			t.Fatal("dense dynamic content did not keep sequential extraction")
		}
		if frame >= reuse.RetentionFrames && (len(cache.nodes) != 0 || len(cache.items) != 0 || len(cache.paints) != 0 || len(cache.nodeBlocks) != 0) {
			t.Fatal("direct frames rebuilt retired storage")
		}
	}
	if allocations := testing.AllocsPerRun(60, func() { incrementalFrame(t, world, canvas) }); allocations != 0 {
		t.Fatalf("sustained dynamic frames allocated %v times", allocations)
	}
	for _, entity := range entities {
		world.GetRef[SpriteLayers2D](entity).Layers[0].Visible = false
	}
	for range 3 {
		incrementalFrame(t, world, canvas)
	}
	if cache.direct || cache.stats.Extracted != 0 || cache.stats.Reused != 100 {
		t.Fatalf("finished dynamic content did not restore static caching: %+v", cache.stats)
	}
	if allocations := testing.AllocsPerRun(60, func() { incrementalFrame(t, world, canvas) }); allocations != 0 {
		t.Fatalf("restored static frames allocated %v times", allocations)
	}
}

func TestRetainedRender2DReclaimsPeakStorage(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 10000, false)
	incrementalFrame(t, world, canvas)
	cache := world.ScopeValue[retainedRender2D, retainedRender2D]()
	scratch := world.FrameScratch[renderItems2DScratch, renderItem2D]()
	if cap(scratch.Values) != 0 {
		t.Fatal("full retained collection allocated a second render item array")
	}
	// Warm buffers also used by direct drawing and camera filtering before
	// returning to a small retained scene.
	collectRenderItems2D(canvas, world, false)
	state := RenderWorldFor(world)
	visibleRenderItems2D(world, state.Items2D, state.Cameras[0], state.Cameras[0].Entity)
	visible := world.FrameScratch[visibleRenderItems2DScratch, uint32]()
	bufferBytes := func() uintptr {
		return uintptr(cap(cache.items)+cap(scratch.Values))*unsafe.Sizeof(renderItem2D{}) +
			uintptr(cap(cache.paints))*unsafe.Sizeof(paintItem2D{}) +
			uintptr(len(cache.nodeBlocks)*64)*unsafe.Sizeof(retainedEntity2D{}) +
			uintptr(cap(visible.Values))*unsafe.Sizeof(uint32(0))
	}
	peak := bufferBytes()
	owner := cache.nodes[entities[0]]
	for _, entity := range entities[100:] {
		world.Despawn(entity)
	}
	for range reuse.RetentionFrames*2 + 4 {
		incrementalFrame(t, world, canvas)
	}
	if len(cache.items) != 100 || cap(cache.items) > 200 || cap(cache.paints) > 200 || len(cache.nodeBlocks) > 5 || cap(scratch.Values) != 0 || cap(visible.Values) != 0 {
		t.Fatalf("peak storage retained: items=%d/%d paints=%d blocks=%d scratch=%d camera=%d", len(cache.items), cap(cache.items), cap(cache.paints), len(cache.nodeBlocks), cap(scratch.Values), cap(visible.Values))
	}
	if snapshots := world.FrameValue[renderSnapshots2DScratch, renderSnapshots2D](); snapshots.capacity() != 0 {
		t.Fatalf("idle snapshot storage retained %d slots", snapshots.capacity())
	}
	if cache.nodes[entities[0]] != owner || !owner.allocated {
		t.Fatal("reclamation moved a live snapshot owner")
	}
	for i := range cache.items {
		item := &cache.items[i]
		if item.paint != &cache.paints[i] || *item.paint != preparePaintItem2D(item) {
			t.Fatalf("item %d retained stale paint data after reclamation", i)
		}
	}
	for _, view := range state.compositors2D {
		if cap(view.coverage) > 200 {
			t.Fatal("compositor retained peak coverage")
		}
		capacity := 0
		for _, tile := range view.tiles {
			capacity += cap(tile.candidates)
		}
		if capacity > len(view.tiles)*32 {
			t.Fatalf("compositor retained %d candidate slots", capacity)
		}
	}
	world.GetMut[Sprite2D](entities[0]).Shape.Fill = graphics.Black
	incrementalFrame(t, world, canvas)
	if state.Preparation2D.Extracted != 1 || state.Composite2D.DirtyTiles == 0 || state.Items2D[0].sprite.Shape.Fill != graphics.Black {
		t.Fatal("reclaimed scene failed to repaint a sparse edit")
	}
	if allocations := testing.AllocsPerRun(60, func() { incrementalFrame(t, world, canvas) }); allocations != 0 {
		t.Fatalf("reclaimed stable scene allocated %v times", allocations)
	}
	t.Logf("primary CPU buffers after 10000 -> 100 entities: %d -> %d bytes (excludes maps, nested snapshots and GPU storage)", peak, bufferBytes())
}

func TestRetainedRender2DDenseWritesReleaseAndRestore(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 100, false)
	incrementalFrame(t, world, canvas)
	cache := world.ScopeValue[retainedRender2D, retainedRender2D]()
	frame := func() {
		for _, entity := range entities {
			sprite := world.GetMut[Sprite2D](entity)
			sprite.Shape.Fill.R = 1 - sprite.Shape.Fill.R
		}
		incrementalFrame(t, world, canvas)
	}
	for range reuse.RetentionFrames * 2 {
		frame()
	}
	if !cache.direct || cache.nodes != nil || cache.items != nil || cache.paints != nil || cache.nodeBlocks != nil {
		t.Fatal("dense writes retained their obsolete scene")
	}
	if allocations := testing.AllocsPerRun(60, frame); allocations != 0 {
		t.Fatalf("sustained dense writes allocated %v times", allocations)
	}
	for range reuse.RetentionFrames * 2 {
		incrementalFrame(t, world, canvas)
	}
	if cache.direct || cache.stats.Extracted != 0 || cache.stats.Reused != 100 {
		t.Fatal("finished dense writes failed to restore retention")
	}
	if snapshots := world.FrameValue[renderSnapshots2DScratch, renderSnapshots2D](); snapshots.capacity() != 0 {
		t.Fatal("restored retained scene pinned direct collector snapshots")
	}
}

func TestRetainedRender2DNodeGrowthAndReuse(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 100, false)
	incrementalFrame(t, world, canvas)
	cache := world.ScopeValue[retainedRender2D, retainedRender2D]()
	owner := cache.nodes[entities[0]]
	spawner := world.NewSpawner3[Sprite2D, UIGlobalTransform, Visibility]()
	for range 200 {
		spawner.Spawn(Sprite2D{Shape: RoundedRectShape2D(graphics.White, 0), Width: 1, Height: 1, Alpha: 1}, UIGlobalTransform{Matrix: mgl32.Ident4()}, Visibility{Visible: true})
	}
	incrementalFrame(t, world, canvas)
	if cache.nodes[entities[0]] != owner {
		t.Fatal("node growth moved a snapshot owner")
	}
	for i := range cache.items {
		if cache.items[i].paint != &cache.paints[i] {
			t.Fatal("paint growth left a stale pointer")
		}
	}
	world.Despawn(entities[0])
	incrementalFrame(t, world, canvas)
	if cache.nodes[entities[0]] != nil || owner.sprites != nil || owner.snapshots != nil || owner.sourceText != nil {
		t.Fatal("removed node retained content")
	}
	blocks := len(cache.nodeBlocks)
	spawner.Spawn(Sprite2D{Shape: RoundedRectShape2D(graphics.Black, 0), Width: 1, Height: 1, Alpha: 1}, UIGlobalTransform{Matrix: mgl32.Ident4()}, Visibility{Visible: true})
	incrementalFrame(t, world, canvas)
	if len(cache.nodeBlocks) != blocks {
		t.Fatal("deleted node storage was not reused")
	}
}

func BenchmarkRetainedRender2DStatic(b *testing.B) {
	for _, mode := range []string{"base", "unused-material", "empty-particle", "removed-particle"} {
		b.Run(mode, func(b *testing.B) {
			world, canvas, _, _ := incrementalRenderFixture(b, 10000, false)
			if mode == "unused-material" {
				RenderWorldFor(world).RegisterMaterial2D[customMaterial2DTest]()
			}
			if mode == "empty-particle" || mode == "removed-particle" {
				RegisterParticleRenderer2D(world)
				spawner := world.NewSpawner1[ParticleEmitter2D]()
				entity := spawner.Spawn(ParticleEmitter2D{})
				if mode == "removed-particle" {
					world.Despawn(entity)
				}
			}
			for range 8 {
				incrementalFrame(b, world, canvas)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				incrementalFrame(b, world, canvas)
			}
			b.StopTimer()
			cache := world.ScopeValue[retainedRender2D, retainedRender2D]()
			bytes := uintptr(cap(cache.items))*unsafe.Sizeof(renderItem2D{}) + uintptr(cap(cache.paints))*unsafe.Sizeof(paintItem2D{})
			for _, block := range cache.nodeBlocks {
				bytes += uintptr(cap(block)) * unsafe.Sizeof(retainedEntity2D{})
			}
			for _, node := range cache.nodes {
				bytes += uintptr(cap(node.sprites))*unsafe.Sizeof(Sprite2D{}) + uintptr(cap(node.extra))*unsafe.Sizeof(int(0))
			}
			b.ReportMetric(float64(bytes), "retained-B")
		})
	}
}
