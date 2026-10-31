package layout

import (
	"testing"

	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/style"
)

func TestClassicScrollbarLayout(t *testing.T) {
	for _, test := range []struct {
		name, container, child string
		width, height          float64
		gutters                Edges
	}{
		{"fits", "overflow:auto", "height:20px", 100, 80, Edges{}},
		{"vertical", "overflow:auto", "height:200px", 90, 80, Edges{Right: 10}},
		{"horizontal", "overflow:auto", "width:200px;height:20px", 100, 70, Edges{Bottom: 10}},
		{"coupled", "overflow:auto", "width:100px;height:200px", 90, 70, Edges{Right: 10, Bottom: 10}},
		{"forced", "overflow:scroll", "height:20px", 90, 70, Edges{Right: 10, Bottom: 10}},
		{"hidden", "overflow:hidden", "width:200px;height:200px", 100, 80, Edges{}},
		{"none", "overflow:auto;scrollbar-width:none", "width:200px;height:200px", 100, 80, Edges{}},
		{"flex", "display:flex;flex-direction:column;overflow:auto", "height:200px;flex-shrink:0", 90, 80, Edges{Right: 10}},
		{"grid", "display:grid;overflow:auto", "height:200px", 90, 80, Edges{Right: 10}},
		{"border box", "box-sizing:border-box;padding:8px;border:2px solid;overflow:auto", "height:200px", 70, 60, Edges{Right: 10}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, styles, face := retainedFixture(t, `<div id="area" style="width:100px;height:80px;scrollbar-width:thin;`+test.container+`"><div id="child" style="`+test.child+`"></div></div>`)
			cache := Cache{ReserveScrollbarSpace: true}
			updateRetained(t, &cache, root, styles, face, nil, true)
			box := cache.nodes[retainedNode(root, "area")]
			if box.Content.Width != test.width || box.Content.Height != test.height || box.ScrollbarGutter != test.gutters {
				t.Fatalf("content=%+v gutters=%+v; want %vx%v %+v", box.Content, box.ScrollbarGutter, test.width, test.height, test.gutters)
			}
			if border := box.BorderBox(); border.Width != 100 || border.Height != 80 {
				t.Fatalf("scrollbar changed outer box: %+v", border)
			}
		})
	}
}

func TestClassicScrollbarWarmReflowDoesNotAllocate(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div id="area" style="width:100px;height:80px;overflow:auto;scrollbar-width:thin"><div id="child" style="height:200px"></div></div>`)
	cache := Cache{ReserveScrollbarSpace: true}
	updateRetained(t, &cache, root, styles, face, nil, true)
	dirty := []*dom.Node{retainedNode(root, "child")}
	for range 4 {
		updateRetained(t, &cache, root, styles, face, dirty, false)
	}
	if allocs := testing.AllocsPerRun(30, func() {
		updateRetained(t, &cache, root, styles, face, dirty, false)
	}); allocs != 0 {
		t.Fatalf("classic scrollbar reflow allocations=%v", allocs)
	}
}

func TestClassicScrollbarDoesNotEnlargeTinyContainer(t *testing.T) {
	for _, size := range []string{"width:5px;height:5px", "position:absolute;top:0;bottom:195px;width:5px"} {
		root, styles, face := retainedFixture(t, `<div id="area" style="`+size+`;overflow:auto;scrollbar-width:thin"><div style="width:20px;height:20px"></div></div>`)
		cache := Cache{ReserveScrollbarSpace: true}
		updateRetained(t, &cache, root, styles, face, nil, true)
		box := cache.nodes[retainedNode(root, "area")]
		if rect := box.BorderBox(); rect.Width != 5 || rect.Height != 5 || box.Content.Width != 0 || box.Content.Height != 0 {
			t.Fatalf("tiny scroll container=%+v, content=%+v", rect, box.Content)
		}
	}
}

func TestClassicScrollbarAutoHeightAndAbsoluteContainingBlock(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div id="area" style="position:relative;width:100px;overflow:auto;scrollbar-width:thin;padding:8px;border:2px solid"><div style="width:200px;height:30px"></div><div id="absolute" style="position:absolute;right:0;top:0;width:10px;height:10px"></div></div>`)
	cache := Cache{ReserveScrollbarSpace: true}
	updateRetained(t, &cache, root, styles, face, nil, true)
	area := cache.nodes[retainedNode(root, "area")]
	if area.Content.Height != 30 || area.BorderBox().Height != 60 || area.BorderBox().Width != 120 {
		t.Fatalf("auto height or outer width = %+v / %+v", area.Content, area.BorderBox())
	}
	absolute := cache.nodes[retainedNode(root, "absolute")]
	if absolute.BorderBox().X+absolute.BorderBox().Width != area.ScrollportBox().X+area.ScrollportBox().Width {
		t.Fatal("absolute child did not use the scrollport containing block")
	}
}

func TestClassicScrollbarRetainedChangesAndModeSwitch(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div id="area" style="width:100px;height:80px;overflow:auto;scrollbar-width:thin"><div id="child" style="height:200px"></div></div>`)
	cache := Cache{ReserveScrollbarSpace: true}
	updateRetained(t, &cache, root, styles, face, nil, true)
	child := retainedNode(root, "child")
	for _, height := range []float64{20, 200, 80, 81, 20} {
		styles[child].Height = style.LengthPx(height)
		dirty := []*dom.Node{child}
		got := updateRetained(t, &cache, root, styles, face, dirty, false)
		fresh := Cache{ReserveScrollbarSpace: true}
		want := updateRetained(t, &fresh, root, styles, face, nil, true)
		assertRetainedMatchesFull(t, got, want)
	}
	cache.ReserveScrollbarSpace = false
	got := updateRetained(t, &cache, root, styles, face, nil, false)
	fresh := Cache{}
	want := updateRetained(t, &fresh, root, styles, face, nil, true)
	assertRetainedMatchesFull(t, got, want)
	for _, box := range cache.nodes {
		if box.ScrollWidth != 0 || box.ScrollHeight != 0 || box.ScrollbarGutter != (Edges{}) {
			t.Fatal("overlay mode retained classic geometry")
		}
	}
}
