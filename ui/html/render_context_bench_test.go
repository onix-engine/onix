package html

import (
	"testing"

	"github.com/diiyw/onix/ecs"
)

func BenchmarkHTMLRenderContext(b *testing.B) {
	definition, err := Define[int, int](NewTemplate(`<span></span>`), "", Behavior[int, int]{Render: func(c RenderContext[int, int], _ *View) { *c.State = *c.Props }})
	if err != nil {
		b.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, New(`<main></main>`))
	owner := world.Spawn(bundle)
	entity, err := MountComponent(world, owner, definition, 42, 0, 1)
	if err != nil {
		b.Fatal(err)
	}
	if err := Sync(world); err != nil {
		b.Fatal(err)
	}
	instance := ecs.GetMut[htmlComponentInstance](world, entity)
	definition.runtime.render(world, entity, instance)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		definition.runtime.render(world, entity, instance)
	}
}
