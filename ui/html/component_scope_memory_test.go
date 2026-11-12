package html

import (
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/html/dom"
)

func TestHTMLComponentScopeSharedAttributesMutationIsolation(t *testing.T) {
	definition, err := DefineScoped(NewTemplate(`<section><div></div><div></div></section>`), "div{color:red}", Behavior[struct{}, struct{}]{})
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	page := New(`<main></main>`)
	page.Width, page.Height = 240, 120
	spawner := world.NewSpawner1[Surface]()
	owner := spawner.Spawn(page)
	first, err := definition.MountComponent(world, owner, struct{}{}, struct{}{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := definition.MountComponent(world, owner, struct{}{}, struct{}{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	a := world.GetRef[htmlComponentInstance](first).Roots[0]
	b := world.GetRef[htmlComponentInstance](second).Roots[0]
	left := Node{world: world, owner: owner, node: a.Children[0]}
	right := Node{world: world, owner: owner, node: a.Children[1]}
	scope := definition.runtime.scopeClass
	if !left.SetAttribute("class", "edited "+scope) || !left.SetHidden(true) || !right.RemoveAttribute("class") {
		t.Fatal("shared scope attributes could not be changed")
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	if left.node.AttrValue("class") != "edited "+scope {
		t.Fatal("removing a sibling class changed the edited node")
	}
	for _, node := range []*dom.Node{a, b, b.Children[0], b.Children[1]} {
		if class, _ := node.Attr("class"); class != scope {
			t.Fatal("scope attribute mutation escaped its node or component instance")
		}
		if _, hidden := node.Attr("hidden"); hidden {
			t.Fatal("hidden attribute escaped its node or component instance")
		}
	}
	if _, ok := definition.runtime.template.Root.Children[0].Attr("class"); ok {
		t.Fatal("mounted scope modified the immutable template")
	}
	if !left.SetHidden(false) || !right.SetAttribute("class", scope) {
		t.Fatal("restoring scope attributes failed")
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
}

func TestHTMLComponentScopePopulationAllocations(t *testing.T) {
	root := dom.NewDocument()
	for range 1000 {
		root.AppendChild(dom.NewElement("td"))
	}
	if allocations := testing.AllocsPerRun(100, func() {
		for _, node := range root.Children {
			node.ShareAttributes(nil)
		}
		prepareHTMLComponentSubtree(root, "scope", nil)
	}); allocations > 3 {
		t.Fatalf("scope population allocated per node: %g allocations", allocations)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		prepareHTMLComponentSubtree(root, "scope", nil)
	}); allocations != 0 {
		t.Fatalf("warm scope preparation allocated %g", allocations)
	}
}
