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

func TestSparseMaterialSnapshotsOwnValuesAcrossGrowthAndCompaction(t *testing.T) {
	node := retainedEntity2D{first: 0}
	items := make([]renderItem2D, 128)
	for i := range items {
		material := graphics.Material2DData{Custom: [4][4]float32{{float32(i/2 + 1)}}}
		items[i] = node.snapshot(renderItem2D{material2D: &material})
		material.Custom[0][0] = -1
		if i > 0 {
			node.extra = append(node.extra, i)
		}
	}
	if len(node.snapshots.materials) != len(items)/2 || items[0].material2D != items[1].material2D {
		t.Fatal("adjacent material values were not shared")
	}
	items = items[:8]
	node.extra = node.extra[:7]
	node.snapshots.materials = node.snapshots.materials[:4]
	node.compact(items)
	if cap(node.snapshots.materials) > 8 {
		t.Fatal("material snapshots retained peak capacity")
	}
	for i, item := range items {
		if item.materialData().Custom[0][0] != float32(i/2+1) {
			t.Fatal("growth or compaction invalidated the owned material")
		}
	}
	var ordinary retainedEntity2D
	if item := ordinary.snapshot(renderItem2D{}); item.material2D != nil || ordinary.snapshots != nil {
		t.Fatal("default material allocated optional storage")
	}
}

func TestRetainedNestedStorageShrinksWhileEntitySurvives(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 100, false)
	RegisterSpriteLayersRenderer2D(world)
	RegisterParticleRenderer2D(world)
	RegisterTilemapRenderer2D(world)
	texture := graphics.CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 4, 4)))
	t.Cleanup(func() { _ = texture.Dispose() })
	layers := make([]SpriteLayer2D, 1024)
	for i := range layers {
		layers[i] = SpriteLayer2D{Visible: true, Sprite: Sprite2D{Texture: texture, Width: 4, Height: 4, Alpha: 1, Tint: graphics.White}}
	}
	emitter := ParticleEmitter2D{Texture: texture, particles: make([]particleState2D, 4096), Colors: make([]graphics.ColorRGBA, 4096), Appearances: make([]ParticleAppearance2D, 4096)}
	emitter.particles[0].x = 17
	tilemap := Tilemap2D{Texture: texture, Tiles: make([]int, 4096), Width: 4096, Height: 1, TileWidth: 1, TileHeight: 1, Columns: 4, Alpha: 1, Revision: 1}
	for _, err := range []error{world.Insert(entities[0], SpriteLayers2D{Layers: layers}), world.Insert(entities[0], emitter), world.Insert(entities[0], tilemap)} {
		if err != nil {
			t.Fatal(err)
		}
	}
	incrementalFrame(t, world, canvas)
	cache := world.ScopeValue[retainedRender2D, retainedRender2D]()
	node := cache.nodes[entities[0]]
	_, peak := node.storage()
	world.GetMut[SpriteLayers2D](entities[0]).Layers = layers[:2]
	e := world.GetMut[ParticleEmitter2D](entities[0])
	e.particles, e.Colors, e.Appearances = e.particles[:2], e.Colors[:2], e.Appearances[:2]
	tiles := world.GetMut[Tilemap2D](entities[0])
	tiles.Tiles, tiles.Width, tiles.Revision = tiles.Tiles[:2], 2, 2
	for range reuse.RetentionFrames*2 + 4 {
		incrementalFrame(t, world, canvas)
	}
	if cap(node.sprites) > 6 || cap(node.extra) > 12 || cap(node.snapshots.particles) > 4 || cap(node.snapshots.particleColors) > 4 || cap(node.snapshots.particleAppearances) > 4 || cap(node.snapshots.tilemaps[0].value.Tiles) > 4 {
		t.Fatal("surviving entity retained nested peak capacity")
	}
	for position := range node.positions {
		item := &cache.items[position]
		if item.snapshot[ParticleEmitter2D]() != nil && (len(item.snapshot[ParticleEmitter2D]().particles) != 2 || item.snapshot[ParticleEmitter2D]().particles[0].x != 17 || &item.snapshot[ParticleEmitter2D]().particles[0] != &node.snapshots.particles[0]) {
			t.Fatal("particle snapshot lost its contents or retained old backing storage")
		}
		if item.snapshot[Tilemap2D]() != nil && item.snapshot[Tilemap2D]() != &node.snapshots.tilemaps[0].value {
			t.Fatal("tilemap item retained an old snapshot pointer")
		}
	}
	dependency := cache.textures[texture]
	if dependency == nil || cap(dependency.positions) > 16 {
		t.Fatal("texture position index retained the old population")
	}
	if allocations := testing.AllocsPerRun(60, func() { incrementalFrame(t, world, canvas) }); allocations != 0 {
		t.Fatalf("reclaimed dynamic snapshots allocated %v times", allocations)
	}
	_, after := node.storage()
	t.Logf("surviving entity nested storage: %d -> %d bytes", peak, after)
}

func TestRetainedNodePoolCompactsScatteredSurvivors(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 4096, false)
	incrementalFrame(t, world, canvas)
	cache := world.ScopeValue[retainedRender2D, retainedRender2D]()
	survivors := make(map[ecs.Entity]*Sprite2D)
	for i, entity := range entities {
		if i%64 == 0 {
			survivors[entity] = cache.items[i].snapshot[Sprite2D]()
		} else {
			world.Despawn(entity)
		}
	}
	incrementalFrame(t, world, canvas)
	for _, item := range cache.items {
		survivors[item.entity] = item.snapshot[Sprite2D]()
	}
	for range reuse.RetentionFrames*2 + 4 {
		incrementalFrame(t, world, canvas)
	}
	if len(cache.nodes) != 64 || len(cache.nodeBlocks) > 2 {
		t.Fatalf("scattered survivors retained %d blocks for %d nodes", len(cache.nodeBlocks), len(cache.nodes))
	}
	for _, item := range cache.items {
		if item.snapshot[Sprite2D]() != survivors[item.entity] || item.snapshot[Sprite2D]().Shape.Fill != graphics.White {
			t.Fatal("node relocation moved or invalidated a drawing snapshot")
		}
	}
	world.GetMut[Sprite2D](entities[0]).Shape.Fill = graphics.Black
	incrementalFrame(t, world, canvas)
	if cache.stats.Extracted != 1 || cache.items[0].snapshot[Sprite2D]().Shape.Fill != graphics.Black {
		t.Fatal("compacted node map lost sparse updates")
	}
}

func TestTransformOutputStorageSizes(t *testing.T) {
	if ordinary, ui := unsafe.Sizeof(transformNode{}), unsafe.Sizeof(uiTransformNode{}); ordinary > 128 || ui > 416 {
		t.Fatalf("transform node sizes = %d/%d bytes", ordinary, ui)
	}
	if size := unsafe.Sizeof(Spine2D{}); size != unsafe.Sizeof((*mgl32.Mat4)(nil)) {
		t.Fatalf("Spine2D retains per-instance geometry: %d bytes", size)
	}
	t.Logf("transform/UI/Spine2D record sizes: %d/%d/%d bytes", unsafe.Sizeof(transformNode{}), unsafe.Sizeof(uiTransformNode{}), unsafe.Sizeof(Spine2D{}))
}
