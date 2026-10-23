package html

import (
	"testing"

	"github.com/diiyw/onix/ecs"
)

func TestHTMLComponentChildSlotReuseAndSnapshots(t *testing.T) {
	type props struct {
		Text      string
		Reference *int
	}
	definition, err := Define[props, string](NewTemplate(`<span></span>`), "", Behavior[props, string]{})
	if err != nil {
		t.Fatal(err)
	}
	instance := &htmlComponentInstance{Next: newHTMLComponentVNodeState()}
	view := &View{instance: instance, template: definition.runtime.template}
	// A valid slot needs a data-ref.
	template, err := compileHTMLComponentTemplate(`<div data-ref="slot"></div><div data-ref="other"></div>`)
	if err != nil {
		t.Fatal(err)
	}
	view.template = template
	reference := 42
	saved := ComponentChild(1, definition, props{"saved", &reference}, "initial")
	if AppendComponentChild[props, string](nil, "slot", 1, definition, props{}, "") || AppendComponentChild(view, "missing", 1, definition, props{}, "") || AppendComponentChild(view, "slot", 1, (*Definition[props, string])(nil), props{}, "") {
		t.Fatal("invalid child accepted")
	}
	AppendComponentChild(view, "slot", 1, definition, props{"first", &reference}, "state")
	AppendComponentChild(view, "slot", 2, definition, props{"removed", &reference}, "removed")
	values := instance.Next.ComponentChildren["slot"]
	first := values[0].values.(*htmlComponentChildValues[props, string])
	second := values[1].values.(*htmlComponentChildValues[props, string])
	// Reset must clear even slots beyond len, while retaining empty workspaces.
	instance.Next.ComponentChildren["slot"] = values[:0]
	instance.Next.reset()
	if *first != (htmlComponentChildValues[props, string]{}) || *second != (htmlComponentChildValues[props, string]{}) {
		t.Fatal("reset retained props or initial state")
	}
	for _, node := range values {
		if node.definition != nil || node.Key != 0 {
			t.Fatal("reset retained child metadata")
		}
	}
	AppendComponentChild(view, "slot", 3, definition, props{"reused", nil}, "new")
	if instance.Next.ComponentChildren["slot"][0].values != first {
		t.Fatal("slot did not reuse parameter storage")
	}
	// Replacing and then extending a slot must not mutate a caller's snapshot.
	view.Components("slot", []ComponentVNode{saved})
	AppendComponentChild(view, "slot", 4, definition, props{"after snapshot", nil}, "")
	view.Components("other", []ComponentVNode{saved})
	view.Components("slot", nil)
	instance.Next.reset()
	snapshot := saved.values.(*htmlComponentChildValues[props, string])
	if snapshot.Props.Text != "saved" || snapshot.Props.Reference != &reference || snapshot.State != "initial" {
		t.Fatal("caller snapshot was mutated by slot reuse")
	}
}

