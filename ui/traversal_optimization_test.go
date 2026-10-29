package ui

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"testing"
)

func BenchmarkTraversalTextFieldCaches(b *testing.B) {
	world := ecs.NewWorld()
	fields := world.NewSpawner1[TextField]()
	for i := range 1000 {
		fields.Spawn(TextField{Password: i%2 == 0, Composition: "x"})
	}
	ensureTextFieldCaches(world)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		ensureTextFieldCaches(world)
	}
}

func BenchmarkTraversalMenuMetrics(b *testing.B) {
	world := ecs.NewWorld()
	owners := world.NewSpawner2[Menu, ContextMenu]()
	owner := owners.Spawn(Menu{}, ContextMenu{})
	items := world.NewSpawner2[Item[Menu], Item[ContextMenu]]()
	labels := world.NewSpawner3[Relation[scene.Text2D], scene.Text2D, Part[ItemPart]]()
	for range 1000 {
		item := items.Spawn(Item[Menu]{Owner: owner}, Item[ContextMenu]{Owner: owner})
		labels.Spawn(Relation[scene.Text2D]{Owner: item}, scene.Text2D{}, Part[ItemPart]{})
	}
	collectSyncMetrics(world)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		collectSyncMetrics(world)
	}
}

func BenchmarkTraversalValueSurfaces(b *testing.B) {
	world := ecs.NewWorld()
	owners := world.NewSpawner1[TextField]()
	values := world.NewSpawner2[Relation[scene.Text2D], Part[ValuePart]]()
	for range 300 {
		owner := owners.Spawn(TextField{})
		values.Spawn(Relation[scene.Text2D]{Owner: owner}, Part[ValuePart]{})
		bundle := ecs.NewBundle(4)
		bundle.Put(Relation[SurfaceStyle]{Owner: owner})
		bundle.Put(scene.NewUITransform())
		bundle.Put(scene.Size2D{})
		bundle.Put(Part[CaretPart]{})
		world.Spawn(bundle)
	}
	syncSurfaceElements(world, Skin{}, syncMetrics{})
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		syncSurfaceElements(world, Skin{}, syncMetrics{})
	}
}

func TestTraversalValueTextIndexPreservesFirstMatchAndRefreshes(t *testing.T) {
	world := ecs.NewWorld()
	owners := world.NewSpawner1[TextField]()
	owner := owners.Spawn(TextField{})
	values := world.NewSpawner2[Relation[scene.Text2D], Part[ValuePart]]()
	first := values.Spawn(Relation[scene.Text2D]{Owner: owner}, Part[ValuePart]{})
	second := values.Spawn(Relation[scene.Text2D]{Owner: owner}, Part[ValuePart]{})
	// Both lack Text2D: malformed first matches must not silently be skipped.
	if got := partEntities[ValuePart, scene.Text2D](world)[owner]; got != first {
		t.Fatalf("first match=%d, want %d", got, first)
	}
	world.Remove[Part[ValuePart]](first)
	if got := partEntities[ValuePart, scene.Text2D](world)[owner]; got != second {
		t.Fatalf("after removal=%d, want %d", got, second)
	}
	if allocs := testing.AllocsPerRun(100, func() { partEntities[ValuePart, scene.Text2D](world) }); allocs != 0 {
		t.Fatalf("index allocated %v times", allocs)
	}
	world.Despawn(second)
	if _, exists := partEntities[ValuePart, scene.Text2D](world)[owner]; exists {
		t.Fatal("index retained despawned text")
	}
}

func TestTraversalTextFieldCacheTransitions(t *testing.T) {
	world := ecs.NewWorld()
	fields := world.NewSpawner1[TextField]()
	password := fields.Spawn(TextField{Password: true, Composition: "x"})
	composed := fields.Spawn(TextField{Composition: "x"})
	plain := fields.Spawn(TextField{})
	ensureTextFieldCaches(world)
	if !world.Has[textFieldMaskCache](password) || world.Has[textFieldDisplayCache](password) || !world.Has[textFieldDisplayCache](composed) || world.Has[textFieldMaskCache](plain) || world.Has[textFieldDisplayCache](plain) {
		t.Fatal("incorrect cache selection")
	}
	field := world.GetMut[TextField](password)
	field.Password = false
	ensureTextFieldCaches(world)
	if !world.Has[textFieldDisplayCache](password) {
		t.Fatal("composition cache missing after password mode changed")
	}
	world.Remove[textFieldMaskCache](password)
	field = world.GetMut[TextField](password)
	field.Password = true
	ensureTextFieldCaches(world)
	if !world.Has[textFieldMaskCache](password) {
		t.Fatal("removed mask cache not restored")
	}
	if allocs := testing.AllocsPerRun(100, func() { ensureTextFieldCaches(world) }); allocs != 0 {
		t.Fatalf("warm cache checks allocated %v times", allocs)
	}
}

func TestTraversalMenuMetricsSharedLabelsAndChanges(t *testing.T) {
	world := newUIWorld(t)
	theme, _ := world.GetResource[Theme]()
	owners := world.NewSpawner3[Menu, ContextMenu, Selector]()
	owner := owners.Spawn(Menu{}, ContextMenu{}, Selector{})
	items := world.NewSpawner3[Item[Menu], Item[ContextMenu], Item[Selector]]()
	item := items.Spawn(Item[Menu]{Owner: owner}, Item[ContextMenu]{Owner: owner}, Item[Selector]{Owner: owner})
	text := scene.Text2D{Value: "Shared menu item", Font: theme.Font, FontSize: 16}
	labels := world.NewSpawner3[Relation[scene.Text2D], scene.Text2D, Part[ItemPart]]()
	label := labels.Spawn(Relation[scene.Text2D]{Owner: item}, text, Part[ItemPart]{})
	expected, _, ok := scene.MeasureTextValue2D(&text, text.Value)
	if !ok || expected <= 0 {
		t.Fatal("test font did not measure")
	}
	changes := observeComponentChanges[scene.Text2D](t, world)
	metrics := collectSyncMetrics(world)
	want := itemMetrics{count: 1, width: expected}
	if metrics.menus[owner] != want || metrics.contextMenus[owner] != want || metrics.selectors[owner] != want {
		t.Fatalf("shared label metrics: %v %v %v, want %v", metrics.menus[owner], metrics.contextMenus[owner], metrics.selectors[owner], want)
	}
	if changes() != 0 {
		t.Fatal("measurement marked text changed")
	}
	if allocs := testing.AllocsPerRun(100, func() { collectSyncMetrics(world) }); allocs != 0 {
		t.Fatalf("metrics allocated %v times", allocs)
	}
	world.Despawn(label)
	metrics = collectSyncMetrics(world)
	if metrics.menus[owner].width != 0 || metrics.contextMenus[owner].width != 0 || metrics.selectors[owner].width != 0 {
		t.Fatal("removed label width retained")
	}
}
