package ui

import (
	"reflect"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/ui/html"
)

func observeComponentChanges[T any](t *testing.T, world *ecs.World) func() int {
	t.Helper()
	count := 0
	system := ecs.SystemConfig{
		Name: "observe_" + reflect.TypeFor[T]().String(),
		System: func(context *ecs.SystemContext) error {
			count = 0
			for range ecs.Query[T](world, ecs.Changed[T](context)).All {
				count++
			}
			return nil
		},
	}
	run := func() int {
		if err := system.Run(world); err != nil {
			t.Fatal(err)
		}
		return count
	}
	run()
	return run
}

func TestButtonLabelMetricsDoNotMarkComponentsChanged(t *testing.T) {
	world := newUIWorld(t)
	owner := SpawnButton(world, ButtonOptions{Width: 120, Height: 32, Label: "Button"})
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	textChanges := observeComponentChanges[scene.Text2D](t, world)
	relationChanges := observeComponentChanges[Relation[scene.Text2D]](t, world)
	partChanges := observeComponentChanges[Part[LabelPart]](t, world)
	widths := collectButtonLabelWidths(world)
	if widths[owner] <= 0 {
		t.Fatalf("label width=%v, want a measured width", widths[owner])
	}
	if textChanges() != 0 || relationChanges() != 0 || partChanges() != 0 {
		t.Fatal("measuring labels marked components changed")
	}
	if allocations := testing.AllocsPerRun(100, func() { collectButtonLabelWidths(world) }); allocations != 0 {
		t.Fatalf("label metrics allocations=%v, want 0", allocations)
	}
	var label ecs.Entity
	for entity, access := range ecs.Query3[Relation[scene.Text2D], scene.Text2D, Part[LabelPart]](world).All {
		relation := access.A.Ref()

		if relation.Owner == owner {
			label = entity
			break
		}
	}
	world.Despawn(label)
	if _, exists := collectButtonLabelWidths(world)[owner]; exists {
		t.Fatal("label metrics retained a removed label")
	}
}

func TestSelectorLabelMetricsRefreshSelectionAndRemoval(t *testing.T) {
	world := ecs.NewWorld()
	selectors := ecs.NewSpawner1[Selector](world)
	owner := selectors.Spawn(Selector{Label: "fallback", Selected: 0})
	items := ecs.NewSpawner1[SelectorItem](world)
	items.Spawn(SelectorItem{Owner: owner, Index: 0, Label: "first"})
	selected := items.Spawn(SelectorItem{Owner: owner, Index: 1, Label: ""})
	items.Spawn(SelectorItem{Owner: owner, Index: 1, Label: "duplicate"})
	selector := ecs.GetRef[Selector](world, owner)
	if label := selectorCurrentLabel(collectSelectorLabels(world), owner, selector); label != "first" {
		t.Fatalf("label=%q, want first", label)
	}
	mutable := ecs.GetMut[Selector](world, owner)
	mutable.Selected = 1
	if label := selectorCurrentLabel(collectSelectorLabels(world), owner, selector); label != "" {
		t.Fatalf("empty first match lost to duplicate or fallback: %q", label)
	}
	if allocations := testing.AllocsPerRun(100, func() { collectSelectorLabels(world) }); allocations != 0 {
		t.Fatalf("selector metrics allocations=%v, want 0", allocations)
	}
	world.Despawn(selected)
	selector = ecs.GetRef[Selector](world, owner)
	if label := selectorCurrentLabel(collectSelectorLabels(world), owner, selector); label != "duplicate" {
		t.Fatalf("removed selected item was retained: %q", label)
	}
	mutable = ecs.GetMut[Selector](world, owner)
	mutable.Selected = 99
	if label := selectorCurrentLabel(collectSelectorLabels(world), owner, selector); label != "fallback" {
		t.Fatalf("missing selection label=%q, want fallback", label)
	}
}

func TestSkippedTextLayoutsDoNotMarkTransformsChanged(t *testing.T) {
	for _, reason := range []string{"html_text", "authored", "editable", "missing_control"} {
		t.Run(reason, func(t *testing.T) {
			world := ecs.NewWorld()
			owner := world.Spawn(ecs.NewBundle(0))
			visuals := ecs.NewSpawner2[Relation[scene.Text2D], scene.UITransform](world)
			visual := visuals.Spawn(Relation[scene.Text2D]{Owner: owner}, scene.NewUITransform())
			switch reason {
			case "html_text":
				_ = ecs.Insert(world, visual, html.ControlText{Control: owner})
			case "authored":
				_ = ecs.Insert(world, visual, Relation[Control]{Owner: owner})
			case "editable":
				_ = ecs.Insert(world, owner, html.Editable{})
			}
			changes := observeComponentChanges[scene.UITransform](t, world)
			layoutTextVisuals(world, nil, nil)
			if changes() != 0 {
				t.Fatal("skipped visual was marked changed")
			}
		})
	}
}

func TestMenuIndicatorsOnlyMarkChangedValues(t *testing.T) {
	world := ecs.NewWorld()
	menus := ecs.NewSpawner2[Menu, IconContent](world)
	entity := menus.Spawn(Menu{}, IconContent{})
	skin := skinFor(world, DefaultTheme())
	syncMenuIndicators(world, skin)
	menuChanges := observeComponentChanges[Menu](t, world)
	iconChanges := observeComponentChanges[IconContent](t, world)
	syncMenuIndicators(world, skin)
	if menuChanges() != 0 || iconChanges() != 0 {
		t.Fatal("unchanged menu indicators were marked changed")
	}
	icon := ecs.GetMut[IconContent](world, entity)
	icon.Value = TextIcon("different")
	iconChanges()
	syncMenuIndicators(world, skin)
	if menuChanges() != 0 || iconChanges() != 1 {
		t.Fatal("indicator update did not mark only IconContent changed")
	}
}

func BenchmarkSelectorLabelMetrics(b *testing.B) {
	for _, count := range []int{100, 1000} {
		name := "100"
		if count == 1000 {
			name = "1000"
		}
		b.Run(name, func(b *testing.B) {
			world := ecs.NewWorld()
			selectors := ecs.NewSpawner1[Selector](world)
			items := ecs.NewSpawner1[SelectorItem](world)
			for range count {
				owner := selectors.Spawn(Selector{})
				items.Spawn(SelectorItem{Owner: owner, Label: "selected"})
			}
			collectSelectorLabels(world)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				collectSelectorLabels(world)
			}
		})
	}
}
