package layout

import (
	"testing"

	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/style"
)

func TestRetainedGridLayoutMatchesFull(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div id="grid" style="display:grid;grid-template-columns:1fr 1fr;grid-template-rows:30px 1fr;width:160px;height:90px;gap:5px"><div id="target"></div><div style="grid-column:2">Two</div><div style="grid-column:1 / 3;grid-row:2">Three</div></div>`)
	grid := retainedNode(root, "grid")
	target := retainedNode(root, "target")
	var cache Cache
	updateRetained(t, &cache, root, styles, face, nil, true)
	for _, width := range []float64{160, 180, 140, 160} {
		styles.Exclusive(grid).Width = style.LengthPx(width)
		got := updateRetained(t, &cache, root, styles, face, []*dom.Node{grid}, false)
		full, err := BuildWithFonts(root, styles, face, face, 320, 200)
		if err != nil {
			t.Fatal(err)
		}
		assertRetainedMatchesFull(t, got, full)
		if target == nil || cache.nodes[target] == nil {
			t.Fatal("retained grid item missing")
		}
	}
}

func TestRetainedGridWarmResizeDoesNotAllocate(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div id="grid" style="display:grid;grid-template-columns:1fr 1fr;grid-template-rows:30px 1fr;width:160px;height:90px;gap:5px"><div></div><div></div><div></div></div>`)
	grid := retainedNode(root, "grid")
	var cache Cache
	updateRetained(t, &cache, root, styles, face, nil, true)
	mutable := styles.Exclusive(grid)
	allocations := testing.AllocsPerRun(100, func() {
		for _, width := range [...]float64{180, 160} {
			mutable.Width = style.LengthPx(width)
			if _, err := cache.Update(root, styles, face, face, 320, 200, []*dom.Node{grid}, nil, false, false); err != nil {
				panic(err)
			}
		}
	})
	if allocations != 0 {
		t.Fatalf("warm grid resize allocated %v objects", allocations)
	}
}

func TestGridAutoTracksStretchFreeSpace(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div id="grid" style="display:grid;grid-template-columns:1fr;grid-template-rows:auto auto;align-content:stretch;width:150px;height:80px"><div id="first" style="height:20px"></div><div id="second" style="height:10px"></div></div>`)
	laidOut, err := Build(root, styles, face, 320, 200)
	if err != nil {
		t.Fatal(err)
	}
	first := findHashTestBoxByID(laidOut, "first")
	second := findHashTestBoxByID(laidOut, "second")
	if first == nil || second == nil {
		t.Fatal("grid children missing")
	}
	if first.BorderBox().Y != 0 || first.BorderBox().Height != 20 || second.BorderBox().Y != 45 || second.BorderBox().Height != 10 {
		t.Fatalf("stretched auto rows = first:%+v second:%+v", first.BorderBox(), second.BorderBox())
	}
}
