package scene

import (
	"reflect"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/go-gl/mathgl/mgl32"
)

func TestInputCaptureLookupTracksFrameSnapshot(t *testing.T) {
	world := ecs.NewWorld()
	camera := spawnInputCameraForTest(t, world, 200, 100)
	bundle := ecs.NewBundle(5)
	bundle.Put(GlobalTransform{Matrix: mgl32.Ident4()})
	bundle.Put(UIGlobalTransform{Matrix: mgl32.Ident4()})
	bundle.Put(Visibility{Visible: true})
	bundle.Put(InputTarget2D{Width: 20, Height: 20})
	bundle.Put(InputBlocker2D{Width: 40, Height: 40})
	entity := world.Spawn(bundle)
	check := func(want bool, width float32, ui bool) {
		t.Helper()
		lookup := inputCaptureLookup2D{entity: entity, cameraEntity: camera}
		cameras, defaultUI := collectInputCameras2D(world, &lookup)
		targets := collectInputTargets2D(world, &lookup)
		got, x, y, ok := lookup.resolve(cameras, defaultUI, targets, entity, camera, 1, 1)
		original, ox, oy, ook := targetByEntity2D(cameras, defaultUI, targets, entity, camera, 1, 1)
		if !reflect.DeepEqual(got, original) || x != ox || y != oy || ok != ook {
			t.Fatalf("capture differs from ordered lookup: got %+v/%g/%g/%v, want %+v/%g/%g/%v", got, x, y, ok, original, ox, oy, ook)
		}
		if ok != want || (ok && (got.width != width || got.ui != ui)) {
			t.Fatalf("capture = width %g, ui %v, found %v; want %g/%v/%v", got.width, got.ui, ok, width, ui, want)
		}
	}
	// World and UI records, plus a blocker, all share the same entity.
	check(true, 20, false)
	world.Remove[GlobalTransform](entity)
	check(true, 20, true)
	target := world.GetMut[InputTarget2D](entity)
	target.Disabled = true
	check(true, 40, true)
	visibility := world.GetMut[Visibility](entity)
	visibility.Visible = false
	check(false, 0, false)
	visibility.Visible = true
	component := world.GetMut[Camera](camera)
	component.Active = false
	check(false, 0, false)
	component.Active = true
	world.Despawn(entity)
	check(false, 0, false)
}

func TestInputCaptureLookupResolvesRemappedIdentity(t *testing.T) {
	camera := inputPickCamera(1, 0, 0)
	first := inputPickQuad(2, 0, 0, true)
	second := inputPickQuad(3, 0, 0, true)
	lookup := inputCaptureLookup2D{
		entity: first.entity, cameraEntity: camera.entity, defaultUICamera: camera.entity,
		camera: camera, cameraFound: true,
	}
	lookup.consider(&first)
	lookup.consider(&second)
	cameras, targets := []inputCamera2D{camera}, []inputPickTarget2D{first, second}
	got, x, y, ok := lookup.resolve(cameras, camera.entity, targets, second.entity, camera.entity, 12, 24)
	if !ok || got.entity != second.entity || x != 12 || y != 24 {
		t.Fatalf("remapped capture = %+v, %g/%g/%v", got, x, y, ok)
	}
	if _, _, _, ok := lookup.resolve(cameras, camera.entity, targets, first.entity, camera.entity, 201, 24); ok {
		t.Fatal("capture accepted coordinates outside its viewport")
	}
	if _, _, _, ok := lookup.resolve(cameras, camera.entity, targets, second.entity, 99, 12, 24); ok {
		t.Fatal("capture accepted an absent camera")
	}
}

func BenchmarkInputCaptureLookup1000(b *testing.B) {
	camera := inputPickCamera(1, 0, 0)
	cameras := []inputCamera2D{camera}
	targets := make([]inputPickTarget2D, 1000)
	for i := range targets {
		targets[i] = inputPickQuad(ecs.Entity(i+2), 0, 0, true)
	}
	entity := targets[len(targets)-1].entity
	lookup := inputCaptureLookup2D{entity: entity, cameraEntity: camera.entity, defaultUICamera: camera.entity, camera: camera, cameraFound: true}
	for i := range targets {
		lookup.consider(&targets[i])
	}
	b.Run("Scan", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, x, _, _ := targetByEntity2D(cameras, camera.entity, targets, entity, camera.entity, 1, 1)
			benchmarkTotal += x
		}
	})
	b.Run("Collected", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, x, _, _ := lookup.resolve(cameras, camera.entity, targets, entity, camera.entity, 1, 1)
			benchmarkTotal += x
		}
	})
}
