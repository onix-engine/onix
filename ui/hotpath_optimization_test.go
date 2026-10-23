package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestStableControlSyncPreservesVersionsAndUpdatesBounds(t *testing.T) {
	world := ecs.NewWorld()
	spawner := ecs.NewSpawner3[Control, scene.InputTarget2D, scene.Size2D](world)
	entity := spawner.Spawn(Control{}, scene.InputTarget2D{Width: 20, Height: 10}, scene.Size2D{Width: 20, Height: 10})
	syncControls(world, Skin{})
	targetVersion, sizeVersion := ecs.ComponentVersion[scene.InputTarget2D](world), ecs.ComponentVersion[scene.Size2D](world)
	syncControls(world, Skin{})
	if targetVersion != ecs.ComponentVersion[scene.InputTarget2D](world) || sizeVersion != ecs.ComponentVersion[scene.Size2D](world) {
		t.Fatal("unchanged control marked components changed")
	}
	SetControlBounds(world, entity, 30, 15)
	syncControls(world, Skin{})
	size, _ := ecs.Get[scene.Size2D](world, entity)
	if size != (scene.Size2D{Width: 30, Height: 15}) {
		t.Fatalf("updated bounds=%v", size)
	}
}

func TestStableSurfaceAndScrollSyncPreservesVersions(t *testing.T) {
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(8)
	ecs.Put(&bundle, scene.Size2D{Width: 40, Height: 20})
	ecs.Put(&bundle, scene.UIBackground2D{})
	ecs.Put(&bundle, scene.UIImage2D{})
	ecs.Put(&bundle, scene.UIBorder2D{})
	ecs.Put(&bundle, scene.ClipRect2D{})
	ecs.Put(&bundle, scene.UIScroll{})
	ecs.Put(&bundle, ScrollArea{Width: 40, Height: 20, ContentWidth: 100, ContentHeight: 80})
	area := world.Spawn(bundle)
	contentSpawner := ecs.NewSpawner2[ScrollContent, scene.UITransform](world)
	content := contentSpawner.Spawn(ScrollContent{Area: area, X: 3, Y: 4}, scene.NewUITransform())
	step := func() {
		applySurface(world, area, 40, 20, SurfaceStyle{})
		prepareScrollAreas(world)
		finalizeScrollAreas(world)
	}
	step()
	version := world.MutationVersion()
	step()
	if version != world.MutationVersion() {
		t.Fatal("unchanged surface/scroll mutated world")
	}
	if allocs := testing.AllocsPerRun(100, step); allocs != 0 {
		t.Fatalf("steady sync allocations=%v", allocs)
	}
	if !SetScrollAreaOffset(world, area, mgl32.Vec2{10, 15}) {
		t.Fatal("scroll update failed")
	}
	finalizeScrollAreas(world)
	transform, _ := ecs.Get[scene.UITransform](world, content)
	if transform.Translation.X.Value != -7 || transform.Translation.Y.Value != -11 {
		t.Fatalf("scroll content=%v", transform.Translation)
	}
}

