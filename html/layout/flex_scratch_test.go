package layout

import (
	"testing"

	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/style"
)

func TestRetainedNestedFlexScratchMatchesFull(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div id="outer" style="display:flex;flex-wrap:wrap;align-content:center;width:100px;height:200px"><div id="inner" style="display:flex;flex-wrap:wrap;align-content:space-around;width:80px;height:100px"><div style="width:40px;height:20px;color:red">One</div><div style="width:40px;height:30px;color:blue">Two</div><div style="width:40px;height:10px">Three</div></div><div style="width:80px;height:50px">Sibling</div></div>`)
	var cache Cache
	outer, inner := retainedNode(root, "outer"), retainedNode(root, "inner")
	updateRetained(t, &cache, root, styles, face, nil, true)
	for _, direction := range []string{"row", "column", "row-reverse", "column-reverse"} {
		for _, width := range []float64{240, 60, 160, 100, 240} {
			styles.Exclusive(outer).Width = style.LengthPx(width)
			styles.Exclusive(outer).FlexDirection = style.Keyword(direction)
			styles.Exclusive(inner).Width = style.LengthPx(width / 2)
			styles.Exclusive(inner).FlexDirection = style.Keyword(direction)
			styles.Exclusive(inner).AlignItems = style.Keyword("flex-start")
			got := updateRetained(t, &cache, root, styles, face, []*dom.Node{outer, inner}, false)
			full, err := BuildWithFonts(root, styles, face, face, 320, 200)
			if err != nil {
				t.Fatal(err)
			}
			assertRetainedMatchesFull(t, got, full)
			if styles[outer].Width != style.LengthPx(width) || styles[inner].Width != style.LengthPx(width/2) {
				t.Fatal("measurement mutated computed styles")
			}
		}
	}
}

func TestFlexScratchReleasesDetachedChildren(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div id="parent" style="display:flex;flex-wrap:wrap;width:60px"><div style="width:40px">One</div><div style="width:40px">Two</div></div>`)
	var cache Cache
	parent := retainedNode(root, "parent")
	updateRetained(t, &cache, root, styles, face, nil, true)
	for _, child := range parent.Children {
		child.Parent = nil
	}
	clear(parent.Children)
	parent.Children = parent.Children[:0]
	updateRetained(t, &cache, root, styles, face, []*dom.Node{parent}, true)
	scratch := cache.nodes[parent].flex
	for _, item := range scratch.items[:cap(scratch.items)] {
		if item.box != nil {
			t.Fatal("scratch retains removed box")
		}
	}
	for _, line := range scratch.lines[:cap(scratch.lines)] {
		if line.items != nil {
			t.Fatal("scratch retains old line")
		}
	}
}
