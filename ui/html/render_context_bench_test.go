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
	bundle.Put(New(`<main></main>`))
	owner := world.Spawn(bundle)
	entity, err := definition.MountComponent(world, owner, 42, 0, 1)
	if err != nil {
		b.Fatal(err)
	}
	if err := Sync(world); err != nil {
		b.Fatal(err)
	}
	instance := world.GetMut[htmlComponentInstance](entity)
	definition.runtime.render(world, entity, instance)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		definition.runtime.render(world, entity, instance)
	}
}
