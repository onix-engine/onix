package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestHTMLBridgeKeepsChangesFromEnclosingSystemTick(t *testing.T) {
	world := ecs.NewWorld()
	spawner := world.NewSpawner1[htmlui.Control]()
	first := spawner.Spawn(htmlui.Control{Tag: "input", Value: "first", RenderRevision: 1})
	second := spawner.Spawn(htmlui.Control{Tag: "input", Value: "second", RenderRevision: 1})
	editables := world.NewSpawner1[htmlui.Editable]()
	editor := editables.Spawn(htmlui.Editable{Value: "editable", RenderRevision: 1})
	system := ecs.SystemConfig{Name: "bridge_then_change", System: func(context *ecs.SystemContext) error {
		enrichHTMLControls(context.World)
		control := context.World.GetMut[htmlui.Control](first)
		control.Value, control.RenderRevision = "changed", 2
		editable := context.World.GetMut[htmlui.Editable](editor)
		editable.Value, editable.RenderRevision = "edited", 2
		return nil
	}}
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	if err := (ecs.SystemConfig{Name: "bridge_next_tick", System: func(context *ecs.SystemContext) error {
		enrichHTMLControls(context.World)
		return nil
	}}).Run(world); err != nil {
		t.Fatal(err)
	}
	if got := world.GetRef[TextField](first).Value; got != "changed" {
		t.Fatalf("missed same-tick control update: %q", got)
	}
	if got := world.GetRef[TextField](second).Value; got != "second" {
		t.Fatalf("unrelated control changed: %q", got)
	}
	if got := world.GetRef[TextField](editor).Value; got != "edited" {
		t.Fatalf("missed same-tick editable update: %q", got)
	}
}

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
	if !world.Has[TextField](first) || world.Has[TextField](second) {
		t.Fatal("bridge did not honor query scope")
	}
	scope.Reset([]ecs.Entity{first, second, text})
	enrichHTMLControls(world)
	if !world.Has[TextField](second) || !world.Has[Relation[scene.Text2D]](text) {
		t.Fatal("expanded scope did not initialize control and text")
	}
	world.Remove[Relation[scene.Text2D]](text)
	enrichHTMLControls(world)
	if !world.Has[Relation[scene.Text2D]](text) {
		t.Fatal("removed relation was not restored")
	}
	var otherScope ecs.QueryScope
	otherScope.Reset([]ecs.Entity{second})
	world.SetQueryScope(&otherScope)
	enrichHTMLControls(world)
	control := world.GetMut[htmlui.Control](first)
	control.Value, control.RenderRevision = "changed outside scope", 2
	enrichHTMLControls(world)
	world.SetQueryScope(&scope)
	enrichHTMLControls(world)
	if got := world.GetRef[TextField](first).Value; got != "changed outside scope" {
		t.Fatalf("returning scope lost update: %q", got)
	}
	world.SetQueryScope(nil)
	enrichHTMLControls(world)
	if allocations := testing.AllocsPerRun(100, func() { enrichHTMLControls(world) }); allocations != 0 {
		t.Fatalf("stable bridge allocated %g objects", allocations)
	}
}
