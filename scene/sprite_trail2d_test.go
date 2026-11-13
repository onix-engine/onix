package scene

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/graphics"
)

func TestSpriteTrail2DSamplesMovementCapsHistoryAndExpires(t *testing.T) {
	world := ecs.NewWorld()
	clock := NewTime()
	clock.Set(20*time.Millisecond, 0, 1)
	world.InsertResource(clock)

	trail := NewSpriteTrail2D()
	trail.Lifetime = 0.1
	trail.Interval = 0
	trail.MinDistance = 5
	trail.MaxSamples = 2
	bundle := ecs.NewBundle(4)
	bundle.Put(Sprite2D{Shape: RoundedRectShape2D(graphics.White, 2), Width: 20, Height: 10, Alpha: 1})
	bundle.Put(trail)
	bundle.Put(GlobalTransform{Matrix: mgl32.Ident4()})
	bundle.Put(Visibility{Visible: true})
	entity := world.Spawn(bundle)

	update := UpdateSpriteTrail2DSystem("")
	if err := update.Run(world); err != nil {
		t.Fatal(err)
	}
	component := world.GetMut[SpriteTrail2D](entity)
	if component.SampleCount() != 1 {
		t.Fatalf("initial sample count = %d, want 1", component.SampleCount())
	}

	global := world.GetMut[GlobalTransform](entity)
	global.Matrix = mgl32.Translate3D(3, 0, 0)
	if err := update.Run(world); err != nil {
		t.Fatal(err)
	}
	if component.SampleCount() != 1 {
		t.Fatalf("movement below minimum distance sampled: %d", component.SampleCount())
	}

	for _, x := range []float32{6, 12} {
		global.Matrix = mgl32.Translate3D(x, 0, 0)
		if err := update.Run(world); err != nil {
			t.Fatal(err)
		}
	}
	if component.SampleCount() != 2 {
		t.Fatalf("capped sample count = %d, want 2", component.SampleCount())
	}
	if component.sample(0).transform[12] != 6 || component.sample(1).transform[12] != 12 {
		t.Fatalf("unexpected retained samples: %+v", component.samples)
	}

	clock.Set(120*time.Millisecond, 0, 2)
	if err := update.Run(world); err != nil {
		t.Fatal(err)
	}
	if component.SampleCount() != 0 {
		t.Fatalf("expired sample count = %d, want 0", component.SampleCount())
	}
}

func TestSpriteTrail2DHiddenEntityAgesWithoutSampling(t *testing.T) {
	world := ecs.NewWorld()
	trail := NewSpriteTrail2D()
	trail.Lifetime = 0.05
	trail.Interval = 0
	trail.MinDistance = 0
	bundle := ecs.NewBundle(4)
	bundle.Put(Sprite2D{Shape: RoundedRectShape2D(graphics.White, 2), Width: 20, Height: 10, Alpha: 1})
	bundle.Put(trail)
	bundle.Put(GlobalTransform{Matrix: mgl32.Ident4()})
	bundle.Put(Visibility{Visible: true})
	entity := world.Spawn(bundle)
	update := UpdateSpriteTrail2DSystem("")
	if err := update.Run(world); err != nil {
		t.Fatal(err)
	}

	visibility := world.GetMut[Visibility](entity)
	visibility.Visible = false
	clock := NewTime()
	clock.Set(100*time.Millisecond, 0, 1)
	world.InsertResource(clock)
	if err := update.Run(world); err != nil {
		t.Fatal(err)
	}
	component, _ := world.Get[SpriteTrail2D](entity)
	if component.SampleCount() != 0 {
		t.Fatalf("hidden entity retained or added samples: %d", component.SampleCount())
	}
}

func TestSpriteTrail2DRenderItemsPrecedeSourceAndSnapshotState(t *testing.T) {
	world := ecs.NewWorld()
	RegisterSpriteRenderer2D(world)
	RegisterSpriteTrailRenderer2D(world)
	trail := NewSpriteTrail2D()
	trail.AlphaStart = 0.5
	bundle := ecs.NewBundle(4)
	bundle.Put(Sprite2D{Shape: RoundedRectShape2D(graphics.White, 2), Width: 20, Height: 10, Alpha: 0.8})
	bundle.Put(trail)
	bundle.Put(GlobalTransform{Matrix: mgl32.Translate3D(7, 9, 0)})
	bundle.Put(Visibility{Visible: true})
	entity := world.Spawn(bundle)
	if err := UpdateSpriteTrail2DSystem("").Run(world); err != nil {
		t.Fatal(err)
	}

	items := collectRenderItems2D(graphics.Canvas2DContext(100, 80), world, false)
	if len(items) != 2 || items[0].entity != entity || items[1].entity != entity {
		t.Fatalf("render items = %+v, want trail then source", items)
	}
	if items[0].snapshot[Sprite2D]().Alpha != 0.4 || items[1].snapshot[Sprite2D]().Alpha != 0.8 {
		t.Fatalf("trail/source alpha = %v/%v, want 0.4/0.8", items[0].snapshot[Sprite2D]().Alpha, items[1].snapshot[Sprite2D]().Alpha)
	}
	component := world.GetMut[SpriteTrail2D](entity)
	component.Clear()
	if items[0].snapshot[Sprite2D]().Alpha != 0.4 || items[0].transform[12] != 7 {
		t.Fatalf("render snapshot changed after clearing simulation component: %+v", items[0])
	}
}

