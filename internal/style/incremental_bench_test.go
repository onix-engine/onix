package style

import (
	"strconv"
	"strings"
	"testing"

	"github.com/diiyw/onix/internal/css"
	"github.com/diiyw/onix/internal/dom"
)

func BenchmarkComputeHoverStylesFull(b *testing.B) {
	doc, stylesheet, hovered := buildHoverBenchmarkFixture(b)
	viewport := Viewport{Width: 800, Height: 600}
	input := &css.State{Hovered: hovered}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ComputeWithViewportAndStateAndQueries(doc, stylesheet, viewport, input, nil); err != nil {
			b.Fatalf("compute styles: %v", err)
		}
	}
}

func BenchmarkComputeHoverStylesIncremental(b *testing.B) {
	doc, stylesheet, hovered := buildHoverBenchmarkFixture(b)
	viewport := Viewport{Width: 800, Height: 600}
	input := &css.State{Hovered: hovered}

	baseStyles, err := ComputeWithViewportAndStateAndQueries(doc, stylesheet, viewport, nil, nil)
	if err != nil {
		b.Fatalf("compute base styles: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := RecomputeSubtreesInPlaceWithViewportAndStateAndQueries(doc, stylesheet, viewport, input, nil, baseStyles, []*dom.Node{hovered}); err != nil {
			b.Fatalf("recompute styles: %v", err)
		}
	}
}

func BenchmarkComputeHoverStylesIncrementalInPlace(b *testing.B) {
	doc, stylesheet, hovered := buildHoverBenchmarkFixture(b)
	viewport := Viewport{Width: 800, Height: 600}
	input := &css.State{Hovered: hovered}

	baseStyles, err := ComputeWithViewportAndStateAndQueries(doc, stylesheet, viewport, nil, nil)
	if err != nil {
		b.Fatalf("compute base styles: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := RecomputeSubtreesInPlaceWithViewportAndStateAndQueries(doc, stylesheet, viewport, input, nil, baseStyles, []*dom.Node{hovered}); err != nil {
			b.Fatalf("recompute styles: %v", err)
		}
	}
}

func TestRecomputeHoverStylesInPlaceWarmPathDoesNotAllocate(t *testing.T) {
	doc, stylesheet, hovered := buildHoverBenchmarkFixture(t)
	viewport := Viewport{Width: 800, Height: 600}
	styles, err := ComputeWithViewportAndStateAndQueries(doc, stylesheet, viewport, nil, nil)
	if err != nil {
		t.Fatalf("compute base styles: %v", err)
	}
	input := &css.State{Hovered: hovered}
	roots := []*dom.Node{hovered}
	if _, err := RecomputeSubtreesInPlaceWithViewportAndStateAndQueries(doc, stylesheet, viewport, input, nil, styles, roots); err != nil {
		t.Fatalf("warm style scratch: %v", err)
	}

	allocations := testing.AllocsPerRun(100, func() {
		if _, err := RecomputeSubtreesInPlaceWithViewportAndStateAndQueries(doc, stylesheet, viewport, input, nil, styles, roots); err != nil {
			t.Fatalf("recompute styles: %v", err)
		}
	})
	if allocations != 0 {
		t.Fatalf("warm in-place style recompute allocated %.0f objects, want 0", allocations)
	}
}

func TestRecomputeFocusWithinInvalidatesAncestorBranch(t *testing.T) {
	doc, err := dom.Parse(`<div id="wrap"><input id="field"></div>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`div { background-color:#112233 } div:focus-within { background-color:#445566 }`)
	if err != nil {
		t.Fatal(err)
	}
	wrap := doc.Children[0]
	field := wrap.Children[0]
	styles, err := ComputeWithState(doc, stylesheet, nil)
	if err != nil {
		t.Fatal(err)
	}
	input := &css.State{Focused: field}
	styles, _, err = RecomputeSubtreesInPlaceWithImpact(doc, stylesheet, Viewport{}, input, nil, styles, []*dom.Node{doc})
	if err != nil {
		t.Fatal(err)
	}
	if got := styles[wrap].BackgroundColor.Color; got != (Color{R: 0x44, G: 0x55, B: 0x66, A: 255}) {
		t.Fatalf("focus-within ancestor color = %#v", got)
	}
}

func TestRecomputeReportsLayoutImpact(t *testing.T) {
	doc, stylesheet, hovered := buildHoverBenchmarkFixture(t)
	stylesheet, err := css.Parse(`.item { width:100px } .item:hover { width:200px }`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	styles, impact, err := RecomputeSubtreesInPlaceWithImpact(doc, stylesheet, Viewport{}, &css.State{Hovered: hovered}, nil, styles, []*dom.Node{doc})
	if err != nil {
		t.Fatal(err)
	}
	if styles[hovered].Width != LengthPx(200) || impact != ChangeRebuild {
		t.Fatalf("hover layout change = width:%#v impact:%v", styles[hovered].Width, impact)
	}
}

func TestRecomputeReportsPaintImpact(t *testing.T) {
	doc, err := dom.Parse(`<button id="button">Button</button>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`#button { background:#112233 } #button:hover { background:#445566 }`)
	if err != nil {
		t.Fatal(err)
	}
	button := doc.Children[0]
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	_, impact, err := RecomputeSubtreesInPlaceWithImpact(doc, stylesheet, Viewport{}, &css.State{Hovered: button}, nil, styles, []*dom.Node{doc})
	if err != nil {
		t.Fatal(err)
	}
	if impact != ChangePaint {
		t.Fatalf("hover paint impact = %v, want %v", impact, ChangePaint)
	}
}

func buildHoverBenchmarkFixture(tb testing.TB) (*dom.Node, *css.Stylesheet, *dom.Node) {
	tb.Helper()

	var html strings.Builder
	html.WriteString("<html><body>")
	for i := 0; i < 200; i++ {
		html.WriteString(`<a id="link`)
		html.WriteString(strconv.Itoa(i))
		html.WriteString(`" class="item">hover</a>`)
	}
	html.WriteString("</body></html>")

	doc, err := dom.Parse(html.String())
	if err != nil {
		tb.Fatalf("parse dom: %v", err)
	}
	stylesheet, err := css.Parse(`.item { color: #111111; background-color: #ffffff; } .item:hover { color: #ff0000; background-color: #ffeeee; }`)
	if err != nil {
		tb.Fatalf("parse css: %v", err)
	}

	body := doc.Children[0].Children[1]
	return doc, stylesheet, body.Children[0]
}
