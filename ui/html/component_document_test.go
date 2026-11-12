package html

import (
	"strings"
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/html/dom"
)

func TestHTMLComponentDocumentTemplateSharesCompilationAndClonesInstances(t *testing.T) {
	root, err := dom.Parse(`<style>p { color: red; }</style><p data-ref="label">original</p>`)
	if err != nil {
		t.Fatal(err)
	}
	template, err := NewDocumentTemplate(root)
	if err != nil {
		t.Fatal(err)
	}
	behavior := Behavior[string, struct{}]{Render: func(context RenderContext[string, struct{}], view *View) {
		view.Text("label", *context.Props)
	}}
	definition, err := DefineScoped(template, "", behavior)
	if err != nil {
		t.Fatal(err)
	}
	reused, err := DefineScoped(template, "", behavior)
	if err != nil {
		t.Fatal(err)
	}
	if definition.runtime.template.Root != root || reused.runtime.template != definition.runtime.template {
		t.Fatal("parsed template was copied or compiled again")
	}
	if len(root.Children) != 1 || root.Children[0].Data != "p" || !strings.Contains(definition.CSS, "color: red") || reused.CSS != definition.CSS {
		t.Fatalf("embedded CSS was lost or retained as content: CSS=%q, root=%s", definition.CSS, root)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	page := New(`<main></main>`)
	page.Width, page.Height = 240, 120
	bundle.Put(page)
	owner := world.Spawn(bundle)
	first, err := definition.MountComponent(world, owner, "first", struct{}{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := reused.MountComponent(world, owner, "second", struct{}{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	a, _ := world.Get[htmlComponentInstance](first)
	b, _ := world.Get[htmlComponentInstance](second)
	if a.Refs["label"] == b.Refs["label"] || a.Refs["label"] == root.Children[0] || htmlNodeText(a.Refs["label"]) != "first" || htmlNodeText(b.Refs["label"]) != "second" {
		t.Fatal("mounts shared mutable nodes or text")
	}
	class, _ := root.Children[0].Attr("class")
	if class != "" || htmlNodeText(root.Children[0]) != "original" {
		t.Fatal("rendering mutated the retained template")
	}
	class, _ = a.Refs["label"].Attr("class")
	if !htmlClassContains(class, definition.runtime.scopeClass) {
		t.Fatal("document template mount lost its CSS scope")
	}
}

func TestHTMLComponentDocumentTemplateScopeAndValidation(t *testing.T) {
	compile := func(source string) Template {
		t.Helper()
		root, err := dom.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		template, err := NewDocumentTemplate(root)
		if err != nil {
			t.Fatal(err)
		}
		return template
	}
	a := compile(`<p id="label" class="text">one</p>`)
	b := compile(`<p class="text" id="label">one</p>`)
	c := compile(`<p id="label" class="text">two</p>`)
	if htmlComponentScopeClass(a, "") != htmlComponentScopeClass(b, "") || htmlComponentScopeClass(a, "") == htmlComponentScopeClass(c, "") || htmlComponentScopeClass(a, "") == htmlComponentScopeClass(a, "p{color:red}") {
		t.Fatal("document scope must be stable across map order and distinguish content and CSS")
	}
	for _, root := range []*dom.Node{nil, dom.NewElement("p")} {
		if _, err := NewDocumentTemplate(root); err == nil {
			t.Fatal("accepted an invalid document root")
		}
	}
	root, err := dom.Parse(`<p data-ref="same"></p><p data-ref="same"></p>`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewDocumentTemplate(root); err == nil {
		t.Fatal("accepted duplicate component references")
	}
}
