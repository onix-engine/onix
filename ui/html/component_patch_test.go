package html

import (
	"github.com/diiyw/onix/ecs"
	"testing"
)

func TestHTMLComponentBindingPatchPreservesChildrenAndEvents(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, "<main></main>")
	child, err := Define[htmlComponentChildProps, struct{}](NewTemplate(`<span data-ref="label"></span>`), "", Behavior[htmlComponentChildProps, struct{}]{Render: func(c RenderContext[htmlComponentChildProps, struct{}], v *View) { v.Text("label", c.Props.Label) }})
	if err != nil {
		t.Fatal(err)
	}
	mode, clicks := 0, 0
	definition, err := Define[struct{}, struct{}](NewTemplate(`<section><button data-ref="button" title="static">Static</button><span data-ref="status"></span><div data-ref="slot"></div><div data-ref="keyed"></div></section>`), "", Behavior[struct{}, struct{}]{
		Render: func(_ RenderContext[struct{}, struct{}], v *View) {
			if mode == 1 {
				v.PatchBindings()
				v.Text("status", "patched")
				v.RemoveAttr("button", "title")
				v.Attr("button", "aria-label", "patched")
				return
			}
			if mode == 2 {
				return
			}
			v.Text("status", "full")
			v.Attr("button", "title", "dynamic")
			v.Attr("button", "class", "retained")
			v.Event("button", EventClick)
			AppendComponentChild(v, "slot", 1, child, htmlComponentChildProps{Label: "child"}, struct{}{})
			v.Keyed("keyed", []VNode{TextVNode(2, "span", "keyed")})
		}, HandleEvent: func(c *ComponentEventContext[struct{}, struct{}]) {
			if c.Ref == "button" {
				clicks++
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	entity, err := MountComponent(world, owner, definition, struct{}{}, struct{}{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	incrementalSync(t, world)
	instance := ecs.GetRef[htmlComponentInstance](world, entity)
	originalChild := instance.ComponentChildren["slot"].Items[0].Entity
	originalKeyed := instance.Refs["keyed"].Children[0]
	mode = 1
	UpdateComponentState[struct{}, struct{}](world, entity, func(*struct{}) {})
	incrementalSync(t, world)
	instance = ecs.GetRef[htmlComponentInstance](world, entity)
	if got := htmlNodeText(instance.Refs["status"]); got != "patched" {
		t.Fatal(got)
	}
	if class, _ := instance.Refs["button"].Attr("class"); class != "retained" {
		t.Fatal("patch lost unspecified attribute")
	}
	if _, exists := instance.Refs["button"].Attr("title"); exists {
		t.Fatal("patch did not remove attribute")
	}
	if !world.IsAlive(originalChild) || instance.ComponentChildren["slot"].Items[0].Entity != originalChild || instance.Refs["keyed"].Children[0] != originalKeyed {
		t.Fatal("patch replaced owned children")
	}
	definition.runtime.event(world, entity, "button", Event{Kind: EventClick}, nil, false)
	if clicks != 1 || len(instance.Current.Events) != 1 {
		t.Fatal("patch lost event handler")
	}
	// Repeated identical scalar patches do not rebuild the view or allocate.
	if n := testing.AllocsPerRun(100, func() { definition.runtime.render(world, entity, instance) }); n != 0 {
		t.Fatalf("warm patch allocated %g", n)
	}
	mode = 0
	UpdateComponentState[struct{}, struct{}](world, entity, func(*struct{}) {})
	incrementalSync(t, world)
	instance = ecs.GetRef[htmlComponentInstance](world, entity)
	if title, _ := instance.Refs["button"].Attr("title"); title != "dynamic" {
		t.Fatal("full render did not restore patched attribute")
	}
	if _, exists := instance.Refs["button"].Attr("aria-label"); exists {
		t.Fatal("full render retained patch-only attribute")
	}
	mode = 2
	UpdateComponentState[struct{}, struct{}](world, entity, func(*struct{}) {})
	incrementalSync(t, world)
	instance = ecs.GetRef[htmlComponentInstance](world, entity)
	if world.IsAlive(originalChild) || len(instance.Current.Events) != 0 || htmlNodeText(instance.Refs["status"]) != "" || len(instance.Refs["keyed"].Children) != 0 {
		t.Fatal("full render did not remove omitted bindings")
	}
}
