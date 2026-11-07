package ui

import (
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/scene"
	htmlui "github.com/onix-engine/onix/ui/html"
)

type htmlFocusMigration struct{}

func TestHTMLFocusLookupPreservesScopedQueryOrdinals(t *testing.T) {
	world := ecs.NewWorld()
	owner := world.Spawn(ecs.NewBundle(0))
	spawner := world.NewSpawner1[htmlui.Focusable]()
	var entities []ecs.Entity
	for range 6 {
		entities = append(entities, spawner.Spawn(htmlui.Focusable{Owner: owner, Tag: "button", Type: "button"}))
	}
	state := &uiEventState{}
	world.InsertResource(state)
	check := func() {
		t.Helper()
		ordinal := 0
		for entity, access := range world.Query[htmlui.Focusable]().All {
			value := access.Get()
			if value.ID != "" || value.Tag != "button" || value.Type != "button" {
				continue
			}
			state.Focused = entity
			captured, ok := captureHTMLFocusRebuildState(world)
			if !ok || captured.Ordinal != ordinal {
				t.Fatalf("capture for %v = %+v, want ordinal %d", entity, captured, ordinal)
			}
			if matched, ok := matchingHTMLFocusable(world, value, ordinal); !ok || matched != entity {
				t.Fatalf("ordinal %d restored %v, want %v", ordinal, matched, entity)
			}
			ordinal++
		}
	}
	check()
	world.Insert(entities[0], htmlFocusMigration{})
	check()
	world.GetMut[htmlui.Focusable](entities[1]).ID = "named"
	world.GetMut[htmlui.Focusable](entities[2]).Tag = "input"
	check()
	var first, second ecs.QueryScope
	first.Reset(entities[:3])
	second.Reset(entities[3:])
	world.SetQueryScope(&first)
	check()
	world.SetQueryScope(&second)
	check()
	first.Reset(entities[3:])
	world.SetQueryScope(&first)
	check()
	world.SetQueryScope(nil)
	world.Despawn(entities[0])
	check()
	if allocs := testing.AllocsPerRun(100, func() {
		captureHTMLFocusRebuildState(world)
	}); allocs != 0 {
		t.Fatalf("warm focus capture allocated %g times", allocs)
	}
}

func TestHTMLAutofocusCandidatesTrackVisibilityActivityAndScopes(t *testing.T) {
	world := ecs.NewWorld()
	owner := world.Spawn(ecs.NewBundle(0))
	spawner := world.NewSpawner2[htmlui.Focusable, scene.Parent]()
	a := spawner.Spawn(htmlui.Focusable{Owner: owner, Tag: "button", Type: "button"}, scene.Parent{Entity: owner})
	b := spawner.Spawn(htmlui.Focusable{Owner: owner, Tag: "button", Type: "button"}, scene.Parent{Entity: owner})
	state := &uiEventState{}
	world.InsertResource(state)
	autofocusHTMLElement(world)
	if state.Focused != 0 {
		t.Fatal("focused an element without autofocus")
	}
	world.GetMut[htmlui.Focusable](b).AutoFocus = true
	world.Insert(owner, scene.Visibility{Visible: false})
	autofocusHTMLElement(world)
	if state.Focused != 0 {
		t.Fatal("focused a hidden ancestor's autofocus candidate")
	}
	world.GetMut[scene.Visibility](owner).Visible = true
	world.Insert(owner, scene.Activity{Active: false})
	autofocusHTMLElement(world)
	if state.Focused != 0 {
		t.Fatal("focused an inactive ancestor's autofocus candidate")
	}
	world.GetMut[scene.Activity](owner).Active = true
	var scope ecs.QueryScope
	scope.Reset([]ecs.Entity{a})
	world.SetQueryScope(&scope)
	autofocusHTMLElement(world)
	if state.Focused != 0 {
		t.Fatal("autofocus escaped query scope")
	}
	scope.Reset([]ecs.Entity{a, b})
	autofocusHTMLElement(world)
	if state.Focused != b {
		t.Fatalf("newly visible scoped autofocus = %v, want %v", state.Focused, b)
	}
	focus(world, state, 0)
	autofocusHTMLElement(world)
	if state.Focused != 0 {
		t.Fatal("autofocus was applied twice to one owner")
	}
	world.Remove[htmlAutofocusApplied](owner)
	autofocusHTMLElement(world)
	if state.Focused != b {
		t.Fatal("removing the applied marker did not restore autofocus eligibility")
	}
	focus(world, state, 0)
	if allocs := testing.AllocsPerRun(100, func() { autofocusHTMLElement(world) }); allocs != 0 {
		t.Fatalf("warm autofocus lookup allocated %g times", allocs)
	}
}
