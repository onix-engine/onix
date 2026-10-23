package module

import (
	"testing"

	html "github.com/diiyw/onix/ui/html"
)

func TestContentRenderContextValueReentry(t *testing.T) {
	content := &Content{}
	propsA, stateA, propsB, stateB := 11, 0, 22, 0
	first := html.RenderContext[int, int]{Component: 1, Props: &propsA, State: &stateA}
	second := html.RenderContext[int, int]{Component: 2, Props: &propsB, State: &stateB}
	view := &html.View{}
	var definition *html.Definition[int, int]
	var saved []ContentRenderContext[int, int]
	var err error
	definition, err = ContentDefine[int, int](content, html.NewTemplate(`<span></span>`), "", ContentBehavior[int, int]{Render: func(c ContentRenderContext[int, int], v *html.View) {
		saved = append(saved, c)
		if c.Component == 1 {
			definition.Behavior.Render(second, v)
		}
		*c.State = *c.Props
	}})
	if err != nil {
		t.Fatal(err)
	}
	definition.Behavior.Render(first, view)
	if len(saved) != 2 || saved[0].World != content || saved[1].World != content || saved[0].Component != 1 || saved[1].Component != 2 || saved[0].Props != &propsA || saved[1].State != &stateB || stateA != 11 || stateB != 22 {
		t.Fatal("module render bridge mixed nested contexts or lost state writes")
	}
}

func TestContentRenderBridgeWarmAllocations(t *testing.T) {
	definition, err := ContentDefine[int, int](&Content{}, html.NewTemplate(`<span></span>`), "", ContentBehavior[int, int]{Render: func(c ContentRenderContext[int, int], _ *html.View) { *c.State = *c.Props }})
	if err != nil {
		t.Fatal(err)
	}
	props, state := 42, 0
	context := html.RenderContext[int, int]{Props: &props, State: &state}
	view := &html.View{}
	if allocs := testing.AllocsPerRun(100, func() { definition.Behavior.Render(context, view) }); allocs != 0 {
		t.Fatalf("render bridge allocated %g times", allocs)
	}
	if state != 42 {
		t.Fatal("render state write lost")
	}
}