func TestStableWidgetSyncDoesNotInvalidateDerivedCaches(t *testing.T) {
	for _, kind := range []string{"button", "joined-button", "button-group", "input", "textarea", "number-input", "menu", "menu-open", "context-menu", "selector", "selector-open", "tabs", "progress", "scroll"} {
		t.Run(kind, func(t *testing.T) {
			world := newUIWorld(t)
			switch kind {
			case "button", "joined-button":
				SpawnButton(world, ButtonOptions{Width: 100, Height: 30, Label: "Button", Radius: 6, JoinRight: kind == "joined-button"})
			case "button-group":
				SpawnButtonGroup(world, ButtonGroupOptions[int]{Height: 30, Items: []ButtonGroupItem[int]{{Width: 50, Label: "A"}, {Width: 50, Label: "B"}}, Divider: &ButtonGroupDividerOptions{Width: 1}})
			case "input":
				SpawnInput(world, TextOptions{Width: 100, Height: 30, Value: "Input"})
			case "textarea":
				SpawnTextarea(world, TextOptions{Width: 100, Height: 80, Value: "Some text\nSome more text", Wrap: true})
			case "number-input":
				SpawnNumberInput(world, NumberInputOptions{Width: 100, Height: 30, Value: "5"})
			case "menu", "menu-open":
				entity := SpawnMenu(world, MenuOptions{Width: 100, Height: 30, Label: "Menu", Items: []string{"A", "B"}})
				menu := ecs.GetMut[Menu](world, entity)
				menu.Open = kind == "menu-open"
			case "context-menu":
				SpawnContextMenu(world, ContextMenuOptions{Width: 100, ItemHeight: 30, Open: true, Items: []string{"A", "B"}})
			case "selector", "selector-open":
				entity := SpawnSelector(world, SelectorOptions{Width: 100, Height: 30, Options: []string{"A", "B"}, ListHeight: 30})
				selector := ecs.GetMut[Selector](world, entity)
				selector.Open = kind == "selector-open"
			case "tabs":
				SpawnTab(world, TabOptions{Width: 100, Height: 80, HeaderHeight: 30, Items: []TabItemSpec{{Label: "A"}, {Label: "B"}}})
			case "progress":
				SpawnProgress(world, ProgressOptions{Width: 100, Height: 10, Max: 1, Value: 0.5, Slider: true})
			case "scroll":
				SpawnScrollArea(world, ScrollAreaOptions{Width: 100, Height: 80, ContentWidth: 200, ContentHeight: 300})
			}
			for range 2 {
				if err := Sync(world); err != nil {
					t.Fatal(err)
				}
				if err := SyncAfterLayout(world); err != nil {
					t.Fatal(err)
				}
			}
			version := world.MutationVersion()
			componentVersions := func() [8]uint64 {
				return [8]uint64{ecs.ComponentVersion[scene.UITransform](world), ecs.ComponentVersion[scene.Size2D](world), ecs.ComponentVersion[scene.Text2D](world), ecs.ComponentVersion[scene.Visibility](world), ecs.ComponentVersion[scene.UIStackIndex](world), ecs.ComponentVersion[scene.UIBackground2D](world), ecs.ComponentVersion[scene.UIImage2D](world), ecs.ComponentVersion[scene.UIBorder2D](world)}
			}
			before := componentVersions()
			if err := Sync(world); err != nil {
				t.Fatal(err)
			}
			if err := SyncAfterLayout(world); err != nil {
				t.Fatal(err)
			}
			if world.MutationVersion() != version {
				t.Fatalf("static widget sync mutated world: geometry versions %v → %v", before, componentVersions())
			}
		})
	}
}

func TestInputBlockerSyncPreservesSourcePrecedenceAndRemoval(t *testing.T) {
	world := newUIWorld(t)
	spawner := ecs.NewSpawner4[scene.InputBlocker2D, scene.Size2D, scene.Text2D, ScrollArea](world)
	text := scene.NewText2D("Bounds wrap to multiple lines\n中文")
	text.WrapMode, text.WrapWidth = scene.TextWrapCharacters, 40
	theme, _ := ecs.GetResource[Theme](world)
	text.Font = theme.Font
	entity := spawner.Spawn(scene.InputBlocker2D{}, scene.Size2D{Width: 40, Height: 20}, text, ScrollArea{Width: 100, Height: 80})
	syncInputBlockers(world)
	blocker, _ := ecs.Get[scene.InputBlocker2D](world, entity)
	if blocker.Width != 100 || blocker.Height != 80 {
		t.Fatalf("scroll source bounds=%v", blocker)
	}
	ecs.Remove[ScrollArea](world, entity)
	syncInputBlockers(world)
	currentText := ecs.GetRef[scene.Text2D](world, entity)
	width, height, _ := scene.MeasureText2D(currentText)
	blocker, _ = ecs.Get[scene.InputBlocker2D](world, entity)
	if blocker.Width != width || blocker.Height != height {
		t.Fatalf("text source bounds=%v", blocker)
	}
	ecs.Remove[scene.Text2D](world, entity)
	syncInputBlockers(world)
	blocker, _ = ecs.Get[scene.InputBlocker2D](world, entity)
	if blocker.Width != 40 || blocker.Height != 20 {
		t.Fatalf("size source bounds=%v", blocker)
	}
	version := world.MutationVersion()
	syncInputBlockers(world)
	if world.MutationVersion() != version {
		t.Fatal("stable blocker sync mutated world")
	}
}

