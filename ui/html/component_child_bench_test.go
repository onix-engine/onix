package html

import "testing"

var componentChildBenchmarkSink ComponentVNode

func BenchmarkHTMLComponentChildSnapshot(b *testing.B) {
	definition, err := Define[htmlComponentChildProps, struct{}](NewTemplate(`<span></span>`), "", Behavior[htmlComponentChildProps, struct{}]{})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		componentChildBenchmarkSink = definition.ComponentChild(1, htmlComponentChildProps{"file.go"}, struct{}{})
	}
}

func BenchmarkHTMLComponentChildSlot(b *testing.B) {
	definition, err := Define[htmlComponentChildProps, struct{}](NewTemplate(`<span></span>`), "", Behavior[htmlComponentChildProps, struct{}]{})
	if err != nil {
		b.Fatal(err)
	}
	template, err := compileHTMLComponentTemplate(`<div data-ref="slot"></div>`)
	if err != nil {
		b.Fatal(err)
	}
	instance := &htmlComponentInstance{Current: newHTMLComponentVNodeState(), Next: newHTMLComponentVNodeState()}
	view := &View{instance: instance, template: template}
	render := func() {
		view.reset()
		for index := uint64(0); index < 100; index++ {
			view.AppendComponentChild("slot", index, definition, htmlComponentChildProps{"file.go"}, struct{}{})
		}
		instance.Current, instance.Next = instance.Next, instance.Current
		instance.Next.reset()
	}
	render()
	render()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		render()
	}
}
