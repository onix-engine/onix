package ui

import (
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func TestFindPartRefreshesForOwnerChangesMigrationAndSlotReuse(t *testing.T) {
	world := ecs.NewWorld()
	spawner := ecs.NewSpawner2[Relation[scene.Text2D], Part[ValuePart]](world)
	first := spawner.Spawn(Relation[scene.Text2D]{Owner: 1}, Part[ValuePart]{})
	second := spawner.Spawn(Relation[scene.Text2D]{Owner: 1}, Part[ValuePart]{})
	if got, _ := FindPart[ValuePart, scene.Text2D](world, 1); got != first {
		t.Fatal("initial query order was not preserved")
	}
	if err := ecs.Insert(world, first, scene.Name{Value: "migrated"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := FindPart[ValuePart, scene.Text2D](world, 1); got != second {
		t.Fatal("column migration did not refresh first match")
	}
	relation := ecs.GetMut[Relation[scene.Text2D]](world, second)
	relation.Owner = 2
	if got, _ := FindPart[ValuePart, scene.Text2D](world, 2); got != second {
		t.Fatal("owner mutation did not refresh index")
	}
	world.Despawn(second)
	replacement := spawner.Spawn(Relation[scene.Text2D]{Owner: 3}, Part[ValuePart]{})
	if _, ok := FindPart[ValuePart, scene.Text2D](world, 2); ok {
		t.Fatal("despawned entry was retained")
	}
	if got, _ := FindPart[ValuePart, scene.Text2D](world, 3); got != replacement {
		t.Fatal("reused entity slot did not refresh index")
	}
	if allocs := testing.AllocsPerRun(100, func() { FindPart[ValuePart, scene.Text2D](world, 3) }); allocs != 0 {
		t.Fatalf("part lookup allocated %v times", allocs)
	}
}

func TestTextareaScrollMeasurementsRefreshBetweenSyncPhases(t *testing.T) {
	world := newUIWorld(t)
	owner := SpawnTextarea(world, TextOptions{Width: 120, Height: 60, Value: "short"})
	track, ok := FindPart[ScrollTrackHorizontalPart, SurfaceStyle](world, owner)
	if !ok {
		t.Fatal("horizontal track missing")
	}
	textEntity, _ := FindPart[ValuePart, scene.Text2D](world, owner)
	for _, value := range []string{"short", strings.Repeat("long text ", 40), "short"} {
		text := ecs.GetMut[scene.Text2D](world, textEntity)
		text.Value = value
		syncSurfaceElements(world, Skin{}, syncMetrics{})
		size, _ := ecs.Get[scene.Size2D](world, track)
		if (size.Width > 0) != (len(value) > 5) {
			t.Fatalf("stale scrollbar measurement for %q: %+v", value, size)
		}
	}
}

func TestTextControlSpawnPreservesPasswordStyleAndBodyDefaults(t *testing.T) {
	world := ecs.NewWorld()
	style := TextFieldStyle{}
	options := TextOptions{Width: 120, Height: 60, Value: "secret", Password: true, Radius: 8, Style: &style, Disabled: true}
	input, textarea := SpawnInput(world, options), SpawnTextarea(world, options)
	for _, entity := range []ecs.Entity{input, textarea} {
		field, _ := ecs.Get[TextField](world, entity)
		if field.Password != (entity == input) || !ecs.Has[StyleOverride[TextFieldStyle]](world, entity) {
			t.Fatal("text behavior or style override changed")
		}
		target, _ := ecs.Get[scene.InputTarget2D](world, entity)
		body, _ := FindPart[BodyPart, SurfaceStyle](world, entity)
		background, _ := ecs.Get[scene.UIBackground2D](world, body)
		image, _ := ecs.Get[scene.UIImage2D](world, body)
		if !target.Disabled || background.Shape.Radius != 8 || image != scene.NewUIImage2D(nil) {
			t.Fatal("spawned body lost radius, image defaults or disabled state")
		}
	}
	if !ecs.Has[textFieldMaskCache](world, input) || ecs.Has[textFieldMaskCache](world, textarea) {
		t.Fatal("incorrect password cache components")
	}
}

func BenchmarkRefactorSpawnButtons(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		world := ecs.NewWorld()
		for range 50 {
			SpawnButton(world, ButtonOptions{Width: 120, Height: 30, Label: "Play"})
		}
	}
}

func BenchmarkRefactorTextareaScrollParts(b *testing.B) {
	world := ecs.NewWorld()
	for range 100 {
		SpawnTextarea(world, TextOptions{Width: 120, Height: 60, Value: "text"})
	}
	syncSurfaceElements(world, Skin{}, syncMetrics{})
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		syncSurfaceElements(world, Skin{}, syncMetrics{})
	}
}