func TestTabHierarchyCacheTracksDynamicChildren(t *testing.T) {
	world := ecs.NewWorld()
	parents := ecs.NewSpawner1[scene.Parent](world)
	root := parents.Spawn(scene.Parent{})
	for range 100 {
		parents.Spawn(scene.Parent{Entity: root})
	}
	syncTabs(world, Skin{})
	cache := ecs.FrameValue[tabHierarchyScratch, tabHierarchy](world)
	if cache.initialized {
		t.Fatal("absent Tab collected parent hierarchy")
	}
	tabs := ecs.NewSpawner2[Tab, scene.Size2D](world)
	tab := tabs.Spawn(Tab{Width: 100, Height: 80, HeaderHeight: 20}, scene.Size2D{})
	other := tabs.Spawn(Tab{Width: 200, Height: 80, HeaderHeight: 20}, scene.Size2D{})
	items := ecs.NewSpawner4[TabItem, scene.Parent, scene.UITransform, scene.Size2D](world)
	item := items.Spawn(TabItem{Tabs: tab, Width: 30}, scene.Parent{Entity: tab}, scene.NewUITransform(), scene.Size2D{})
	syncTabs(world, Skin{})
	if len(tabChildren(cache.children, cache.ranges, tab)) != 1 {
		t.Fatal("new child not indexed")
	}
	versions := cache.versions
	syncTabs(world, Skin{})
	if cache.versions != versions {
		t.Fatal("stable parent input changed cache")
	}
	parent := ecs.GetMut[scene.Parent](world, item)
	parent.Entity = other
	syncTabs(world, Skin{})
	if len(tabChildren(cache.children, cache.ranges, tab)) != 0 || len(tabChildren(cache.children, cache.ranges, other)) != 1 {
		t.Fatal("reparent not reflected")
	}
	ecs.Remove[scene.Parent](world, item)
	syncTabs(world, Skin{})
	if len(tabChildren(cache.children, cache.ranges, other)) != 0 {
		t.Fatal("removed parent retained")
	}
	world.Despawn(item)
	items.Spawn(TabItem{Tabs: tab}, scene.Parent{Entity: tab}, scene.NewUITransform(), scene.Size2D{})
	syncTabs(world, Skin{})
	if len(tabChildren(cache.children, cache.ranges, tab)) != 1 {
		t.Fatal("entity reuse not indexed")
	}
}

func TestCombinedTextWidthsPreserveLabelsAndReadOnlyMeasurement(t *testing.T) {
	world := newUIWorld(t)
	theme, _ := ecs.GetResource[Theme](world)
	owner := SpawnButton(world, ButtonOptions{Width: 100, Height: 30, Label: "Button", Icon: TextIcon("+")})
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	version := world.MutationVersion()
	icons, labels := collectTextWidths(world, theme)
	if icons[owner] <= 0 || labels[owner] <= 0 || world.MutationVersion() != version {
		t.Fatal("combined measurement failed or marked text changed")
	}
	if allocs := testing.AllocsPerRun(100, func() { collectTextWidths(world, theme) }); allocs != 0 {
		t.Fatalf("combined measurement allocations=%v", allocs)
	}
}

func BenchmarkHotpathTabsAbsent(b *testing.B) {
	world := ecs.NewWorld()
	parents := ecs.NewSpawner1[scene.Parent](world)
	root := parents.Spawn(scene.Parent{})
	for range 10000 {
		parents.Spawn(scene.Parent{Entity: root})
	}
	syncTabs(world, Skin{})
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		syncTabs(world, Skin{})
	}
}

func BenchmarkHotpathStaticControls(b *testing.B) {
	world := ecs.NewWorld()
	spawner := ecs.NewSpawner3[Control, scene.InputTarget2D, scene.Size2D](world)
	for range 10000 {
		spawner.Spawn(Control{}, scene.InputTarget2D{Width: 20, Height: 10}, scene.Size2D{Width: 20, Height: 10})
	}
	syncControls(world, Skin{})
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		syncControls(world, Skin{})
	}
}
