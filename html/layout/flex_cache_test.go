package layout

import (
	"fmt"
	"strings"
	"testing"

	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/style"
)

func nestedFlexFixture(depth int) string {
	return `<div id="outer" style="display:flex;flex-direction:column;width:280px;height:180px">` +
		strings.Repeat(`<div style="display:flex;flex-direction:column;min-height:0"><div style="display:flex;height:20px"><span>Icon</span><span>Directory</span></div>`, depth) +
		`<div id="leaf" style="display:flex;height:24px"><span>Name</span><span>Changes</span></div>` +
		strings.Repeat(`</div>`, depth) + `</div>`
}

func TestNestedFlexMeasurementWork(t *testing.T) {
	for _, depth := range []int{2, 4, 6, 8} {
		t.Run(fmt.Sprint(depth), func(t *testing.T) {
			root, styles, face := retainedFixture(t, nestedFlexFixture(depth))
			var cache Cache
			got := updateRetained(t, &cache, root, styles, face, nil, true)
			full, err := BuildWithFonts(root, styles, face, face, 320, 200)
			if err != nil {
				t.Fatal(err)
			}
			assertRetainedMatchesFull(t, got, full)
			t.Logf("depth=%d boxes=%d work=%+v", depth, len(cache.nodes), cache.Work)
			if cache.Work.Reflowed > len(cache.nodes)*10 {
				t.Fatalf("nested flex repeats subtree measurements: %+v for %d boxes", cache.Work, len(cache.nodes))
			}
		})
	}
}

func TestFlexMeasurementCacheMatchesFull(t *testing.T) {
	cases := []struct{ name, content string }{
		{"nested", nestedFlexFixture(3)},
		{"wrap", `<div id="outer" style="display:flex;flex-wrap:wrap;align-content:stretch;width:280px;height:180px"><div id="leaf" style="display:flex;flex-direction:column;min-width:40px;max-width:150px;padding:3px"><span>Words that wrap onto multiple lines</span><input value="Control"/></div><div style="display:flex;width:100px"><span>Other</span><span>Row</span></div></div>`},
		{"positioned", `<div id="outer" style="display:flex;flex-direction:column;position:relative;width:280px;height:180px"><div id="leaf" style="display:flex;position:relative;left:3px;top:2px"><span>Text</span><div style="position:absolute;right:0;bottom:0">Bottom</div></div><div style="display:flex"><span>Sibling</span><span>Text</span></div></div>`},
		{"float", `<div id="outer" style="display:flex;flex-direction:column;width:280px;height:180px"><div id="leaf"><div style="float:left;width:60px;height:40px">Float</div>Text beside the float should wrap</div><div style="display:flex"><span>Sibling</span></div></div>`},
		{"grid", `<div id="outer" style="display:flex;flex-direction:column;width:280px;height:180px"><div id="leaf" style="display:grid;grid-template-columns:1fr 1fr"><div style="display:flex"><span>First</span><span>Cell</span></div><div>Second cell wraps here</div></div></div>`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root, styles, face := retainedFixture(t, test.content)
			outer, leaf := retainedNode(root, "outer"), retainedNode(root, "leaf")
			var cache Cache
			for i := range 24 {
				styles.Exclusive(outer).FlexDirection = style.Keyword([]string{"row", "column", "row-reverse", "column-reverse"}[i%4])
				styles.Exclusive(outer).Width = style.LengthPx([]float64{280, 110, 200}[i%3])
				styles.Exclusive(outer).Height = style.LengthPx([]float64{180, 80}[i%2])
				styles.Exclusive(leaf).Width = []style.Value{style.Auto(), style.LengthPx(75)}[i%2]
				styles.Exclusive(leaf).AlignSelf = style.Keyword([]string{"stretch", "center", "flex-start"}[i%3])
				got := updateRetained(t, &cache, root, styles, face, []*dom.Node{outer, leaf}, i == 0)
				full, err := BuildWithFonts(root, styles, face, face, 320, 200)
				if err != nil {
					t.Fatal(err)
				}
				t.Run(fmt.Sprint(i), func(t *testing.T) { assertRetainedMatchesFull(t, got, full) })
			}
		})
	}
}

func TestNestedFlexWarmResizeDoesNotAllocate(t *testing.T) {
	root, styles, face := retainedFixture(t, nestedFlexFixture(4))
	var cache Cache
	updateRetained(t, &cache, root, styles, face, nil, true)
	outer := retainedNode(root, "outer")
	mutable := styles.Exclusive(outer)
	dirty := []*dom.Node{outer}
	allocations := testing.AllocsPerRun(50, func() {
		for _, width := range [...]float64{240, 280} {
			mutable.Width = style.LengthPx(width)
			updateRetained(t, &cache, root, styles, face, dirty, false)
		}
	})
	if allocations != 0 {
		t.Fatalf("nested flex resize allocated %g objects", allocations)
	}
}

func TestFlexBasisCacheInvalidatesOnTextChange(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div style="display:flex;flex-direction:column;width:160px;height:180px"><div style="display:flex;flex-direction:column"><div id="target">Short</div><div>Following</div></div></div>`)
	var cache Cache
	updateRetained(t, &cache, root, styles, face, nil, true)
	text := retainedNode(root, "target").Children[0]
	for i := range 6 {
		text.Data = []string{"Short", "Words that wrap onto several lines and change the intrinsic flex basis"}[i%2]
		got := updateRetained(t, &cache, root, styles, face, []*dom.Node{text}, false)
		full, err := BuildWithFonts(root, styles, face, face, 320, 200)
		if err != nil {
			t.Fatal(err)
		}
		assertRetainedMatchesFull(t, got, full)
	}
}
