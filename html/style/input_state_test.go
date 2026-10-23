package style

import (
	"reflect"
	"testing"

	"github.com/diiyw/onix/html/css"
	"github.com/diiyw/onix/html/dom"
)

func TestInputStateRecomputeMatchesFullCascade(t *testing.T) {
	doc, err := dom.Parse(`<html><body><section id="panel"><button id="first"><span>First</span></button><button id="second">Second</button></section><aside>Unchanged</aside></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`
 button{border:1px solid #111;color:#111}
 button:hover{color:#f00}
 button:active{width:180px}
 button:focus-visible{background:#123456}
 #panel:focus-within{color:#00f}
 #panel:hover #second{background:#abcdef}
 button:not(:hover){border-color:#222}
 button:is(:hover,:focus) span{color:#0f0}
 aside{box-shadow:0 18px 48px rgba(0,0,0,.38)}
 `)
	if err != nil {
		t.Fatal(err)
	}
	panel := doc.Children[0].Children[1].Children[0]
	first, second := panel.Children[0], panel.Children[1]
	states := []css.State{
		{}, {Hovered: first}, {Hovered: second}, {},
		{Focused: first, FocusVisible: true}, {Focused: first},
		{Focused: second, FocusVisible: true}, {},
		{Active: first}, {Active: second}, {},
	}
	viewport := Viewport{Width: 800, Height: 600}
	styles, err := ComputeWithViewportAndState(doc, stylesheet, viewport, &states[0])
	if err != nil {
		t.Fatal(err)
	}
	for index := 1; index < len(states); index++ {
		styles, _, err = RecomputeInputStateInPlaceWithImpact(doc, stylesheet, viewport, &states[index-1], &states[index], styles)
		if err != nil {
			t.Fatal(err)
		}
		want, err := ComputeWithViewportAndState(doc, stylesheet, viewport, &states[index])
		if err != nil {
			t.Fatal(err)
		}
		for node, expected := range want {
			if !reflect.DeepEqual(styles[node], expected) {
				t.Fatalf("transition %d: incremental style differs from full cascade for %s", index, node.Data)
			}
		}
	}
}

func TestInputStateRecomputeReusesUnchangedStyles(t *testing.T) {
	doc, err := dom.Parse(`<button>Hover</button><aside>Static shadow</aside>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`button:hover{color:#ff0000} aside{box-shadow:0 18px 48px rgba(0,0,0,.38)}`)
	if err != nil {
		t.Fatal(err)
	}
	states := [2]css.State{{}, {Hovered: doc.Children[0]}}
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	shadow := styles[doc.Children[1]].BoxShadow
	if len(shadow) == 0 {
		t.Fatal("missing static shadow")
	}
	update := func() {
		for i := 0; i < 2; i++ {
			if _, _, err := RecomputeInputStateInPlaceWithImpact(doc, stylesheet, Viewport{}, &states[i], &states[1-i], styles); err != nil {
				panic(err)
			}
		}
	}
	update()
	if &styles[doc.Children[1]].BoxShadow[0] != &shadow[0] {
		t.Fatal("unrelated shadow recomputed on hover")
	}
	// Hover restyles the button and the text inheriting its color, so interning
	// allocates one instance per distinct style in each toggle direction.
	if got := testing.AllocsPerRun(100, update); got > 4 {
		t.Fatalf("hover allocated %g objects, want one instance per changed style", got)
	}
}
