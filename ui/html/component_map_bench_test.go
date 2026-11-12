package html

import (
	"testing"

	"github.com/onix-engine/onix/ecs"
)

func BenchmarkHTMLComponentMountStorage(b *testing.B) {
	for _, bound := range []bool{false, true} {
		name, source := "Static", `<span>static</span>`
		behavior := Behavior[struct{}, struct{}]{}
		if bound {
			name, source = "Text", `<span data-ref="label"></span>`
			behavior.Render = func(_ RenderContext[struct{}, struct{}], view *View) { view.Text("label", "value") }
		}
		b.Run(name, func(b *testing.B) {
			definition, err := Define[struct{}, struct{}](NewTemplate(source), "", behavior)
			if err != nil {
				b.Fatal(err)
			}
			world := ecs.NewWorld()
			pages := world.NewSpawner1[Surface]()
			owner := pages.Spawn(New(`<main></main>`))
			run := func() {
				entity, err := definition.MountComponent(world, owner, struct{}{}, struct{}{}, 1)
				if err != nil {
					b.Fatal(err)
				}
				instance := world.GetMut[htmlComponentInstance](entity)
				definition.runtime.render(world, entity, instance)
				if !UnmountComponent(world, entity) {
					b.Fatal("unmount failed")
				}
				queue := world.FrameValue[htmlComponentDirtyTag, htmlComponentPending]()
				queue.Entities = queue.Entities[:0]
				clearHTMLMutations(world.GetMut[htmlRenderState](owner))
			}
			run()
			b.ReportAllocs()
			for b.Loop() {
				run()
			}
		})
	}
}

func BenchmarkHTMLComponentCSSReferences(b *testing.B) {
	world := ecs.NewWorld()
	pages := world.NewSpawner1[Surface]()
	owner := pages.Spawn(New(`<main></main>`))
	definition := &htmlDefinitionRuntime{}
	const cssText = `.component { color: red; }`
	if err := attachHTMLComponentCSS(world, owner, definition, cssText); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if err := attachHTMLComponentCSS(world, owner, definition, cssText); err != nil {
			b.Fatal(err)
		}
		if err := detachHTMLComponentCSS(world, owner, definition); err != nil {
			b.Fatal(err)
		}
	}
}
