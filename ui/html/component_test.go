package html

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
)

type htmlComponentCounterProps struct {
	Step int
}

type htmlComponentChildProps struct {
	Label string
}

type htmlComponentParentState struct {
	Show   bool
	Second bool
	Label  string
}

func TestHTMLComponentVNodeReconcilesTypedChildren(t *testing.T) {
	var unmounted int
	childDefinition, err := Define[htmlComponentChildProps, struct{}](
		NewTemplate(`<span data-ref="label"></span>`), "", Behavior[htmlComponentChildProps, struct{}]{
			Render: func(context RenderContext[htmlComponentChildProps, struct{}], view *View) {
				view.Text("label", context.Props.Label)
			},
			Unmount: func(*ComponentContext[htmlComponentChildProps, struct{}]) {
				unmounted++
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	parentDefinition, err := Define[struct{}, htmlComponentParentState](
		NewTemplate(`<section data-ref="slot"></section>`), "", Behavior[struct{}, htmlComponentParentState]{
			Render: func(context RenderContext[struct{}, htmlComponentParentState], view *View) {
				if context.State.Show {
					nodes := []ComponentVNode{
						ComponentChild(7, childDefinition, htmlComponentChildProps{Label: context.State.Label}, struct{}{}),
					}
					if context.State.Second {
						nodes = append(nodes, ComponentChild(8, childDefinition, htmlComponentChildProps{Label: "second"}, struct{}{}))
					}
					view.Components("slot", nodes)
				}
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main>`)
	page.Width, page.Height = 240, 120
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	parent, err := MountComponent(world, owner, parentDefinition, struct{}{}, htmlComponentParentState{Show: true, Label: "one"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	parentInstance, _ := ecs.Get[htmlComponentInstance](world, parent)
	children := parentInstance.ComponentChildren["slot"].Items
	if len(children) != 1 || !world.IsAlive(children[0].Entity) {
		t.Fatalf("component children = %#v", children)
	}
	child := children[0].Entity
	childInstance, _ := ecs.Get[htmlComponentInstance](world, child)
	if got := htmlNodeText(childInstance.Refs["label"]); got != "one" {
		t.Fatalf("initial child text = %q", got)
	}

	if !UpdateComponentState[struct{}, htmlComponentParentState](world, parent, func(state *htmlComponentParentState) {
		state.Label = "two"
	}) {
		t.Fatal("parent state update failed")
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	parentInstance, _ = ecs.Get[htmlComponentInstance](world, parent)
	if len(parentInstance.ComponentChildren["slot"].Items) != 1 || parentInstance.ComponentChildren["slot"].Items[0].Entity != child {
		t.Fatal("same keyed child was not reused")
	}
	childInstance, _ = ecs.Get[htmlComponentInstance](world, child)
	if got := htmlNodeText(childInstance.Refs["label"]); got != "two" {
		t.Fatalf("updated child text = %q", got)
	}
	if !UpdateComponentState[struct{}, htmlComponentParentState](world, parent, func(state *htmlComponentParentState) {
		state.Second = true
	}) {
		t.Fatal("parent second child update failed")
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	parentInstance, _ = ecs.Get[htmlComponentInstance](world, parent)
	if len(parentInstance.ComponentChildren["slot"].Items) != 2 {
		t.Fatalf("component children after insert = %#v", parentInstance.ComponentChildren["slot"].Items)
	}
	second := parentInstance.ComponentChildren["slot"].Items[1].Entity
	secondInstance, _ := ecs.Get[htmlComponentInstance](world, second)
	if len(parentInstance.Refs["slot"].Children) != 2 || parentInstance.Refs["slot"].Children[0] != childInstance.Roots[0] || parentInstance.Refs["slot"].Children[1] != secondInstance.Roots[0] {
		t.Fatalf("child roots are not ordered: slot=%#v first=%p second=%p", parentInstance.Refs["slot"].Children, childInstance.Roots[0], secondInstance.Roots[0])
	}

	if !UpdateComponentState[struct{}, htmlComponentParentState](world, parent, func(state *htmlComponentParentState) {
		state.Show = false
	}) {
		t.Fatal("parent hide update failed")
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	if world.IsAlive(child) || world.IsAlive(second) || unmounted != 2 {
		t.Fatalf("child removal alive=%v unmounted=%d", world.IsAlive(child), unmounted)
	}
}

func TestHTMLComponentDefersUnmountRequestedByRender(t *testing.T) {
	rendered := 0
	unmounted := 0
	definition, err := Define[struct{}, struct{}](
		NewTemplate(`<div data-ref="root">content</div>`), "",
		Behavior[struct{}, struct{}]{
			Render: func(context RenderContext[struct{}, struct{}], _ *View) {
				rendered++
				if rendered == 1 && !UnmountComponent(context.World, context.Component) {
					t.Fatalf("render-time unmount was not accepted")
				}
			},
			Unmount: func(*ComponentContext[struct{}, struct{}]) {
				unmounted++
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main>`)
	page.Width, page.Height = 200, 100
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	component, err := MountComponent(world, owner, definition, struct{}{}, struct{}{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	if world.IsAlive(component) || rendered != 1 || unmounted != 1 {
		t.Fatalf("render-time unmount alive=%v rendered=%d unmounted=%d", world.IsAlive(component), rendered, unmounted)
	}
	root, err := Root(world, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(root.node.Children) != 1 || root.node.Children[0].Data != "main" {
		t.Fatalf("DOM after render-time unmount = %#v", root.node.Children)
	}
}

func TestHydrateHTMLComponentAdoptsExistingDOM(t *testing.T) {
	definition, err := Define[struct{}, struct{}](
		NewTemplate(`<section data-ref="root"><span data-ref="label"></span></section>`), "",
		Behavior[struct{}, struct{}]{
			Render: func(_ RenderContext[struct{}, struct{}], view *View) {
				view.Text("label", "client")
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main><section data-ref="root"><span data-ref="label">server</span></section>`)
	page.Width, page.Height = 240, 120
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	root, err := Root(world, owner)
	if err != nil {
		t.Fatal(err)
	}
	adopted := root.node.Children[1]
	component, err := HydrateComponent(world, owner, definition, struct{}{}, struct{}{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	instance, _ := ecs.Get[htmlComponentInstance](world, component)
	if len(instance.Roots) != 1 || instance.Roots[0] != adopted {
		t.Fatalf("hydrated roots = %#v, want %p", instance.Roots, adopted)
	}
	if got := htmlNodeText(instance.Refs["label"]); got != "client" {
		t.Fatalf("hydrated label = %q", got)
	}
	serialized, err := Serialize(world, owner)
	if err != nil {
		t.Fatal(err)
	}
	want := `<main></main><section data-ref="root"><span data-ref="label">client</span></section>`
	if serialized != want {
		t.Fatalf("hydrated HTML = %q, want %q", serialized, want)
	}
}

func TestHydrateHTMLComponentIntoSlot(t *testing.T) {
	parentDefinition, err := Define[struct{}, struct{}](
		NewTemplate(`<section data-ref="slot"></section>`), "", Behavior[struct{}, struct{}]{},
	)
	if err != nil {
		t.Fatal(err)
	}
	childDefinition, err := Define[struct{}, struct{}](
		NewTemplate(`<span data-ref="label"></span>`), "",
		Behavior[struct{}, struct{}]{
			Render: func(_ RenderContext[struct{}, struct{}], view *View) {
				view.Text("label", "client")
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<section data-ref="slot"><span data-ref="label">server</span></section>`)
	page.Width, page.Height = 240, 120
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	parent, err := HydrateComponent(world, owner, parentDefinition, struct{}{}, struct{}{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	parentInstance, _ := ecs.Get[htmlComponentInstance](world, parent)
	adopted := parentInstance.Refs["slot"].Children[0]
	child, err := HydrateComponentInto(world, parent, "slot", childDefinition, struct{}{}, struct{}{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	childInstance, _ := ecs.Get[htmlComponentInstance](world, child)
	if len(childInstance.Roots) != 1 || childInstance.Roots[0] != adopted {
		t.Fatalf("hydrated slot root = %#v, want %p", childInstance.Roots, adopted)
	}
	if got := htmlNodeText(childInstance.Refs["label"]); got != "client" {
		t.Fatalf("hydrated slot label = %q", got)
	}
}

func TestHydrateHTMLComponentRejectsMissingOrAmbiguousDOM(t *testing.T) {
	definition, err := Define[struct{}, struct{}](
		NewTemplate(`<article data-ref="root"></article>`), "", Behavior[struct{}, struct{}]{},
	)
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main><article data-ref="root"></article><article data-ref="root"></article>`)
	page.Width, page.Height = 240, 120
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	if _, err := HydrateComponent(world, owner, definition, struct{}{}, struct{}{}, 1); err == nil {
		t.Fatal("ambiguous hydration unexpectedly succeeded")
	}
	if world.EntityCount() != 1 {
		t.Fatalf("ambiguous hydration spawned an entity: count=%d", world.EntityCount())
	}
	missing, err := Define[struct{}, struct{}](
		NewTemplate(`<aside data-ref="root"></aside>`), "", Behavior[struct{}, struct{}]{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := HydrateComponent(world, owner, missing, struct{}{}, struct{}{}, 2); err == nil {
		t.Fatal("missing hydration unexpectedly succeeded")
	}
}

func TestHTMLComponentReceivesFocusAndBlurEvents(t *testing.T) {
	calls := make([]EventKind, 0, 2)
	definition, err := Define[struct{}, struct{}](
		NewTemplate(`<input data-ref="field" id="field">`), "",
		Behavior[struct{}, struct{}]{
			Render: func(_ RenderContext[struct{}, struct{}], view *View) {
				view.Event("field", EventFocus)
				view.Event("field", EventBlur)
			},
			HandleEvent: func(context *ComponentEventContext[struct{}, struct{}]) {
				calls = append(calls, context.Event.Kind)
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main>`)
	page.Width, page.Height = 240, 120
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	component, err := MountComponent(world, owner, definition, struct{}{}, struct{}{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	state, _ := ecs.Get[htmlRenderState](world, owner)
	instance, _ := ecs.Get[htmlComponentInstance](world, component)
	control := state.EntityGroups[htmlRenderIdentity{Node: instance.Refs["field"], Role: htmlRoleControl}].Bindings[0].Entity
	if control == 0 {
		t.Fatal("focusable input control is missing")
	}
	DispatchAction(world, Event{Entity: owner, Target: control, Kind: EventFocus, Bubbles: false})
	DispatchAction(world, Event{Entity: owner, Target: control, Kind: EventBlur, Bubbles: false})
	if len(calls) != 2 || calls[0] != EventFocus || calls[1] != EventBlur {
		t.Fatalf("focus/blur calls = %#v", calls)
	}
}

func TestHTMLComponentKeepsUpdatesRequestedDuringRender(t *testing.T) {
	type state struct{ Count int }
	rendered := 0
	definition, err := Define[struct{}, state](
		NewTemplate(`<span data-ref="value"></span>`), "",
		Behavior[struct{}, state]{
			Render: func(context RenderContext[struct{}, state], view *View) {
				rendered++
				view.Text("value", strconv.Itoa(context.State.Count))
				if rendered == 1 && !UpdateComponentState[struct{}, state](context.World, context.Component, func(value *state) {
					value.Count = 1
				}) {
					t.Fatalf("render-time state update failed")
				}
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main>`)
	page.Width, page.Height = 200, 100
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	component, err := MountComponent(world, owner, definition, struct{}{}, state{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	instance, _ := ecs.Get[htmlComponentInstance](world, component)
	if rendered != 1 || htmlNodeText(instance.Refs["value"]) != "0" {
		t.Fatalf("first render count=%d value=%q", rendered, htmlNodeText(instance.Refs["value"]))
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	if rendered != 2 || htmlNodeText(instance.Refs["value"]) != "1" {
		t.Fatalf("second render count=%d value=%q", rendered, htmlNodeText(instance.Refs["value"]))
	}
}

func TestHTMLComponentCoalescesUpdatesBeforeSync(t *testing.T) {
	type state struct{ Count int }
	rendered := 0
	definition, err := Define[struct{}, state](
		NewTemplate(`<span data-ref="value"></span>`), "",
		Behavior[struct{}, state]{
			Render: func(context RenderContext[struct{}, state], view *View) {
				rendered++
				view.Text("value", strconv.Itoa(context.State.Count))
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main>`)
	page.Width, page.Height = 200, 100
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	component, err := MountComponent(world, owner, definition, struct{}{}, state{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	if rendered != 1 {
		t.Fatalf("initial render count=%d", rendered)
	}
	for count := 1; count <= 3; count++ {
		if !UpdateComponentState[struct{}, state](world, component, func(value *state) {
			value.Count++
		}) {
			t.Fatalf("state update %d failed", count)
		}
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	instance, _ := ecs.Get[htmlComponentInstance](world, component)
	if rendered != 2 || htmlNodeText(instance.Refs["value"]) != "3" {
		t.Fatalf("coalesced render count=%d value=%q", rendered, htmlNodeText(instance.Refs["value"]))
	}
}

func BenchmarkHTMLComponentStateSync(b *testing.B) {
	type state struct{ Count int }
	definition, err := Define[struct{}, state](
		NewTemplate(`<section data-ref="root"><span data-ref="value"></span></section>`), "",
		Behavior[struct{}, state]{
			Render: func(context RenderContext[struct{}, state], view *View) {
				view.Text("value", strconv.Itoa(context.State.Count))
			},
		},
	)
	if err != nil {
		b.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main>`)
	page.Width, page.Height = 240, 120
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	component, err := MountComponent(world, owner, definition, struct{}{}, state{}, 1)
	if err != nil {
		b.Fatal(err)
	}
	if err := Sync(world); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if !UpdateComponentState[struct{}, state](world, component, func(value *state) {
			value.Count++
		}) {
			b.Fatal("state update failed")
		}
		if err := Sync(world); err != nil {
			b.Fatal(err)
		}
	}
}

func TestDefineScopedHTMLScopesTemplateAndCSS(t *testing.T) {
	first, err := DefineScoped[struct{}, struct{}](
		NewTemplate(`<button data-ref="button" class="button">one</button>`),
		`.button { color: red; }`, Behavior[struct{}, struct{}]{},
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := DefineScoped[struct{}, struct{}](
		NewTemplate(`<button data-ref="button" class="button">two</button>`),
		`.button { color: blue; }`, Behavior[struct{}, struct{}]{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.runtime.scopeClass == "" || first.runtime.scopeClass == second.runtime.scopeClass {
		t.Fatalf("scope classes = %q and %q", first.runtime.scopeClass, second.runtime.scopeClass)
	}
	if !strings.Contains(first.CSS, ".button."+first.runtime.scopeClass) || !strings.Contains(second.CSS, ".button."+second.runtime.scopeClass) {
		t.Fatalf("scoped CSS = %q, %q", first.CSS, second.CSS)
	}

	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main>`)
	page.Width, page.Height = 240, 120
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	firstEntity, err := MountComponent(world, owner, first, struct{}{}, struct{}{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	secondEntity, err := MountComponent(world, owner, second, struct{}{}, struct{}{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	firstInstance, _ := ecs.Get[htmlComponentInstance](world, firstEntity)
	secondInstance, _ := ecs.Get[htmlComponentInstance](world, secondEntity)
	firstClass, _ := firstInstance.Roots[0].Attr("class")
	secondClass, _ := secondInstance.Roots[0].Attr("class")
	if !htmlClassContains(firstClass, first.runtime.scopeClass) || htmlClassContains(firstClass, second.runtime.scopeClass) {
		t.Fatalf("first class = %q", firstClass)
	}
	if !htmlClassContains(secondClass, second.runtime.scopeClass) || htmlClassContains(secondClass, first.runtime.scopeClass) {
		t.Fatalf("second class = %q", secondClass)
	}
}

func TestScopedHTMLPreservesMediaConditions(t *testing.T) {
	definition, err := DefineScoped[struct{}, struct{}](
		NewTemplate(`<section data-ref="card" class="card">card</section>`),
		`@media (min-width: 100px) { .card { color: red; } }`, Behavior[struct{}, struct{}]{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(definition.CSS, "@media (min-width: 100px)") || !strings.Contains(definition.CSS, ".card."+definition.runtime.scopeClass) {
		t.Fatalf("scoped media CSS = %q", definition.CSS)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main>`)
	page.Width, page.Height = 240, 120
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	if _, err := MountComponent(world, owner, definition, struct{}{}, struct{}{}, 1); err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
}

func TestHTMLComponentAttributeLastWriteWinsAcrossRenders(t *testing.T) {
	definition, err := Define[struct{}, bool](
		NewTemplate(`<fieldset data-ref="content"><div data-ref="dialog" hidden>dialog</div></fieldset>`), "",
		Behavior[struct{}, bool]{Render: func(context RenderContext[struct{}, bool], view *View) {
			view.Attr("dialog", "hidden", "")
			view.RemoveAttr("content", "disabled")
			if *context.State {
				view.RemoveAttr("dialog", "hidden")
				view.Attr("content", "disabled", "")
			}
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main>`)
	page.Width, page.Height = 240, 120
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	component, err := MountComponent(world, owner, definition, struct{}{}, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, open := range []bool{false, true, true, false, false, true, false} {
		UpdateComponentState[struct{}, bool](world, component, func(value *bool) { *value = open })
		if err := Sync(world); err != nil {
			t.Fatal(err)
		}
		instance, _ := ecs.Get[htmlComponentInstance](world, component)
		_, hidden := instance.Refs["dialog"].Attr("hidden")
		_, disabled := instance.Refs["content"].Attr("disabled")
		if hidden == open || disabled != open {
			t.Fatalf("open=%v: hidden=%v disabled=%v", open, hidden, disabled)
		}
	}
}

func TestScopedHTMLDynamicClassKeepsScope(t *testing.T) {
	type state struct{ Active bool }
	definition, err := DefineScoped[struct{}, state](
		NewTemplate(`<div data-ref="target"></div>`), `.active { color: red; }`,
		Behavior[struct{}, state]{
			Render: func(context RenderContext[struct{}, state], view *View) {
				if context.State.Active {
					view.Attr("target", "class", "active")
				} else {
					view.RemoveAttr("target", "class")
				}
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main>`)
	page.Width, page.Height = 240, 120
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	component, err := MountComponent(world, owner, definition, struct{}{}, state{Active: true}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	instance, _ := ecs.Get[htmlComponentInstance](world, component)
	class, _ := instance.Refs["target"].Attr("class")
	if !htmlClassContains(class, "active") || !htmlClassContains(class, definition.runtime.scopeClass) {
		t.Fatalf("dynamic class = %q", class)
	}
	if !UpdateComponentState[struct{}, state](world, component, func(value *state) { value.Active = false }) {
		t.Fatal("state update failed")
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	class, _ = instance.Refs["target"].Attr("class")
	if class != definition.runtime.scopeClass {
		t.Fatalf("class after RemoveAttr = %q, want %q", class, definition.runtime.scopeClass)
	}
}

func TestScopedHTMLKeyedVNodeKeepsScope(t *testing.T) {
	definition, err := DefineScoped[struct{}, struct{}](
		NewTemplate(`<ul data-ref="list"></ul>`), `.item { color: red; }`,
		Behavior[struct{}, struct{}]{
			Render: func(_ RenderContext[struct{}, struct{}], view *View) {
				view.Keyed("list", []VNode{VNodeElement(1, "li", []VNodeAttribute{{Name: "class", Value: "item"}}, []VNode{TextVNode(2, "span", "text")})})
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main>`)
	page.Width, page.Height = 240, 120
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	component, err := MountComponent(world, owner, definition, struct{}{}, struct{}{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	instance, _ := ecs.Get[htmlComponentInstance](world, component)
	item := instance.Refs["list"].Children[0]
	child := item.Children[0]
	itemClass, _ := item.Attr("class")
	childClass, _ := child.Attr("class")
	if !htmlClassContains(itemClass, definition.runtime.scopeClass) || !htmlClassContains(childClass, definition.runtime.scopeClass) {
		t.Fatalf("keyed classes = %q, %q", itemClass, childClass)
	}
}

func TestScopedHTMLUnmountRemovesCSSAfterLastInstance(t *testing.T) {
	definition, err := DefineScoped[struct{}, struct{}](
		NewTemplate(`<div data-ref="root"></div>`), `.root { color: red; }`, Behavior[struct{}, struct{}]{},
	)
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main>`)
	page.Width, page.Height = 240, 120
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	first, err := MountComponent(world, owner, definition, struct{}{}, struct{}{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MountComponent(world, owner, definition, struct{}{}, struct{}{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	pageValue, _ := ecs.Get[Surface](world, owner)
	if !strings.Contains(pageValue.CSS, definition.CSS) {
		t.Fatalf("page CSS after mount = %q", pageValue.CSS)
	}
	if !UnmountComponent(world, first) {
		t.Fatal("first unmount failed")
	}
	pageValue, _ = ecs.Get[Surface](world, owner)
	if !strings.Contains(pageValue.CSS, definition.CSS) {
		t.Fatalf("page CSS after first unmount = %q", pageValue.CSS)
	}
	if !UnmountComponent(world, second) {
		t.Fatal("second unmount failed")
	}
	pageValue, _ = ecs.Get[Surface](world, owner)
	if strings.Contains(pageValue.CSS, definition.CSS) {
		t.Fatalf("page CSS after last unmount = %q", pageValue.CSS)
	}
}

type htmlComponentCounterState struct {
	Count int
}

type htmlComponentListEntry struct {
	Key  uint64
	Text string
}

type htmlComponentListState struct {
	Items []htmlComponentListEntry
}

func TestHTMLComponentRendersAndPatchesDynamicValues(t *testing.T) {
	definition, err := Define[htmlComponentCounterProps, htmlComponentCounterState](
		NewTemplate(`<section data-ref="root"><button data-ref="add">add</button><span data-ref="count"></span></section>`),
		`section { color: red; }`,
		Behavior[htmlComponentCounterProps, htmlComponentCounterState]{
			Render: func(context RenderContext[htmlComponentCounterProps, htmlComponentCounterState], view *View) {
				view.Text("count", strconv.Itoa(context.State.Count))
				view.Attr("add", "title", strconv.Itoa(context.State.Count))
				view.Event("add", EventClick)
			},
			HandleEvent: func(context *ComponentEventContext[htmlComponentCounterProps, htmlComponentCounterState]) {
				context.UpdateState(func(state *htmlComponentCounterState) {
					state.Count += context.Props.Step
				})
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	world := ecs.NewWorld()
	pageBundle := ecs.NewBundle(1)
	page := New(`<main id="app"></main>`)
	page.Width, page.Height = 320, 180
	ecs.Put(&pageBundle, page)
	owner := world.Spawn(pageBundle)
	component, err := MountComponent(world, owner, definition, htmlComponentCounterProps{Step: 2}, htmlComponentCounterState{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}

	instance, ok := ecs.Get[htmlComponentInstance](world, component)
	if !ok || instance.Refs["count"] == nil {
		t.Fatal("component refs were not mounted")
	}
	if got := htmlNodeText(instance.Refs["count"]); got != "0" {
		t.Fatalf("initial count = %q, want 0", got)
	}
	if got, ok := instance.Refs["add"].Attr("title"); !ok || got != "0" {
		t.Fatalf("initial title = %q, present=%v", got, ok)
	}
	button := instance.Refs["add"]
	if button == nil {
		t.Fatal("button ref was not mounted")
	}
	state, ok := ecs.Get[htmlRenderState](world, owner)
	if !ok || state.EventTargets[button] == 0 {
		t.Fatal("component event target was not generated")
	}
	context := scene.InputEventContext{
		Event:  &scene.InputEvent{Entity: state.EventTargets[button], Kind: scene.InputEventTap},
		Target: state.EventTargets[button], Current: state.EventTargets[button],
	}
	dispatchHTMLNodePointer(world, owner, button, &context)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	if got := htmlNodeText(instance.Refs["count"]); got != "2" {
		t.Fatalf("patched count = %q, want 2", got)
	}
	if got, ok := instance.Refs["add"].Attr("title"); !ok || got != "2" {
		t.Fatalf("patched title = %q, present=%v", got, ok)
	}

	updated, ok := ecs.Get[Component[htmlComponentCounterProps, htmlComponentCounterState]](world, component)
	if !ok || updated.State.Count != 2 {
		t.Fatalf("component state = %#v, want count 2", updated.State)
	}
	pageValue, ok := ecs.Get[Surface](world, owner)
	if !ok || pageValue.CSS == "" {
		t.Fatal("component CSS was not merged into the page")
	}
}

func TestHTMLComponentUnmountRemovesRootsAndBindings(t *testing.T) {
	definition, err := Define[struct{}, struct{}](
		NewTemplate(`<div data-ref="root">content</div>`),
		"",
		Behavior[struct{}, struct{}]{},
	)
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main>`)
	page.Width, page.Height = 200, 100
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	component, err := MountComponent(world, owner, definition, struct{}{}, struct{}{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	if !UnmountComponent(world, component) {
		t.Fatal("unmount failed")
	}
	if world.IsAlive(component) {
		t.Fatal("component entity is still alive")
	}
	root, err := Root(world, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(root.node.Children) != 1 || root.node.Children[0].Data != "main" || len(root.node.Children[0].Children) != 0 {
		t.Fatalf("page DOM after unmount = %#v", root.node.Children)
	}
}

func TestHTMLComponentMountsIntoParentSlot(t *testing.T) {
	parentDefinition, err := Define[struct{}, struct{}](
		NewTemplate(`<section data-ref="content"></section>`),
		"",
		Behavior[struct{}, struct{}]{},
	)
	if err != nil {
		t.Fatal(err)
	}
	childDefinition, err := Define[struct{}, struct{}](
		NewTemplate(`<span data-ref="label"></span>`),
		"",
		Behavior[struct{}, struct{}]{
			Render: func(_ RenderContext[struct{}, struct{}], view *View) {
				view.Text("label", "child")
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main>`)
	page.Width, page.Height = 200, 100
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	parent, err := MountComponent(world, owner, parentDefinition, struct{}{}, struct{}{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	child, err := MountComponentInto(world, parent, "content", childDefinition, struct{}{}, struct{}{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	parentInstance, _ := ecs.Get[htmlComponentInstance](world, parent)
	childInstance, _ := ecs.Get[htmlComponentInstance](world, child)
	if len(parentInstance.Children) != 1 || parentInstance.Children[0] != child {
		t.Fatalf("parent children = %#v", parentInstance.Children)
	}
	if len(childInstance.Roots) != 1 || childInstance.Roots[0].Parent != parentInstance.Refs["content"] {
		t.Fatal("child root is not attached to parent slot")
	}
	if got := htmlNodeText(childInstance.Refs["label"]); got != "child" {
		t.Fatalf("child label = %q", got)
	}
	if !UnmountComponent(world, parent) {
		t.Fatal("parent unmount failed")
	}
	if world.IsAlive(child) || world.IsAlive(parent) {
		t.Fatal("parent unmount did not remove child and parent entities")
	}
}

func TestHTMLComponentKeyedNodesReuseDOMIdentity(t *testing.T) {
	definition, err := Define[struct{}, htmlComponentListState](
		NewTemplate(`<ul data-ref="list"></ul>`),
		"",
		Behavior[struct{}, htmlComponentListState]{
			Render: func(context RenderContext[struct{}, htmlComponentListState], view *View) {
				nodes := make([]VNode, 0, len(context.State.Items))
				for _, item := range context.State.Items {
					nodes = append(nodes, TextVNode(item.Key, "li", item.Text))
				}
				view.Keyed("list", nodes)
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main>`)
	page.Width, page.Height = 240, 120
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	component, err := MountComponent(world, owner, definition, struct{}{}, htmlComponentListState{
		Items: []htmlComponentListEntry{{1, "one"}, {2, "two"}, {3, "three"}},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	instance, _ := ecs.Get[htmlComponentInstance](world, component)
	container := instance.Refs["list"]
	if container == nil || len(container.Children) != 3 {
		t.Fatalf("initial list children = %d", len(container.Children))
	}
	first := append([]*dom.Node(nil), container.Children...)
	if got := htmlNodeText(first[1]); got != "two" {
		t.Fatalf("initial second item = %q", got)
	}
	if !UpdateComponentState[struct{}, htmlComponentListState](world, component, func(state *htmlComponentListState) {
		state.Items = []htmlComponentListEntry{{3, "three updated"}, {1, "one"}, {4, "four"}}
	}) {
		t.Fatal("state update failed")
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	if len(container.Children) != 3 || container.Children[0] != first[2] || container.Children[1] != first[0] {
		t.Fatal("keyed nodes were not moved and reused")
	}
	if got := htmlNodeText(container.Children[0]); got != "three updated" {
		t.Fatalf("updated first item = %q", got)
	}
	if first[1].Parent != nil {
		t.Fatal("removed keyed node is still attached")
	}
}

func TestHTMLComponentKeyedVNodeChildrenReuseDOMIdentity(t *testing.T) {
	type listState struct{ Items []htmlComponentListEntry }
	definition, err := Define[struct{}, listState](
		NewTemplate(`<ul data-ref="list"></ul>`),
		"",
		Behavior[struct{}, listState]{
			Render: func(context RenderContext[struct{}, listState], view *View) {
				nodes := make([]VNode, 0, len(context.State.Items))
				for _, item := range context.State.Items {
					nodes = append(nodes, VNodeElement(item.Key, "li", nil, []VNode{
						TextVNode(item.Key+100, "span", item.Text),
					}))
				}
				view.Keyed("list", nodes)
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main>`)
	page.Width, page.Height = 240, 120
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	component, err := MountComponent(world, owner, definition, struct{}{}, listState{
		Items: []htmlComponentListEntry{{1, "one"}, {2, "two"}},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	instance, _ := ecs.Get[htmlComponentInstance](world, component)
	container := instance.Refs["list"]
	first := append([]*dom.Node(nil), container.Children...)
	spans := append([]*dom.Node(nil), first[0].Children...)
	if len(first) != 2 || len(spans) != 1 || htmlNodeText(spans[0]) != "one" {
		t.Fatalf("initial nested VNodes = %#v", container.Children)
	}

	if !UpdateComponentState[struct{}, listState](world, component, func(state *listState) {
		state.Items = []htmlComponentListEntry{{2, "two updated"}, {1, "one"}, {3, "three"}}
	}) {
		t.Fatal("state update failed")
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	if len(container.Children) != 3 || container.Children[0] != first[1] || container.Children[1] != first[0] {
		t.Fatal("nested keyed elements were not reused")
	}
	if container.Children[1].Children[0] != spans[0] || htmlNodeText(container.Children[0].Children[0]) != "two updated" {
		t.Fatal("nested keyed children were not reused or patched")
	}
}

func TestHTMLComponentKeyedVNodeRemovalReleasesNestedState(t *testing.T) {
	type listState struct{ Items []htmlComponentListEntry }
	definition, err := Define[struct{}, listState](
		NewTemplate(`<ul data-ref="list"></ul>`), "",
		Behavior[struct{}, listState]{
			Render: func(context RenderContext[struct{}, listState], view *View) {
				nodes := make([]VNode, 0, len(context.State.Items))
				for _, item := range context.State.Items {
					nodes = append(nodes, VNodeElement(item.Key, "li", nil, []VNode{
						TextVNode(item.Key+100, "span", item.Text),
					}))
				}
				view.Keyed("list", nodes)
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main>`)
	page.Width, page.Height = 240, 120
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	component, err := MountComponent(world, owner, definition, struct{}{}, listState{
		Items: []htmlComponentListEntry{{1, strings.Repeat("old", 1024)}, {2, "keep"}},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	instance, _ := ecs.Get[htmlComponentInstance](world, component)
	list := instance.Keyed["list"]
	parent := instance.Refs["list"]
	if len(list.Items) != 2 {
		t.Fatalf("initial keyed items = %d", len(list.Items))
	}
	removed := list.Items[0].Node
	if !UpdateComponentState[struct{}, listState](world, component, func(state *listState) {
		state.Items = []htmlComponentListEntry{{2, "keep"}}
	}) {
		t.Fatal("state update failed")
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	if len(list.Children) != 1 || list.Children[instance.Refs["list"].Children[0]] == nil {
		t.Fatalf("removed keyed child state retained: %d entries", len(list.Children))
	}
	if list.Nodes[removed] {
		t.Fatal("removed keyed DOM node retained in list index")
	}
	for _, child := range parent.Children[:cap(parent.Children)] {
		if child == removed {
			t.Fatal("removed keyed DOM node retained by parent children backing storage")
		}
	}
}

func TestHTMLComponentVNodeStateResetClearsCapacity(t *testing.T) {
	state := newHTMLComponentVNodeState()
	state.Keyed["code"] = make([]VNode, 1, 2)
	state.Keyed["code"][0] = TextVNode(1, "span", strings.Repeat("source", 1024))
	state.Keyed["code"] = state.Keyed["code"][:0]
	state.ComponentChildren["projects"] = make([]ComponentVNode, 1, 2)
	state.ComponentChildren["projects"] = state.ComponentChildren["projects"][:0]
	state.reset()
	if retained := state.Keyed["code"][:cap(state.Keyed["code"])]; retained[0].Text != "" {
		t.Fatal("keyed VNode backing storage retained text")
	}
	if retained := state.ComponentChildren["projects"][:cap(state.ComponentChildren["projects"])]; retained[0].values != nil {
		t.Fatal("component VNode backing storage retained child values")
	}
}

func TestHTMLComponentKeyedTextUpdatesDoNotAccumulateRenderEntities(t *testing.T) {
	type listState struct{ Revision int }
	definition, err := Define[struct{}, listState](
		NewTemplate(`<pre data-ref="list"></pre>`), "",
		Behavior[struct{}, listState]{
			Render: func(context RenderContext[struct{}, listState], view *View) {
				nodes := make([]VNode, 0, 64)
				for index := uint64(0); index < 64; index++ {
					nodes = append(nodes, TextVNode(index+1, "span", fmt.Sprintf("revision-%d-%d", context.State.Revision, index)))
				}
				view.Keyed("list", nodes)
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main>`)
	page.Width, page.Height = 640, 480
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	component, err := MountComponent(world, owner, definition, struct{}{}, listState{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	initial := world.EntityCount()
	for revision := 1; revision <= 8; revision++ {
		if !UpdateComponentState[struct{}, listState](world, component, func(state *listState) { state.Revision = revision }) {
			t.Fatal("state update failed")
		}
		if err := Sync(world); err != nil {
			t.Fatal(err)
		}
		if got := world.EntityCount(); got > initial+8 {
			t.Fatalf("revision %d retained %d entities, initial %d", revision, got, initial)
		}
	}
}

func TestHTMLComponentEventsBubbleThroughDOMAncestors(t *testing.T) {
	calls := make([]string, 0, 2)
	definition, err := Define[struct{}, struct{}](
		NewTemplate(`<div data-ref="parent"><button data-ref="child">child</button></div>`),
		"",
		Behavior[struct{}, struct{}]{
			Render: func(_ RenderContext[struct{}, struct{}], view *View) {
				view.Event("parent", EventClick)
				view.Event("child", EventClick)
			},
			HandleEvent: func(context *ComponentEventContext[struct{}, struct{}]) {
				calls = append(calls, context.Ref)
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main>`)
	page.Width, page.Height = 240, 120
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	component, err := MountComponent(world, owner, definition, struct{}{}, struct{}{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	instance, _ := ecs.Get[htmlComponentInstance](world, component)
	state, _ := ecs.Get[htmlRenderState](world, owner)
	target := state.EventTargets[instance.Refs["child"]]
	if target == 0 {
		t.Fatal("child event target is missing")
	}
	context := scene.InputEventContext{Event: &scene.InputEvent{Entity: target, Kind: scene.InputEventTap}, Target: target, Current: target}
	dispatchHTMLNodePointer(world, owner, instance.Refs["child"], &context)
	if len(calls) != 2 || calls[0] != "child" || calls[1] != "parent" {
		t.Fatalf("bubbled calls = %#v", calls)
	}
}

func TestHTMLComponentReceivesNativeEventsAndBindsMultipleKinds(t *testing.T) {
	type eventState struct {
		Count int
		Value string
	}
	calls := make([]EventKind, 0, 2)
	definition, err := Define[struct{}, eventState](
		NewTemplate(`<input data-ref="field" id="field" value="initial">`),
		"",
		Behavior[struct{}, eventState]{
			Render: func(_ RenderContext[struct{}, eventState], view *View) {
				view.Event("field", EventInput)
				view.Event("field", EventChange)
			},
			HandleEvent: func(context *ComponentEventContext[struct{}, eventState]) {
				calls = append(calls, context.Event.Kind)
				context.UpdateState(func(state *eventState) {
					state.Count++
					state.Value = context.Event.Value
				})
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main>`)
	page.Width, page.Height = 240, 120
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	component, err := MountComponent(world, owner, definition, struct{}{}, eventState{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}

	state, _ := ecs.Get[htmlRenderState](world, owner)
	instance, _ := ecs.Get[htmlComponentInstance](world, component)
	control := state.EntityGroups[htmlRenderIdentity{Node: instance.Refs["field"], Role: htmlRoleControl}].Bindings[0].Entity
	if control == 0 {
		t.Fatal("native input control is missing")
	}
	ecs.WriteMessage(world, Event{Entity: owner, Target: control, ID: "field", Kind: EventInput, Value: "typed", Bubbles: true})
	ecs.WriteMessage(world, Event{Entity: owner, Target: control, ID: "field", Kind: EventChange, Value: "committed", Bubbles: true})
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}

	updated, _ := ecs.Get[Component[struct{}, eventState]](world, component)
	if updated.State.Count != 2 || updated.State.Value != "committed" {
		t.Fatalf("native event state = %#v, want two updates", updated.State)
	}
	if len(calls) != 2 || calls[0] != EventInput || calls[1] != EventChange {
		t.Fatalf("native event calls = %#v", calls)
	}
}

func TestHTMLComponentCancelableActionEventCanPreventDefault(t *testing.T) {
	called := false
	definition, err := Define[struct{}, struct{}](
		NewTemplate(`<input data-ref="field" id="field">`),
		"",
		Behavior[struct{}, struct{}]{
			Render: func(_ RenderContext[struct{}, struct{}], view *View) {
				view.Event("field", EventBeforeInput)
			},
			HandleEvent: func(context *ComponentEventContext[struct{}, struct{}]) {
				called = true
				context.PreventDefault()
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main>`)
	page.Width, page.Height = 240, 120
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	if _, err := MountComponent(world, owner, definition, struct{}{}, struct{}{}, 1); err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}

	event := DispatchAction(world, Event{
		Entity: owner, ID: "field", Kind: EventBeforeInput,
		Bubbles: true, Cancelable: true, InputType: "insertText", Data: "x",
	})
	if !called || !event.DefaultPrevented {
		t.Fatalf("beforeinput event called=%v prevented=%v", called, event.DefaultPrevented)
	}
}

func TestSerializeHTMLIncludesComposedComponentDOM(t *testing.T) {
	definition, err := Define[struct{}, struct{}](
		NewTemplate(`<section data-ref="root"><span data-ref="label"></span><input data-ref="field" disabled></section>`),
		"",
		Behavior[struct{}, struct{}]{
			Render: func(_ RenderContext[struct{}, struct{}], view *View) {
				view.Text("label", `<value &>`)
				view.Attr("field", "title", `quoted "value" & <tag>`)
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main id="app" title="a &amp; b"></main>`)
	page.Width, page.Height = 240, 120
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	if _, err := MountComponent(world, owner, definition, struct{}{}, struct{}{}, 1); err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}

	got, err := Serialize(world, owner)
	if err != nil {
		t.Fatal(err)
	}
	want := `<main id="app" title="a &amp; b"></main><section data-ref="root"><span data-ref="label">&lt;value &amp;&gt;</span><input data-ref="field" disabled="" title="quoted &quot;value&quot; &amp; &lt;tag&gt;"></section>`
	if got != want {
		t.Fatalf("serialized HTML = %q, want %q", got, want)
	}

	root, err := Root(world, owner)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := root.Serialize()
	if err != nil || fragment != want {
		t.Fatalf("serialized root = %q, err=%v", fragment, err)
	}
}

func TestHTMLComponentStylePatchUsesIncrementalRendering(t *testing.T) {
	type styleState struct{ Color string }
	definition, err := Define[struct{}, styleState](
		NewTemplate(`<section data-ref="root"><div data-ref="target">Target</div><div data-ref="sibling">Sibling</div></section>`),
		"",
		Behavior[struct{}, styleState]{
			Render: func(context RenderContext[struct{}, styleState], view *View) {
				view.Attr("target", "style", "color:"+context.State.Color)
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main>`)
	page.Width, page.Height, page.Font = 240, 120, notosanssc.NotoSansSCRegular
	ecs.Put(&bundle, page)
	owner := world.Spawn(bundle)
	component, err := MountComponent(world, owner, definition, struct{}{}, styleState{Color: "red"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	initial, _ := ecs.Get[htmlRenderState](world, owner)
	initialBoxes := initial.Boxes
	initialChildren := append([]ecs.Entity(nil), initial.Children...)
	instance, _ := ecs.Get[htmlComponentInstance](world, component)
	target := instance.Refs["target"]
	if target == nil {
		t.Fatal("style target is missing")
	}

	if !UpdateComponentState[struct{}, styleState](world, component, func(state *styleState) {
		state.Color = "blue"
	}) {
		t.Fatal("state update failed")
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	updated, _ := ecs.Get[htmlRenderState](world, owner)
	if updated.Boxes != initialBoxes || updated.SyncStats.Layouts != 0 || updated.SyncStats.Spawned != 0 || updated.SyncStats.Despawned != 0 {
		t.Fatalf("style patch rebuilt retained layout or entities: %+v", updated.SyncStats)
	}
	if updated.SyncStats.PaintUpdated == 0 || updated.SyncStats.Styled == 0 || updated.SyncStats.Styled > 2 {
		t.Fatalf("style patch did not stay local: %+v", updated.SyncStats)
	}
	if len(updated.Children) != len(initialChildren) {
		t.Fatalf("child count changed from %d to %d", len(initialChildren), len(updated.Children))
	}
	for index, child := range initialChildren {
		if updated.Children[index] != child {
			t.Fatalf("child %d identity changed", index)
		}
	}
	if value, ok := target.Attr("style"); !ok || value != "color:blue" {
		t.Fatalf("style attribute = %q, present=%v", value, ok)
	}
}

func TestHTMLComponentUnmountManyRootsRetainsStaticDOMAndInvalidatesFilters(t *testing.T) {
	definition, err := Define[struct{}, struct{}](
		NewTemplate(strings.Repeat(`<span>item</span>`, 128)+`<svg><filter id="obsolete"></filter></svg>`),
		"", Behavior[struct{}, struct{}]{},
	)
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	owners := ecs.NewSpawner1[Surface](world)
	owner := owners.Spawn(New(`<main>Keep</main>`))
	component, err := MountComponent(world, owner, definition, struct{}{}, struct{}{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	instance, _ := ecs.Get[htmlComponentInstance](world, component)
	roots := append([]*dom.Node(nil), instance.Roots...)
	root, err := Root(world, owner)
	if err != nil {
		t.Fatal(err)
	}
	if !UnmountComponent(world, component) {
		t.Fatal("unmount failed")
	}
	if len(root.node.Children) != 1 || root.node.Children[0].Data != "main" {
		t.Fatal("bulk root removal changed static DOM")
	}
	for _, node := range roots {
		if node.Parent != nil {
			t.Fatal("unmounted root retained its parent")
		}
	}
	state, _ := ecs.Get[htmlRenderState](world, owner)
	if state.Mutations[root.node]&htmlMutationDefinitions == 0 {
		t.Fatal("removing an SVG filter did not invalidate document definitions")
	}
}
