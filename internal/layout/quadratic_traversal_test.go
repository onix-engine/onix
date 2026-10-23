package layout

import (
	"fmt"
	"strings"
	"testing"

	"github.com/diiyw/onix/internal/dom"
	"github.com/diiyw/onix/internal/style"
)

func TestGridTrackPositionsAcrossGapsSpansAndResize(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div id="grid" style="display:grid;grid-template-columns:10px 20px 30px 40px;grid-template-rows:12px 22px 32px;column-gap:3px;row-gap:4px;width:200px;height:150px;justify-content:center;align-content:center"><div id="span" style="grid-column:2 / 5;grid-row:2 / 4"></div><div id="first" style="grid-column:1;grid-row:1"></div></div>`)
	grid, span := retainedNode(root, "grid"), retainedNode(root, "span")
	var cache Cache
	updateRetained(t, &cache, root, styles, face, nil, true)
	for _, width := range []float64{200, 220, 180, 200} {
		styles.Exclusive(grid).Width = style.LengthPx(width)
		got := updateRetained(t, &cache, root, styles, face, []*dom.Node{grid}, false)
		full, err := BuildWithFonts(root, styles, face, face, 320, 200)
		if err != nil {
			t.Fatal(err)
		}
		assertRetainedMatchesFull(t, got, full)
		if rect := cache.nodes[span].BorderBox(); rect.Width != 96 || rect.Height != 58 || rect.X != (width-109)/2+13 || rect.Y != 54 {
			t.Fatalf("unexpected spanned grid cell: %+v", rect)
		}
	}
}

func TestDirtyLayoutReparentMatchesFreshLayout(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div id="left"><div id="moving"><div id="leaf" style="height:10px"></div></div></div><div id="right"></div>`)
	left, right := retainedNode(root, "left"), retainedNode(root, "right")
	moving, leaf := retainedNode(root, "moving"), retainedNode(root, "leaf")
	var cache Cache
	updateRetained(t, &cache, root, styles, face, nil, true)
	for range 3 {
		left.Children = nil
		right.AppendChild(moving)
		got := updateRetained(t, &cache, root, styles, face, []*dom.Node{leaf, moving, right}, true)
		full, err := BuildWithFonts(root, styles, face, face, 320, 200)
		if err != nil {
			t.Fatal(err)
		}
		assertRetainedMatchesFull(t, got, full)
		left, right = right, left
	}
}

func BenchmarkGridTrackTraversal(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			source := fmt.Sprintf(`<div id="grid" style="display:grid;grid-template-columns:repeat(%d, 1px);height:10px;width:%dpx">%s</div>`, count, count, strings.Repeat(`<div></div>`, count))
			root, styles, face := retainedFixture(b, source)
			grid := retainedNode(root, "grid")
			var cache Cache
			updateRetained(b, &cache, root, styles, face, nil, true)
			mutable := styles.Exclusive(grid)
			dirty := []*dom.Node{grid}
			b.ReportAllocs()
			for b.Loop() {
				if mutable.Width.LengthPx == float64(count) {
					mutable.Width = style.LengthPx(float64(count + 1))
				} else {
					mutable.Width = style.LengthPx(float64(count))
				}
				updateRetained(b, &cache, root, styles, face, dirty, false)
			}
		})
	}
}

func BenchmarkDirtyLayoutTraversal(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			root := dom.NewDocument()
			parent := root
			for range count {
				node := dom.NewElement("div", map[string]string{"style": "height:1px"})
				parent.AppendChild(node)
				parent = node
			}
			styles, err := style.Compute(root, nil)
			if err != nil {
				b.Fatal(err)
			}
			face := benchFace(b)
			var cache Cache
			updateRetained(b, &cache, root, styles, face, nil, true)
			dirty := make([]*dom.Node, 0, count)
			for node := root; len(node.Children) != 0; {
				node = node.Children[0]
				dirty = append(dirty, node)
			}
			b.ReportAllocs()
			for b.Loop() {
				updateRetained(b, &cache, root, styles, face, dirty, false)
			}
		})
	}
}
