package ui

import (
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/scene"
	htmlui "github.com/onix-engine/onix/ui/html"
)

func TestHTMLBridgeTracksScopeAndMissingRelations(t *testing.T) {
	world := ecs.NewWorld()
	controls := world.NewSpawner1[htmlui.Control]()
	first := controls.Spawn(htmlui.Control{Tag: "input", RenderRevision: 1})
	second := controls.Spawn(htmlui.Control{Tag: "input", RenderRevision: 1})
	texts := world.NewSpawner1[htmlui.ControlText]()
	text := texts.Spawn(htmlui.ControlText{Control: second})
	var scope ecs.QueryScope
	scope.Reset([]ecs.Entity{first})
	world.SetQueryScope(&scope)
	enrichHTMLControls(world)
	if !world.Has[Input](first) || world.Has[Input](second) {
		t.Fatal("bridge did not honor query scope")
	}
	scope.Reset([]ecs.Entity{first, second, text})
	enrichHTMLControls(world)
	if !world.Has[Input](second) || !world.Has[Relation[scene.Text2D]](text) {
		t.Fatal("expanded scope did not initialize control and text")
	}
	world.Remove[Relation[scene.Text2D]](text)
	enrichHTMLControls(world)
	if !world.Has[Relation[scene.Text2D]](text) {
		t.Fatal("removed relation was not restored")
	}
	world.SetQueryScope(nil)
	enrichHTMLControls(world)
	if allocations := testing.AllocsPerRun(100, func() { enrichHTMLControls(world) }); allocations != 0 {
		t.Fatalf("stable bridge allocated %g objects", allocations)
	}
}

func TestHTMLBridgeBatchesEditableAndControlParts(t *testing.T) {
	world := ecs.NewWorld()
	controlSpawner := world.NewSpawner1[htmlui.Control]()
	control := controlSpawner.Spawn(htmlui.Control{Tag: "textarea", RenderRevision: 1})
	editableSpawner := world.NewSpawner1[htmlui.Editable]()
	editable := editableSpawner.Spawn(htmlui.Editable{})
	textSpawner := world.NewSpawner1[htmlui.EditableText]()
	text := textSpawner.Spawn(htmlui.EditableText{Editable: editable})
	visualSpawner := world.NewSpawner1[htmlui.ControlVisual]()
	selection := visualSpawner.Spawn(htmlui.ControlVisual{Control: control, Selection: true, Index: 3})
	thumb := visualSpawner.Spawn(htmlui.ControlVisual{Control: control, Kind: htmlui.ControlVisualScrollThumbVertical})
	enrichHTMLControls(world)
	if !world.Has[Textarea](control) || !world.Has[Control](editable) ||
		world.GetRef[Relation[scene.Text2D]](text).Owner != editable || !world.Has[Part[ValuePart]](text) ||
		world.GetRef[Relation[SurfaceStyle]](selection).Owner != control || world.GetRef[Indexed[SelectionPart]](selection).Value != 3 ||
		!world.Has[Part[ScrollThumbVerticalPart]](thumb) {
		t.Fatal("control and editable parts were not bound together")
	}
	world.Remove[Relation[SurfaceStyle]](thumb)
	enrichHTMLControls(world)
	if world.GetRef[Relation[SurfaceStyle]](thumb).Owner != control {
		t.Fatal("removed visual relation was not repaired")
	}
}
