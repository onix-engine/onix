package module

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestModuleHTMLSelectionIsPrivateAndExpiresWithDocument(t *testing.T) {
	plan, source := compileFixture(t, map[string]string{
		"resources/scenes/main.json": `{"modules":[{"id":"a","mod":"page"},{"id":"b","mod":"page"}]}`,
		"page/mod.json":              `{"components":{"size":[200,100],"html":{"resource":"page"}},"resources":{"page":{"kind":"html","path":"index.html"}}}`,
		"page/index.html":            `<select id="mode"><option>A</option><option>B</option></select>`,
	})
	r := newFixture(t, plan, Registry{}, Options{Source: source})
	s, err := r.Manager().StartEntry()
	must(t, err)
	a, b := &s.modules[0], &s.modules[1]
	doc, err := HTML(a, a.Root())
	must(t, err)
	node, ok := doc.FindByID("mode")
	if !ok || !node.SetSelectedIndex(1) {
		t.Fatal("owned select could not be changed")
	}
	must(t, htmlui.Sync(r.world))
	controlA, _ := htmlui.ControlByID(r.world, a.root, "mode")
	controlB, _ := htmlui.ControlByID(r.world, b.root, "mode")
	valueA, _ := ecs.Get[htmlui.Control](r.world, controlA)
	valueB, _ := ecs.Get[htmlui.Control](r.world, controlB)
	if valueA.Selected != 1 || valueB.Selected != 0 {
		t.Fatal("selection leaked between instances", valueA.Selected, valueB.Selected)
	}
	surface, _ := Get[htmlui.Surface](a, a.Root())
	surface.Value = `<select id="mode"><option>New</option></select>`
	must(t, Set(a, a.Root(), surface))
	if node.SetSelectedIndex(0) {
		t.Fatal("old document handle remained valid")
	}
	doc, err = HTML(b, b.Root())
	must(t, err)
	node, _ = doc.FindByID("mode")
	must(t, b.Dispose())
	if node.SetSelectedIndex(1) {
		t.Fatal("disposed handle remained valid")
	}
}