func TestSpriteTrail2DReserveAndClearKeepStorage(t *testing.T) {
	trail := NewSpriteTrail2D()
	trail.Reserve(3)
	if cap(trail.samples) != 3 {
		t.Fatalf("reserved capacity = %d, want 3", cap(trail.samples))
	}
	trail.appendSample(spriteTrailSample2D{})
	trail.Clear()
	if len(trail.samples) != 0 || cap(trail.samples) != 3 {
		t.Fatalf("cleared samples = len %d cap %d, want len 0 cap 3", len(trail.samples), cap(trail.samples))
	}
	if allocations := testing.AllocsPerRun(1_000, func() {
		trail.appendSample(spriteTrailSample2D{})
		trail.Clear()
	}); allocations != 0 {
		t.Fatalf("reserved trail update allocated %f objects", allocations)
	}
}

func TestSpriteTrailRingKeepsOrderAndReleasesReferences(t *testing.T) {
	trail := NewSpriteTrail2D()
	trail.MaxSamples = 4
	trail.Reserve(4)
	texture := &graphics.Texture{}
	for i := 1; i <= 12; i++ {
		var survivor *spriteTrailSample2D
		if trail.SampleCount() == 4 {
			survivor = trail.sample(1)
		}
		trail.appendSample(spriteTrailSample2D{sprite: Sprite2D{Texture: texture}, width: float32(i)})
		if survivor != nil && trail.sample(0) != survivor {
			t.Fatal("a full trail moved its surviving samples")
		}
		for index := range trail.SampleCount() {
			if trail.sample(index).width != float32(max(1, i-3)+index) {
				t.Fatal("wrapped trail changed chronological order")
			}
		}
	}
	trail.MaxSamples = 2
	trail.appendSample(spriteTrailSample2D{sprite: Sprite2D{Texture: texture}, width: 13})
	if trail.SampleCount() != 2 || trail.sample(0).width != 12 || trail.sample(1).width != 13 {
		t.Fatal("lowering the sample limit retained excess history")
	}
	trail.Reserve(10)
	if trail.SampleCount() != 2 || trail.sample(0).width != 12 || trail.sample(1).width != 13 {
		t.Fatal("growing a wrapped trail lost live snapshots")
	}
	trail.Lifetime = 1
	trail.sample(0).age, trail.sample(1).age = .9, .2
	survivor := trail.sample(1)
	trail.advance(.2)
	if trail.SampleCount() != 1 || trail.sample(0) != survivor || trail.sample(0).width != 13 {
		t.Fatal("expiration moved or lost the surviving snapshot")
	}
	trail.Clear()
	for _, sample := range trail.samples[:cap(trail.samples)] {
		if sample.sprite.Texture != nil {
			t.Fatal("clear or expiration retained a retired texture")
		}
	}
	// Clear a physically wrapped range as well as a grown, linear range.
	for range 11 {
		trail.appendSample(spriteTrailSample2D{sprite: Sprite2D{Texture: texture}})
	}
	trail.Clear()
	for _, sample := range trail.samples[:cap(trail.samples)] {
		if sample.sprite.Texture != nil {
			t.Fatal("clearing a wrapped trail left an old texture")
		}
	}
	if allocations := testing.AllocsPerRun(100, func() {
		trail.appendSample(spriteTrailSample2D{})
		trail.advance(.01)
	}); allocations != 0 {
		t.Fatalf("warm ring updates allocated %g times", allocations)
	}
}

func BenchmarkSpriteTrailBuffer(b *testing.B) {
	for _, count := range []int{24, 1024} {
		b.Run("ring/"+strconv.Itoa(count), func(b *testing.B) {
			trail := NewSpriteTrail2D()
			trail.MaxSamples = count
			trail.Reserve(count)
			for range count {
				trail.appendSample(spriteTrailSample2D{})
			}
			b.ReportAllocs()
			for b.Loop() {
				trail.appendSample(spriteTrailSample2D{})
			}
		})
		b.Run("shift/"+strconv.Itoa(count), func(b *testing.B) {
			values := make([]spriteTrailSample2D, count)
			b.ReportAllocs()
			for b.Loop() {
				values = slices.Delete(values, 0, 1)
				values = append(values, spriteTrailSample2D{})
			}
		})
	}
}
