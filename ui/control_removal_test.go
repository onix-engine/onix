package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func TestUnbindControlPreservesAuthoredDataAndMigratesEachNodeOnce(t *testing.T) {
	world := ecs.NewWorld()
	roots := world.NewSpawner7[scene.Name, scene.UITransform, Control, scene.InputTarget2D, scene.InputState2D, Button, Variants[ControlVariant]]()
	parts := world.NewSpawner7[scene.Name, scene.UITransform, scene.Parent, Relation[Control], Relation[SurfaceStyle], Part[BodyPart], scene.ClipRect2D]()
	root := roots.Spawn(scene.Name{Value: "root"}, scene.UITransformFromXY(10, 20), Control{}, scene.InputTarget2D{}, scene.InputState2D{}, Button{}, Variants[ControlVariant]{})
	first := parts.Spawn(scene.Name{Value: "first"}, scene.UITransformFromXY(1, 2), scene.Parent{Entity: root}, Relation[Control]{Owner: root}, Relation[SurfaceStyle]{Owner: root}, Part[BodyPart]{}, scene.ClipRect2D{Width: 30})
	second := parts.Spawn(scene.Name{Value: "second"}, scene.UITransformFromXY(3, 4), scene.Parent{Entity: root}, Relation[Control]{Owner: root}, Relation[SurfaceStyle]{Owner: root}, Part[BodyPart]{}, scene.ClipRect2D{Width: 40})
	world.GetMut[Variants[ControlVariant]](root).Items = []Variant[ControlVariant]{{Entity: first}, {Entity: second}}
	before := world.ComponentStructureVersion[scene.Name]()
	UnbindControl(world, root)
	// Each retained column is appended once and removed once per entity.
	if delta := world.ComponentStructureVersion[scene.Name]() - before; delta != 6 {
		t.Fatalf("three entities migrated their retained Name column %d times, want 6", delta)
	}
	if world.Has[Control](root) || world.Has[Button](root) || world.Has[scene.InputTarget2D](root) || world.Has[Variants[ControlVariant]](root) {
		t.Fatal("root behavior was retained")
	}
	if *world.GetRef[scene.UITransform](root) != scene.UITransformFromXY(10, 20) {
		t.Fatal("authored root transform was overwritten")
	}
	for _, entity := range []ecs.Entity{first, second} {
		if !world.IsAlive(entity) || world.Has[Part[BodyPart]](entity) || world.Has[Relation[Control]](entity) || world.Has[Relation[SurfaceStyle]](entity) {
			t.Fatal("composition behavior was retained or node was destroyed")
		}
		if world.GetRef[scene.ClipRect2D](entity) == nil || world.GetRef[scene.Parent](entity).Entity != root {
			t.Fatal("authored clip or hierarchy was removed")
		}
	}
	before = world.ComponentStructureVersion[scene.Name]()
	UnbindControl(world, root)
	if world.ComponentStructureVersion[scene.Name]() != before {
		t.Fatal("repeated unbind migrated a neutral node")
	}
}

func TestUnbindCompositionPartRemovesConditionalClipsAndOwnerLinks(t *testing.T) {
	world := ecs.NewWorld()
	rootSpawner := world.NewSpawner2[Progress, Variants[ControlVariant]]()
	root := rootSpawner.Spawn(Progress{}, Variants[ControlVariant]{})
	bundle := ecs.NewBundle(9)
	bundle.Put(scene.Name{Value: "part"})
	bundle.Put(Relation[Control]{Owner: root})
	bundle.Put(Relation[Progress]{Owner: root})
	bundle.Put(Part[FillPart]{})
	bundle.Put(Part[ThumbPart]{})
	bundle.Put(Part[SelectorListPart]{})
	bundle.Put(Part[ScrollThumbVerticalPart]{})
	bundle.Put(scene.ClipRect2D{})
	bundle.Put(scene.ClipExempt2D{})
	entity := world.Spawn(bundle)
	progress := world.GetMut[Progress](root)
	progress.Fill, progress.Thumb = entity, entity
	world.GetMut[Variants[ControlVariant]](root).Items = []Variant[ControlVariant]{{Entity: entity}}
	before := world.ComponentStructureVersion[scene.Name]()
	UnbindControlCompositionPart(world, entity)
	if delta := world.ComponentStructureVersion[scene.Name]() - before; delta != 2 {
		t.Fatalf("part migration count = %d, want 2", delta)
	}
	progress = world.GetRef[Progress](root)
	if progress.Fill != 0 || progress.Thumb != 0 || world.Has[Variants[ControlVariant]](root) ||
		world.Has[scene.ClipRect2D](entity) || world.Has[scene.ClipExempt2D](entity) ||
		world.Has[Relation[Progress]](entity) || world.Has[Part[FillPart]](entity) {
		t.Fatal("part teardown left runtime owner links, markers or clips")
	}
}

func TestUnbindInputAlsoRemovesPasswordCache(t *testing.T) {
	world := ecs.NewWorld()
	entity := SpawnInput(world, TextOptions{Width: 120, Height: 30, Password: true, Value: "secret"})
	if !world.Has[textFieldMaskCache](entity) {
		t.Fatal("input has no password cache")
	}
	UnbindControl(world, entity)
	if world.Has[TextField](entity) || world.Has[Input](entity) || world.Has[textFieldMaskCache](entity) {
		t.Fatal("input teardown left text runtime state")
	}
}

func TestUnbindControlWithPartsDoesNotAllocateAfterWarmup(t *testing.T) {
	world := ecs.NewWorld()
	roots := world.NewSpawner3[scene.Name, Control, Variants[ControlVariant]]()
	parts := world.NewSpawner4[scene.Name, Relation[Control], Part[LabelPart], Relation[scene.Text2D]]()
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
