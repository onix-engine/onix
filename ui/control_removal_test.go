package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func TestUnbindControlPreservesAuthoredDataAndMigratesEachNodeOnce(t *testing.T) {
	world := ecs.NewWorld()
	roots := ecs.NewSpawner7[scene.Name, scene.UITransform, Control, scene.InputTarget2D, scene.InputState2D, Button, Variants[ControlVariant]](world)
	parts := ecs.NewSpawner7[scene.Name, scene.UITransform, scene.Parent, Relation[Control], Relation[SurfaceStyle], Part[BodyPart], scene.ClipRect2D](world)
	root := roots.Spawn(scene.Name{Value: "root"}, scene.UITransformFromXY(10, 20), Control{}, scene.InputTarget2D{}, scene.InputState2D{}, Button{}, Variants[ControlVariant]{})
	first := parts.Spawn(scene.Name{Value: "first"}, scene.UITransformFromXY(1, 2), scene.Parent{Entity: root}, Relation[Control]{Owner: root}, Relation[SurfaceStyle]{Owner: root}, Part[BodyPart]{}, scene.ClipRect2D{Width: 30})
	second := parts.Spawn(scene.Name{Value: "second"}, scene.UITransformFromXY(3, 4), scene.Parent{Entity: root}, Relation[Control]{Owner: root}, Relation[SurfaceStyle]{Owner: root}, Part[BodyPart]{}, scene.ClipRect2D{Width: 40})
	ecs.GetMut[Variants[ControlVariant]](world, root).Items = []Variant[ControlVariant]{{Entity: first}, {Entity: second}}
	before := ecs.ComponentStructureVersion[scene.Name](world)
	UnbindControl(world, root)
	// Each retained column is appended once and removed once per entity.
	if delta := ecs.ComponentStructureVersion[scene.Name](world) - before; delta != 6 {
		t.Fatalf("three entities migrated their retained Name column %d times, want 6", delta)
	}
	if ecs.Has[Control](world, root) || ecs.Has[Button](world, root) || ecs.Has[scene.InputTarget2D](world, root) || ecs.Has[Variants[ControlVariant]](world, root) {
		t.Fatal("root behavior was retained")
	}
	if *ecs.GetRef[scene.UITransform](world, root) != scene.UITransformFromXY(10, 20) {
		t.Fatal("authored root transform was overwritten")
	}
	for _, entity := range []ecs.Entity{first, second} {
		if !world.IsAlive(entity) || ecs.Has[Part[BodyPart]](world, entity) || ecs.Has[Relation[Control]](world, entity) || ecs.Has[Relation[SurfaceStyle]](world, entity) {
			t.Fatal("composition behavior was retained or node was destroyed")
		}
		if ecs.GetRef[scene.ClipRect2D](world, entity) == nil || ecs.GetRef[scene.Parent](world, entity).Entity != root {
			t.Fatal("authored clip or hierarchy was removed")
		}
	}
	before = ecs.ComponentStructureVersion[scene.Name](world)
	UnbindControl(world, root)
	if ecs.ComponentStructureVersion[scene.Name](world) != before {
		t.Fatal("repeated unbind migrated a neutral node")
	}
}

func TestUnbindCompositionPartRemovesConditionalClipsAndOwnerLinks(t *testing.T) {
	world := ecs.NewWorld()
	rootSpawner := ecs.NewSpawner2[Progress, Variants[ControlVariant]](world)
	root := rootSpawner.Spawn(Progress{}, Variants[ControlVariant]{})
	bundle := ecs.NewBundle(9)
	ecs.Put(&bundle, scene.Name{Value: "part"})
	ecs.Put(&bundle, Relation[Control]{Owner: root})
	ecs.Put(&bundle, Relation[Progress]{Owner: root})
	ecs.Put(&bundle, Part[FillPart]{})
	ecs.Put(&bundle, Part[ThumbPart]{})
	ecs.Put(&bundle, Part[SelectorListPart]{})
	ecs.Put(&bundle, Part[ScrollThumbVerticalPart]{})
	ecs.Put(&bundle, scene.ClipRect2D{})
	ecs.Put(&bundle, scene.ClipExempt2D{})
	entity := world.Spawn(bundle)
	progress := ecs.GetMut[Progress](world, root)
	progress.Fill, progress.Thumb = entity, entity
	ecs.GetMut[Variants[ControlVariant]](world, root).Items = []Variant[ControlVariant]{{Entity: entity}}
	before := ecs.ComponentStructureVersion[scene.Name](world)
	UnbindControlCompositionPart(world, entity)
	if delta := ecs.ComponentStructureVersion[scene.Name](world) - before; delta != 2 {
		t.Fatalf("part migration count = %d, want 2", delta)
	}
	progress = ecs.GetRef[Progress](world, root)
	if progress.Fill != 0 || progress.Thumb != 0 || ecs.Has[Variants[ControlVariant]](world, root) ||
		ecs.Has[scene.ClipRect2D](world, entity) || ecs.Has[scene.ClipExempt2D](world, entity) ||
		ecs.Has[Relation[Progress]](world, entity) || ecs.Has[Part[FillPart]](world, entity) {
		t.Fatal("part teardown left runtime owner links, markers or clips")
	}
}

func TestUnbindInputAlsoRemovesPasswordCache(t *testing.T) {
	world := ecs.NewWorld()
	entity := SpawnInput(world, TextOptions{Width: 120, Height: 30, Password: true, Value: "secret"})
	if !ecs.Has[textFieldMaskCache](world, entity) {
		t.Fatal("input has no password cache")
	}
	UnbindControl(world, entity)
	if ecs.Has[TextField](world, entity) || ecs.Has[Input](world, entity) || ecs.Has[textFieldMaskCache](world, entity) {
		t.Fatal("input teardown left text runtime state")
	}
}

func TestUnbindControlWithPartsDoesNotAllocateAfterWarmup(t *testing.T) {
	world := ecs.NewWorld()
	roots := ecs.NewSpawner3[scene.Name, Control, Variants[ControlVariant]](world)
	parts := ecs.NewSpawner4[scene.Name, Relation[Control], Part[LabelPart], Relation[scene.Text2D]](world)
	variants := make([]Variant[ControlVariant], 2)
	run := func() {
		root := roots.Spawn(scene.Name{}, Control{}, Variants[ControlVariant]{Items: variants})
		first := parts.Spawn(scene.Name{}, Relation[Control]{Owner: root}, Part[LabelPart]{}, Relation[scene.Text2D]{Owner: root})
		second := parts.Spawn(scene.Name{}, Relation[Control]{Owner: root}, Part[LabelPart]{}, Relation[scene.Text2D]{Owner: root})
		variants[0].Entity, variants[1].Entity = first, second
		UnbindControl(world, root)
		world.Despawn(root)
		world.Despawn(first)
		world.Despawn(second)
	}
	if allocs := testing.AllocsPerRun(100, run); allocs != 0 {
		t.Fatalf("control teardown allocated %v times", allocs)
	}
}
