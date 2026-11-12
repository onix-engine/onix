package ui

import (
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/internal/reuse"
	"github.com/onix-engine/onix/scene"
)

func TestUILayoutMapRetentionPreservesLiveState(t *testing.T) {
	world := ecs.NewWorld()
	roots := world.NewSpawner2[scene.UITransform, scene.Size2D]()
	root := roots.Spawn(scene.NewUITransform(), scene.Size2D{Width: 320, Height: 240})
	widgets := world.NewSpawner5[Layout, Widget, scene.UITransform, scene.Size2D, scene.Parent]()
	entities := make([]ecs.Entity, 256)
	for i := range entities {
		entities[i] = widgets.Spawn(Layout{Type: LayoutHorizontal}, Widget{Enabled: true, AlignLeft: true, AlignMode: WidgetAlignOnce}, scene.NewUITransform(), scene.Size2D{Width: 20, Height: 20}, scene.Parent{Entity: root})
	}
	update := func() {
		if err := UpdateLayout(world); err != nil {
			t.Fatal(err)
		}
	}
	update()
	scratch := world.FrameValue[uiLayoutScratchTag, uiLayoutScratch]()
	runtime := world.FrameValue[uiWidgetRuntimeStateTag, uiWidgetRuntimeStates]()
	oldLayouts, oldWidgets := scratch.runtime, runtime.values
	if len(oldLayouts) != len(entities) || len(oldWidgets) != len(entities) {
		t.Fatal("layout and widget caches were not populated")
	}
	live := entities[0]
	layoutState, widgetState := oldLayouts[live], oldWidgets[live]
	world.Maintain()
	for _, entity := range entities[1:] {
		world.Despawn(entity)
	}
	update()
	for range reuse.RetentionFrames + 1 {
		world.Maintain()
	}
	oldLayouts[0], oldWidgets[0] = nil, uiWidgetRuntimeState{}
	if _, found := scratch.runtime[0]; found {
		t.Fatal("layout cache retained its original peak storage")
	}
	if _, found := runtime.values[0]; found {
		t.Fatal("widget cache retained its original peak storage")
	}
	if len(scratch.runtime) != 1 || scratch.runtime[live] != layoutState || len(runtime.values) != 1 || runtime.values[live] != widgetState || scene.HierarchyFor(world).Depth(live) != 1 {
		t.Fatal("reclamation changed live layout or alignment state")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		update()
		world.Maintain()
	}); allocations != 0 {
		t.Fatalf("stable layout maintenance allocated %g times", allocations)
	}
}

func TestPartIndexMapRetentionPreservesLookupAndFramePeaks(t *testing.T) {
	world := ecs.NewWorld()
	parts := world.NewSpawner2[Relation[scene.Text2D], Part[ValuePart]]()
	entities := make([]ecs.Entity, 256)
	for i := range entities {
		entities[i] = parts.Spawn(Relation[scene.Text2D]{Owner: ecs.Entity(i + 1)}, Part[ValuePart]{})
	}
	old := partEntities[ValuePart, scene.Text2D](world)
	world.Maintain()
	for _, entity := range entities[1:] {
		world.Despawn(entity)
	}
	current := partEntities[ValuePart, scene.Text2D](world)
	for range reuse.RetentionFrames - 1 {
		partEntities[ValuePart, scene.Text2D](world)
		world.Maintain()
	}
	// The large population earlier in the first shrinking frame must count.
	old[0] = entities[0]
	if _, found := current[0]; !found {
		t.Fatal("part cache discarded an earlier population too soon")
	}
	delete(old, 0)
	for range 2 {
		partEntities[ValuePart, scene.Text2D](world)
		world.Maintain()
	}
	current = partEntities[ValuePart, scene.Text2D](world)
	old[0] = entities[0]
	if len(current) != 1 || current[1] != entities[0] {
		t.Fatal("part cache retained its original peak storage or lost its live entry")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		FindPart[ValuePart, scene.Text2D](world, 1)
		world.Maintain()
	}); allocations != 0 {
		t.Fatalf("stable part maintenance allocated %g times", allocations)
	}
	for range reuse.RetentionFrames {
		world.Maintain()
	}
	index := world.FrameValue[partIndex[ValuePart, scene.Text2D], partIndex[ValuePart, scene.Text2D]]()
	if index.entities != nil {
		t.Fatal("unused part cache retained its storage")
	}
	if entity, found := FindPart[ValuePart, scene.Text2D](world, 1); !found || entity != entities[0] {
		t.Fatal("lookup failed to rebuild an idle cache")
	}
}
