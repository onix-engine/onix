package style

import (
	"fmt"
	"strings"
	"testing"

	"github.com/onix-engine/onix/html/css"
	"github.com/onix-engine/onix/html/dom"
)

func TestCandidateTraversalCascadeWeights(t *testing.T) {
	// Class order and bucket order must never replace stylesheet source order.
	for _, classes := range []string{"a b", "b a", "a b a b"} {
		node := dom.NewElement("div", map[string]string{"id": "target", "class": classes, "style": "height:7px;color:purple"})
		sheet, err := css.Parse(`
			.b {width:10px;margin-left:8px;color:red!important}
			#target {height:3px}
			.a {width:20px;margin:2px;color:blue!important}
			div {height:4px}
			* {height:5px}
			#target {height:6px}
		`)
		if err != nil {
			t.Fatal(err)
		}
		styles, err := Compute(node, sheet)
		if err != nil {
			t.Fatal(err)
		}
		got := styles[node]
		if got.Width != LengthPx(20) || got.Height != LengthPx(7) || got.Margin.Left != LengthPx(2) || got.Color != ColorValue(Color{B: 255, A: 255}) {
			t.Fatalf("class=%q: incorrect source order, shorthand, inline or important precedence", classes)
		}
	}
}

func BenchmarkCandidateCascade(b *testing.B) {
	for _, count := range []int{8, 64, 256} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			var source strings.Builder
			for index := range count {
				fmt.Fprintf(&source, ".c%d {width:%dpx} ", index%4, index+1)
			}
			sheet, err := css.Parse(source.String())
			if err != nil {
				b.Fatal(err)
			}
			node := dom.NewElement("div", map[string]string{"class": "c3 c1 c0 c2"})
			scratch := acquireCascadeScratch()
			defer releaseCascadeScratch(scratch)
			scratch.compiled = compileStylesheet(sheet)
			run := func() {
				scratch.matcher.Reset()
				if _, err := collectSpecifiedValuesInto(node, sheet, nil, nil, scratch); err != nil {
					b.Fatal(err)
				}
			}
			run()
			b.ReportAllocs()
			for b.Loop() {
				run()
			}
		})
	}
}
