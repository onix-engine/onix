package html

import (
	"strings"
	"testing"

	"github.com/onix-engine/onix/ecs"
)

func TestHTMLComponentBindingsActivateAfterStaticRenders(t *testing.T) {
	type state struct{ Active, Patch bool }
	definition, err := Define[struct{}, state](NewTemplate(`<div data-ref="root"></div>`), "", Behavior[struct{}, state]{
		Render: func(context RenderContext[struct{}, state], view *View) {
			if !context.State.Active {
				return
			}
			if context.State.Patch {
				view.PatchBindings()
			}
			view.Text("root", "active")
			view.Attr("root", "title", "bound")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	pages := world.NewSpawner1[Surface]()
	owner := pages.Spawn(New(`<main></main>`))
	entity, err := definition.MountComponent(world, owner, struct{}{}, state{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	render := func() {
		instance := world.GetMut[htmlComponentInstance](entity)
		definition.runtime.render(world, entity, instance)
	}
	for range 3 {
		render()
	}
	instance := world.GetRef[htmlComponentInstance](entity)
	if instance.Handlers != nil || instance.EventNodes != nil || instance.Keyed != nil || instance.ComponentChildren != nil || instance.Current.Texts != nil || instance.Next.Attrs != nil {
		t.Fatal("static renders allocated unused binding maps")
	}
	component := world.GetMut[Component[struct{}, state]](entity)
	component.State = state{Active: true, Patch: true}
	render()
	instance = world.GetRef[htmlComponentInstance](entity)
	if node := instance.Refs["root"]; node.AttrValue("title") != "bound" || !strings.Contains(node.String(), "active") {
		t.Fatal("first patch failed to activate text and attribute bindings")
	}
	component = world.GetMut[Component[struct{}, state]](entity)
	component.State = state{Active: true}
	render()
	render()
	if allocations := testing.AllocsPerRun(100, render); allocations != 0 {
		t.Fatalf("warm binding reconciliation allocated %g times", allocations)
	}
	world.GetMut[Component[struct{}, state]](entity).State = state{}
	render()
	instance = world.GetRef[htmlComponentInstance](entity)
	node := instance.Refs["root"]
	if _, present := node.Attr("title"); present || strings.Contains(node.String(), "active") {
		t.Fatal("omitted bindings were not removed")
	}
}

func TestHTMLComponentCSSReferencesPreserveOrderAndBase(t *testing.T) {
	world := ecs.NewWorld()
	pages := world.NewSpawner1[Surface]()
	owner := pages.Spawn(New(`<main></main>`))
	first, second := &htmlDefinitionRuntime{}, &htmlDefinitionRuntime{}
	const firstCSS, secondCSS = `.first{color:red}`, `.second{color:blue}`
	attach := func(definition *htmlDefinitionRuntime, cssText string) {
		if err := attachHTMLComponentCSS(world, owner, definition, cssText); err != nil {
			t.Fatal(err)
		}
	}
	detach := func(definition *htmlDefinitionRuntime) {
		if err := detachHTMLComponentCSS(world, owner, definition); err != nil {
			t.Fatal(err)
		}
	}
	css := func() string { return world.GetRef[Surface](owner).CSS }
	attach(first, firstCSS)
	attach(second, secondCSS)
	if allocations := testing.AllocsPerRun(100, func() {
		attach(first, firstCSS)
		detach(first)
	}); allocations != 0 {
		t.Fatalf("retained stylesheet references allocated %g times", allocations)
	}
	SetBaseCSS(world, owner, `.base{display:block}`)
	if want := ".base{display:block}\n" + firstCSS + "\n" + secondCSS; css() != want {
		t.Fatalf("stylesheet=%q, want %q", css(), want)
	}
	detach(first)
	if strings.Contains(css(), firstCSS) || !strings.Contains(css(), secondCSS) {
		t.Fatal("last reference removed the wrong stylesheet")
	}
	attach(first, firstCSS)
	if want := ".base{display:block}\n" + secondCSS + "\n" + firstCSS; css() != want {
		t.Fatalf("remounted stylesheet=%q, want %q", css(), want)
	}
	detach(first)
	detach(second)
	if css() != `.base{display:block}` {
		t.Fatal("component removal lost the base stylesheet")
	}
}
