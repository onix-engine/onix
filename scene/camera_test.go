package scene

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/go-gl/mathgl/mgl32"
)

func TestSortedCameraEntitiesPanicsForNilWorld(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected nil world to panic")
		}
	}()
	SortedCameraEntities(nil)
}

func TestCameraFallbackTracksAuthoredCameraActivity(t *testing.T) {
	world := ecs.NewWorld()
	fallback := SpawnCamera2D(world)
	world.GetMut[Camera](fallback).Fallback = true
	parentBundle := ecs.NewBundle(1)
	parentBundle.Put(Activity{Active: true})
	parent := world.Spawn(parentBundle)
	authored := SpawnCamera2D(world)
	_ = world.Insert(authored, Parent{Entity: parent})
	for _, active := range []bool{true, false, true} {
		*world.GetMut[Activity](parent) = Activity{Active: active}
		want := fallback
		if active {
			want = authored
		}
		if got, ok := PrimaryCamera2D(world); !ok || got != want {
			t.Fatal("incorrect primary camera", active, got)
		}
		if cameras := SortedCameraEntities(world); len(cameras) != 1 || cameras[0] != want {
			t.Fatal("fallback camera participated alongside authored camera", cameras)
		}
	}
	world.Despawn(authored)
	if got, ok := PrimaryCamera2D(world); !ok || got != fallback {
		t.Fatal("fallback did not recover after unload")
	}
}

func TestRenderLayersGrowBeyondSingleMachineWord(t *testing.T) {
	layers := RenderLayer(0).With(65).With(130)
	for _, layer := range []uint{0, 65, 130} {
		if !layers.Contains(layer) {
			t.Fatalf("render layers do not contain %d", layer)
		}
	}
	if layers.Intersects(RenderLayer(64)) || !layers.Intersects(RenderLayer(130)) {
		t.Fatalf("unexpected layer intersection")
	}
	layers = layers.Without(65)
	if layers.Contains(65) || !layers.Contains(130) {
		t.Fatalf("unexpected layers after removal")
	}
}

func TestRenderLayersZeroValueUsesDefaultLayer(t *testing.T) {
	var layers RenderLayers
	if !layers.Contains(0) || !layers.Intersects(RenderLayer(0)) {
		t.Fatal("zero-value RenderLayers must contain the default layer")
	}
	if RenderLayersNone().Intersects(layers) {
		t.Fatal("explicit empty RenderLayers intersected the default layer")
	}
}

func TestSortedCameraEntitiesUsesOrderThenEntity(t *testing.T) {
	world := ecs.NewWorld()
	spawn := func(order int, active bool) ecs.Entity {
		bundle := ecs.NewBundle(1)
		camera := NewCamera()
		camera.Order = order
		camera.Active = active
		bundle.Put(camera)
		return world.Spawn(bundle)
	}
	first := spawn(5, true)
	second := spawn(-1, true)
	third := spawn(5, true)
	spawn(-10, false)
	got := SortedCameraEntities(world)
	want := []ecs.Entity{second, first, third}
	if len(got) != len(want) {
		t.Fatalf("sorted cameras = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("sorted cameras = %v, want %v", got, want)
		}
	}
}

func TestCameraFrameDoesNotAllocateAfterWarmup(t *testing.T) {
	ctx := graphics.Canvas2DContext(320, 180)
	restore := graphics.SetCurrentContextForTesting(ctx)
	t.Cleanup(restore)

	world := ecs.NewWorld()

	graphics.BindFrame(world, graphics.CurrentContext())
	RegisterCameraRenderer2D(world)
	bundle := ecs.NewBundle(4)
	bundle.Put(NewCamera())
	bundle.Put(Camera2D{})
	bundle.Put(OrthographicProjection2D())
	bundle.Put(GlobalTransform{Matrix: mgl32.Ident4()})
	world.Spawn(bundle)
	context := ecs.SystemContext{World: world}
	if err := ExtractCameras(&context); err != nil {
		t.Fatal(err)
	}
	if err := renderCameras(&context); err != nil {
		t.Fatal(err)
	}

	if allocations := testing.AllocsPerRun(100, func() {
		if err := ExtractCameras(&context); err != nil {
			t.Fatal(err)
		}
		if err := renderCameras(&context); err != nil {
			t.Fatal(err)
		}
	}); allocations != 0 {
		t.Fatalf("camera frame allocated %.0f objects, want 0", allocations)
	}
}
