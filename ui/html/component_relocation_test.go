package html

import "testing"

func TestHTMLComponentChildrenSurviveParentRelocation(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<main></main>`)
	leaf := func(label string) *Definition[struct{}, struct{}] {
		d, err := Define[struct{}, struct{}](NewTemplate(`<span>`+label+`</span>`), "", Behavior[struct{}, struct{}]{})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	first, second := leaf("first"), leaf("second")
	child := first
	parentDefinition, err := Define[struct{}, struct{}](NewTemplate(`<section data-ref="slot"></section>`), "", Behavior[struct{}, struct{}]{
		Render: func(_ RenderContext[struct{}, struct{}], v *View) {
			v.AppendComponentChild("slot", 1, child, struct{}{}, struct{}{})
			v.AppendComponentChild("slot", 2, child, struct{}{}, struct{}{})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	dummyA, err := first.MountComponent(world, owner, struct{}{}, struct{}{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	dummyB, err := first.MountComponent(world, owner, struct{}{}, struct{}{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := parentDefinition.MountComponent(world, owner, struct{}{}, struct{}{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	incrementalSync(t, world)
	initial, _ := world.Get[htmlComponentInstance](parent)
	if initial.ChildrenPending || initial.Rendering || !initial.Mounted {
		t.Fatal("parent render state was lost while growing child storage")
	}
	// Move the children before the parent in their shared archetype. Replacing
	// either child now moves the parent out of its current ECS storage row.
	UnmountComponent(world, dummyA)
	UnmountComponent(world, dummyB)
	for round := range 4 {
		before, _ := world.Get[htmlComponentInstance](parent)
		old := [2]htmlComponentChildItem{before.ComponentChildren["slot"].Items[0], before.ComponentChildren["slot"].Items[1]}
		child = second
		label := "second"
		if round%2 != 0 {
			child, label = first, "first"
		}
		UpdateComponentProps[struct{}, struct{}](world, parent, struct{}{})
		incrementalSync(t, world)
		instance, _ := world.Get[htmlComponentInstance](parent)
		if instance.ChildrenPending || instance.Rendering || !instance.Mounted {
			t.Fatal("parent render state was lost during relocation")
		}
		items := instance.ComponentChildren["slot"].Items
		if len(items) != 2 || len(instance.Children) != 2 || len(instance.Refs["slot"].Children) != 2 {
			t.Fatal("replacement lost children or retained old roots")
		}
		for i, item := range items {
			if world.IsAlive(old[i].Entity) {
				t.Fatal("replaced child is still alive")
			}
			current, ok := world.Get[htmlComponentInstance](item.Entity)
			if !ok || current.Page != owner || instance.Refs["slot"].Children[i] != current.Roots[0] || htmlNodeText(current.Roots[0]) != label {
				t.Fatal("replacement child content or DOM order is incorrect")
			}
		}
	}
}
