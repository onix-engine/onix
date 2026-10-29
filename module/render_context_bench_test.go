package module

import (
	"testing"

	html "github.com/diiyw/onix/ui/html"
)

func BenchmarkContentRenderBridge(b *testing.B) {
	content := &Content{}
	definition, err := content.Define[int, int](html.NewTemplate(`<span></span>`), "", ContentBehavior[int, int]{Render: func(c ContentRenderContext[int, int], _ *html.View) { *c.State = *c.Props }})
	if err != nil {
		b.Fatal(err)
	}
	props, state := 42, 0
	ctx := html.RenderContext[int, int]{Props: &props, State: &state}
	view := &html.View{}
	definition.Behavior.Render(ctx, view)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		definition.Behavior.Render(ctx, view)
	}
}
