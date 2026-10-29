package html

import (
	"testing"

	"github.com/diiyw/onix/ecs"
)

func TestHTMLRenderContextValueReentryAcrossWorlds(t *testing.T) {
	var definition *Definition[int, int]
	var secondWorld *ecs.World
	var second ecs.Entity
	var nested bool
	var saved []RenderContext[int, int]
	var err error
	definition, err = Define[int, int](NewTemplate(`<span></span>`), "", Behavior[int, int]{Render: func(c RenderContext[int, int], _ *View) {
		saved = append(saved, c)
		if nested {
			nested = false
			definition.runtime.render(secondWorld, second, secondWorld.GetMut[htmlComponentInstance](second))
		}
		*c.State = *c.Props
	}})
	if err != nil {
		t.Fatal(err)
	}
	mount := func(props int) (*ecs.World, ecs.Entity) {
		world := ecs.NewWorld()
		bundle := ecs.NewBundle(1)
		bundle.Put(New(`<main></main>`))
		owner := world.Spawn(bundle)
		entity, err := definition.MountComponent(world, owner, props, 0, 1)
		if err != nil {
			t.Fatal(err)
		}
		if err := Sync(world); err != nil {
			t.Fatal(err)
		}
		return world, entity
	}
	firstWorld, first := mount(11)
	secondWorld, second = mount(22)
	saved = saved[:0]
	nested = true
	definition.runtime.render(firstWorld, first, firstWorld.GetMut[htmlComponentInstance](first))
	if len(saved) != 2 || saved[0].World != firstWorld || saved[1].World != secondWorld || saved[0].Component != first || saved[1].Component != second {
		t.Fatal("nested renders mixed component context identity")
	}
	firstComponent := firstWorld.GetMut[Component[int, int]](first)
	secondComponent := secondWorld.GetMut[Component[int, int]](second)
	if firstComponent.State != 11 || secondComponent.State != 22 || saved[0].Props != &firstComponent.Props || saved[1].State != &secondComponent.State {
		t.Fatal("render props/state no longer refer to their component")
	}
	firstContext := saved[0]
	UpdateComponentProps[int, int](secondWorld, second, 33)
	definition.runtime.render(secondWorld, second, secondWorld.GetMut[htmlComponentInstance](second))
	if saved[0] != firstContext {
		t.Fatal("a later render mutated a saved context value")
	}
}
