package scene

import (
	"image"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/go-gl/mathgl/mgl32"
)

func TestParticleSnapshotsReleaseUnusedTextures(t *testing.T) {
	texture := new(graphics.Texture)
	emitter := ParticleEmitter2D{
		Appearances: []ParticleAppearance2D{{Texture: texture}, {Texture: texture}},
		particles:   []particleState2D{{texture: texture}, {texture: texture}},
	}
	var snapshots renderSnapshots2D
	snapshots.addEmitter(emitter)
	for _, count := range []int{1, 0} {
		snapshots.reset()
		emitter.Appearances = emitter.Appearances[:count]
		emitter.particles = emitter.particles[:count]
		snapshot := snapshots.addEmitter(emitter)
		snapshots.releaseUnused()
		if count > 0 && (snapshot.Appearances[0].Texture != texture || snapshot.particles[0].texture != texture) {
			t.Fatal("active snapshot lost its texture")
		}
		for _, value := range snapshots.particleAppearances[count:cap(snapshots.particleAppearances)] {
			if value.Texture != nil {
				t.Fatal("unused appearance retains a texture")
			}
		}
		for _, value := range snapshots.particles[count:cap(snapshots.particles)] {
			if value.texture != nil {
				t.Fatal("unused particle retains a texture")
			}
		}
	}
}

func TestRetainedDynamicNodesReleasedAfterRemoval(t *testing.T) {
	for _, mode := range []string{"despawn", "component", "visibility"} {
		t.Run(mode, func(t *testing.T) {
			world, canvas, _, _ := incrementalRenderFixture(t, 100, false)
			RegisterTilemapRenderer2D(world)
			texture := graphics.CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 2, 2)))
			t.Cleanup(func() { _ = texture.Dispose() })
			spawner := ecs.NewSpawner3[Tilemap2D, UIGlobalTransform, Visibility](world)
			cache := ecs.ScopeValue[retainedRender2D, retainedRender2D](world)
			for range 32 {
				entity := spawner.Spawn(Tilemap2D{Texture: texture, Tiles: []int{0}, Width: 1, Height: 1, TileWidth: 2, TileHeight: 2, Columns: 1, Alpha: 1}, UIGlobalTransform{Matrix: mgl32.Ident4()}, Visibility{Visible: true})
				incrementalFrame(t, world, canvas)
				node := cache.nodes[entity]
				if node == nil || node.dynamic == 0 {
					t.Fatal("dynamic entity was not cached")
				}
				switch mode {
				case "despawn":
					world.Despawn(entity)
				case "component":
					ecs.Remove[Tilemap2D](world, entity)
				case "visibility":
					ecs.Remove[Visibility](world, entity)
				}
				incrementalFrame(t, world, canvas)
				if cache.nodes[entity] != nil || len(cache.nodes) != 100 || node.snapshots != nil {
					t.Fatal("removed dynamic entity retains its render node")
				}
			}
			if len(cache.nodeBlocks) != 2 {
				t.Fatal("dynamic churn grew the node pool")
			}
		})
	}
}