func TestHTMLAppendComponentChildrenReconcile(t *testing.T) {
	type props struct{ Label string }
	type entry struct {
		Key   uint64
		Label string
		Kind  int
	}
	makeDefinition := func() *Definition[props, int] {
		d, err := Define[props, int](NewTemplate(`<span data-ref="label"></span>`), "", Behavior[props, int]{Render: func(c RenderContext[props, int], v *View) { v.Text("label", c.Props.Label) }})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	definitions := []*Definition[props, int]{makeDefinition(), makeDefinition()}
	other, err := Define[string, int](NewTemplate(`<b data-ref="label"></b>`), "", Behavior[string, int]{Render: func(c RenderContext[string, int], v *View) { v.Text("label", *c.Props) }})
	if err != nil {
		t.Fatal(err)
	}
	entries := []entry{{1, "one", 0}, {2, "two", 0}}
	parentDefinition, err := Define[struct{}, struct{}](NewTemplate(`<section data-ref="slot"></section>`), "", Behavior[struct{}, struct{}]{Render: func(_ RenderContext[struct{}, struct{}], v *View) {
		for _, e := range entries {
			if e.Kind == 2 {
				AppendComponentChild(v, "slot", e.Key, other, e.Label, 999)
			} else {
				AppendComponentChild(v, "slot", e.Key, definitions[e.Kind], props{e.Label}, 999)
			}
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, New(`<main></main>`))
	owner := world.Spawn(bundle)
	parent, err := MountComponent(world, owner, parentDefinition, struct{}{}, struct{}{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	sync := func() []htmlComponentChildItem {
		t.Helper()
		UpdateComponentProps[struct{}, struct{}](world, parent, struct{}{})
		if err := Sync(world); err != nil {
			t.Fatal(err)
		}
		instance, _ := ecs.Get[htmlComponentInstance](world, parent)
		items := instance.ComponentChildren["slot"].Items
		for i, item := range items {
			child, _ := ecs.Get[htmlComponentInstance](world, item.Entity)
			if instance.Refs["slot"].Children[i] != child.Roots[0] {
				t.Fatal("child roots out of order")
			}
		}
		return items
	}
	items := sync()
	first, second := items[0].Entity, items[1].Entity
	UpdateComponentState[props, int](world, first, func(s *int) { *s = 77 })
	for round := 0; round < 6; round++ {
		entries = []entry{{2, "two updated", 0}, {1, "one updated", 0}, {1, "ignored duplicate", 0}}
		items = sync()
		if len(items) != 2 || items[0].Entity != second || items[1].Entity != first {
			t.Fatal("keyed identity lost on reorder")
		}
		child, _ := ecs.Get[Component[props, int]](world, first)
		if child.State != 77 || child.Props.Label != "one updated" {
			t.Fatalf("child state or props = %#v", child)
		}
	}
	entries = []entry{{1, "different definition", 1}}
	items = sync()
	replacement := items[0].Entity
	if world.IsAlive(first) || world.IsAlive(second) || replacement == first {
		t.Fatal("definition replacement failed to unmount old children")
	}
	shrunk, _ := ecs.Get[htmlComponentInstance](world, parent)
	for _, buffer := range []htmlComponentVNodeState{shrunk.Current, shrunk.Next} {
		values := buffer.ComponentChildren["slot"]
		for _, node := range values[:cap(values)][1:] {
			if node.values != nil || node.definition != nil {
				t.Fatal("shortened slot retained removed parameter workspaces")
			}
		}
	}
	entries = []entry{{1, "different type", 2}}
	items = sync()
	typedReplacement := items[0].Entity
	child, ok := ecs.Get[Component[string, int]](world, typedReplacement)
	if !ok || child.Props != "different type" || child.State != 999 || world.IsAlive(replacement) {
		t.Fatal("typed parameter storage was not replaced")
	}
	entries = nil
	items = sync()
	if len(items) != 0 || world.IsAlive(typedReplacement) {
		t.Fatal("omitted slot retained children")
	}
	instance, _ := ecs.Get[htmlComponentInstance](world, parent)
	for _, buffer := range []htmlComponentVNodeState{instance.Current, instance.Next} {
		values := buffer.ComponentChildren["slot"]
		for _, node := range values[:cap(values)] {
			if node.values != nil || node.definition != nil {
				t.Fatal("omitted slot retained parameter workspaces")
			}
		}
	}
	entries = []entry{{3, "new after omission", 0}}
	items = sync()
	if len(items) != 1 || items[0].Key != 3 {
		t.Fatal("slot failed to mount after omission")
	}
}

func TestHTMLAppendComponentChildrenWarmAllocations(t *testing.T) {
	definition, err := Define[htmlComponentChildProps, struct{}](NewTemplate(`<span></span>`), "", Behavior[htmlComponentChildProps, struct{}]{})
	if err != nil {
		t.Fatal(err)
	}
	template, err := compileHTMLComponentTemplate(`<div data-ref="slot"></div>`)
	if err != nil {
		t.Fatal(err)
	}
	instance := &htmlComponentInstance{Current: newHTMLComponentVNodeState(), Next: newHTMLComponentVNodeState()}
	view := &View{instance: instance, template: template}
	render := func() {
		view.reset()
		for index := uint64(0); index < 100; index++ {
			AppendComponentChild(view, "slot", index, definition, htmlComponentChildProps{"updated"}, struct{}{})
		}
		instance.Current, instance.Next = instance.Next, instance.Current
		instance.Next.reset()
	}
	render()
	render()
	if allocs := testing.AllocsPerRun(100, render); allocs != 0 {
		t.Fatalf("warm child declarations allocate %g times", allocs)
	}
}
