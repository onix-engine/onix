package html

import (
	"testing"
)

func TestHTMLControlByIDUsesDocumentOrderAndSurface(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<input id="field" value="first"><input id="field" value="second"><div id="blocked">first</div><input id="blocked"><textarea id="notes">text</textarea><input id="typed" type="checkbox">`)
	entity, ok := ControlByID(world, owner, "field")
	if !ok || world.GetRef[Control](entity).Value != "first" {
		t.Fatal("duplicate id did not resolve to first element")
	}
	if _, ok := ControlByID(world, owner, "blocked"); ok {
		t.Fatal("non-control first match was skipped")
	}
	for _, id := range []string{"notes", "typed"} {
		if _, ok := ControlByID(world, owner, id); !ok {
			t.Fatalf("missing control %s", id)
		}
	}
	other := New(`<input id="field" value="other">`)
	other.Width, other.Height = 200, 100
	spawner := world.NewSpawner1[Surface]()
	second := spawner.Spawn(other)
	if _, ok := ControlByID(world, second, "field"); ok {
		t.Fatal("unrendered surface returned a control")
	}
	incrementalSync(t, world)
	foreign, ok := ControlByID(world, second, "field")
	if !ok || foreign == entity || world.GetRef[Control](foreign).Owner != second {
		t.Fatal("lookup crossed HTML surfaces")
	}
	version := world.ComponentVersion[htmlRenderState]()
	if allocs := testing.AllocsPerRun(100, func() { ControlByID(world, owner, "field") }); allocs != 0 {
		t.Fatalf("indexed lookup allocations = %v", allocs)
	}
	if world.ComponentVersion[htmlRenderState]() != version {
		t.Fatal("clean lookup marked state changed")
	}
}

func TestHTMLControlByIDInvalidatesRenamedRemovedAndRebuiltNodes(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="root"><input id="field"></div>`)
	old, _ := ControlByID(world, owner, "field")
	node := incrementalHTMLNode(t, world, owner, "field")
	node.SetAttribute("id", "renamed")
	if _, ok := ControlByID(world, owner, "field"); ok {
		t.Fatal("dirty index returned old id")
	}
	if entity, ok := ControlByID(world, owner, "renamed"); !ok || entity != old {
		t.Fatal("renaming lost retained control")
	}
	incrementalSync(t, world)
	root := incrementalHTMLNode(t, world, owner, "root")
	replacement := node.Clone()
	root.Clear()
	root.Append(replacement)
	if _, ok := ControlByID(world, owner, "renamed"); ok {
		t.Fatal("replacement returned detached control before render")
	}
	incrementalSync(t, world)
	next, ok := ControlByID(world, owner, "renamed")
	if !ok || next == old {
		t.Fatal("replacement did not rebuild control lookup")
	}
	world.Despawn(next)
	if _, ok := ControlByID(world, owner, "renamed"); ok {
		t.Fatal("lookup returned a retired entity")
	}
	Set(world, owner, `<input id="fresh">`)
	if _, ok := ControlByID(world, owner, "renamed"); ok {
		t.Fatal("new source returned old rendered tree")
	}
}
