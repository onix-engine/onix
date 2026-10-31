package html

import (
	"math"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/html/style"
	"github.com/diiyw/onix/scene"
)

func htmlStickyPosition(t testing.TB, world *ecs.World, entity ecs.Entity) (x, y float32) {
	t.Helper()
	if entity == 0 {
		t.Fatal("missing sticky visual")
	}
	for entity != 0 {
		if transform := world.GetRef[scene.UITransform](entity); transform != nil {
			x += transform.Translation.X.Value
			y += transform.Translation.Y.Value
		}
		parent, ok := world.Get[scene.Parent](entity)
		if !ok {
			break
		}
		entity = parent.Entity
	}
	return
}

// Expected border-box positions were also checked against Chromium. Scroll
// updates deliberately run without HTML Sync to exercise the input-frame path.
func TestHTMLStickyChromiumGeometry(t *testing.T) {
	for _, tt := range []struct {
		name, content, scroller string
		scroll, wantY           float32
	}{
		{"normal flow", `<div style="height:40px"></div><div id="target" style="position:sticky;top:10px;height:20px;background:red"></div><div style="height:1000px"></div>`, "", 0, 40},
		{"top", `<div style="height:40px"></div><div id="target" style="position:sticky;top:10px;height:20px;background:red"></div><div style="height:1000px"></div>`, "", 80, 10},
		{"direct child", `<div id="target" style="position:sticky;top:0;height:20px;background:red"></div><div style="height:1000px"></div>`, "", 200, 0},
		{"containing block", `<div style="height:40px"></div><div style="height:140px"><div id="target" style="position:sticky;top:10px;height:20px;background:red"></div></div><div style="height:1000px"></div>`, "", 170, -10},
		{"bottom", `<div style="height:80px"></div><div id="target" style="position:sticky;bottom:10px;height:30px;background:red"></div><div style="height:1000px"></div>`, "", 0, 60},
		{"oversized", `<div style="height:40px"></div><div id="target" style="position:sticky;top:10px;bottom:10px;height:140px;background:red"></div><div style="height:1000px"></div>`, "", 80, 10},
		{"margins", `<div style="height:40px"></div><div style="height:140px;display:flow-root"><div id="target" style="position:sticky;top:10px;height:20px;margin:10px 0 15px;background:red"></div></div><div style="height:1000px"></div>`, "", 170, -25},
		{"hidden blocks outer scroll", `<div style="overflow:hidden;height:180px"><div style="height:40px"></div><div id="target" style="position:sticky;top:10px;height:20px;background:red"></div></div><div style="height:1000px"></div>`, "", 80, -40},
		{"clip permits outer scroll", `<div style="overflow:clip;height:180px"><div style="height:40px"></div><div id="target" style="position:sticky;top:10px;height:20px;background:red"></div></div><div style="height:1000px"></div>`, "", 80, 10},
		{"padding", `<div id="target" style="position:sticky;top:0;height:20px;background:red"></div><div style="height:1000px"></div>`, "padding:10px;border:2px solid black;", 900, 12},
		{"percentage inset", `<div id="target" style="position:sticky;top:10%;height:20px;background:red"></div><div style="height:1000px"></div>`, "padding:10px;border:2px solid black;", 200, 22},
		{"auto insets", `<div style="height:40px"></div><div id="target" style="position:sticky;height:20px;background:red"></div><div style="height:1000px"></div>`, "", 80, -40},
	} {
		t.Run(tt.name, func(t *testing.T) {
			world, owner := incrementalHTMLFixture(t, `<div id="scroll" style="height:100px;width:200px;overflow:auto;`+tt.scroller+`">`+tt.content+`</div>`)
			target := incrementalHTMLNode(t, world, owner, "target")
			scroll := incrementalHTMLNode(t, world, owner, "scroll")
			area, ok := scroll.ScrollContainer()
			if !ok {
				t.Fatal("missing scroll container")
			}
			bounds, _ := scroll.Bounds()
			entity := htmlCullRole(world, owner, target, htmlRoleSurface)
			for _, position := range []float32{tt.scroll, 0, tt.scroll} {
				world.GetMut[scene.UIScroll](area).Position[1] = -position
				SyncScrollContent(world)
				if position == tt.scroll {
					_, y := htmlStickyPosition(t, world, entity)
					if math.Abs(float64(y-bounds.Y-tt.wantY)) > 1e-5 {
						t.Fatalf("sticky y=%v, want %v", y-bounds.Y, tt.wantY)
					}
				}
			}
		})
	}
}

func TestHTMLStickyHorizontal(t *testing.T) {
	for _, tt := range []struct {
		inset   string
		x, want float32
	}{
		{"left:10px", -100, 10},
		{"right:10px;margin-left:180px", 0, 180},
		{"left:10%;right:0", -100, 20},
	} {
		t.Run(tt.inset, func(t *testing.T) {
			world, owner := incrementalHTMLFixture(t, `<div id="scroll" style="width:200px;height:100px;overflow:auto"><div style="width:800px;height:300px"><div id="target" style="position:sticky;width:50px;height:20px;background:red;`+tt.inset+`"></div></div></div>`)
			scroll := incrementalHTMLNode(t, world, owner, "scroll")
			area, _ := scroll.ScrollContainer()
			world.GetMut[scene.UIScroll](area).Position[0] = tt.x
			SyncScrollContent(world)
			target := incrementalHTMLNode(t, world, owner, "target")
			x, _ := htmlStickyPosition(t, world, htmlCullRole(world, owner, target, htmlRoleSurface))
			bounds, _ := scroll.Bounds()
			if x-bounds.X != tt.want {
				t.Fatalf("sticky x=%v, want %v", x-bounds.X, tt.want)
			}
		})
	}
}

func TestHTMLStickyNestedContexts(t *testing.T) {
	for _, tt := range []struct {
		name, content       string
		outer, inner, wantY float32
	}{
		{"nested sticky", `<div style="position:sticky;top:0;height:60px;background:red"><div style="height:20px"></div><div id="target" style="position:sticky;top:10px;height:20px;background:blue"></div></div>`, 70, 0, 20},
		{"nested scroll", `<div style="height:40px"></div><div id="inner" style="height:80px;overflow:auto"><div style="height:40px"></div><div id="target" style="position:sticky;top:5px;height:20px;background:red"></div><div style="height:1000px"></div></div>`, 20, 50, 25},
		{"sticky clip", `<div id="target" style="position:sticky;top:10px;height:60px;overflow:hidden;background:red"><div id="child" style="height:20px;background:blue"></div></div>`, 70, 0, 10},
		{"opacity ancestor", `<div style="opacity:.5;height:200px"><div id="target" style="position:sticky;top:10px;height:20px;background:red"></div></div>`, 70, 0, 10},
	} {
		t.Run(tt.name, func(t *testing.T) {
			world, owner := incrementalHTMLFixture(t, `<div id="scroll" style="height:100px;overflow:auto">`+tt.content+`<div style="height:1000px"></div></div>`)
			scroll := incrementalHTMLNode(t, world, owner, "scroll")
			area, _ := scroll.ScrollContainer()
			world.GetMut[scene.UIScroll](area).Position[1] = -tt.outer
			if tt.inner != 0 {
				inner := incrementalHTMLNode(t, world, owner, "inner")
				innerArea, _ := inner.ScrollContainer()
				world.GetMut[scene.UIScroll](innerArea).Position[1] = -tt.inner
			}
			SyncScrollContent(world)
			target := incrementalHTMLNode(t, world, owner, "target")
			_, y := htmlStickyPosition(t, world, htmlCullRole(world, owner, target, htmlRoleSurface))
			bounds, _ := scroll.Bounds()
			if y-bounds.Y != tt.wantY {
				t.Fatalf("sticky y=%v, want %v", y-bounds.Y, tt.wantY)
			}
			if child, ok := NodeByID(world, owner, "child"); ok {
				_, childY := htmlStickyPosition(t, world, htmlCullRole(world, owner, child, htmlRoleSurface))
				if childY != y {
					t.Fatalf("sticky clip descendant y=%v, want %v", childY, y)
				}
			}
		})
	}
}

func TestHTMLStickyRightInset(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="scroll" style="width:200px;height:100px;overflow:auto;scrollbar-width:none"><div style="display:flex;width:800px;height:300px"><div style="width:180px;flex-shrink:0"></div><div id="target" style="position:sticky;right:10px;width:50px;height:20px;flex-shrink:0;background:red"></div></div></div>`)
	SyncScrollContent(world)
	target := incrementalHTMLNode(t, world, owner, "target")
	scroll := incrementalHTMLNode(t, world, owner, "scroll")
	x, _ := htmlStickyPosition(t, world, htmlCullRole(world, owner, target, htmlRoleSurface))
	bounds, _ := scroll.Bounds()
	if x-bounds.X != 140 {
		t.Fatalf("right sticky x=%v, want 140", x-bounds.X)
	}
}

func TestHTMLStickyRetainedPaintAndMutations(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="scroll" style="height:100px;overflow:auto"><div id="target" style="position:sticky;top:0;height:20px;background:red"><input id="field" value="Kept"/></div>`+strings.Repeat(`<div style="height:20px">Content</div>`, 200)+`</div>`)
	root := world.GetRef[htmlRenderState](owner).Boxes
	target := incrementalHTMLNode(t, world, owner, "target")
	scroll := incrementalHTMLNode(t, world, owner, "scroll")
	area, _ := scroll.ScrollContainer()
	entity := htmlCullRole(world, owner, target, htmlRoleSurface)
	field := htmlCullRole(world, owner, incrementalHTMLNode(t, world, owner, "field"), htmlRoleControl)
	_, initialFieldY := htmlStickyPosition(t, world, field)
	for _, offset := range []float32{50, 1000, 2000, 0} {
		world.GetMut[scene.UIScroll](area).Position[1] = -offset
		incrementalSync(t, world)
		SyncScrollContent(world)
		if htmlCullRole(world, owner, target, htmlRoleSurface) != entity {
			t.Fatal("scroll culling retired the sticky surface")
		}
		_, y := htmlStickyPosition(t, world, entity)
		bounds, _ := scroll.Bounds()
		if y != bounds.Y {
			t.Fatalf("sticky surface y=%v, want %v", y, bounds.Y)
		}
		_, fieldY := htmlStickyPosition(t, world, field)
		if fieldY != initialFieldY {
			t.Fatalf("descendant control moved twice: %v, want %v", fieldY, initialFieldY)
		}
	}
	if world.GetRef[htmlRenderState](owner).Boxes != root {
		t.Fatal("scrolling rebuilt layout")
	}
	for _, position := range []string{"static", "sticky", "relative", "sticky"} {
		target.SetAttribute("style", "position:"+position+";top:10px;height:20px;background:red")
		incrementalSync(t, world)
		world.GetMut[scene.UIScroll](area).Position[1] = -50
		SyncScrollContent(world)
		_, y := htmlStickyPosition(t, world, entity)
		bounds, _ := scroll.Bounds()
		want := float32(-50)
		if position == "sticky" {
			want = 10
		} else if position == "relative" {
			want = -40
		}
		if y-bounds.Y != want {
			t.Fatalf("%s y=%v, want %v", position, y-bounds.Y, want)
		}
	}
	if allocations := testing.AllocsPerRun(100, func() {
		world.GetMut[scene.UIScroll](area).Position[1] = -60
		if err := Sync(world); err != nil {
			panic(err)
		}
		SyncScrollContent(world)
		world.GetMut[scene.UIScroll](area).Position[1] = -50
		if err := Sync(world); err != nil {
			panic(err)
		}
		SyncScrollContent(world)
	}); allocations != 0 {
		t.Fatalf("sticky scroll allocated %v objects", allocations)
	}
}

func TestHTMLStickyTableHeaders(t *testing.T) {
	for _, tag := range []string{"th", "tr", "thead"} {
		t.Run(tag, func(t *testing.T) {
			world, owner := incrementalHTMLFixture(t, `<style>table{border-spacing:0}td,th{padding:0;height:20px}`+tag+`{position:sticky;top:0}</style><div id="scroll" style="height:100px;overflow:auto"><table id="table"><thead><tr><th id="target" style="background:red">Header</th></tr></thead><tbody>`+strings.Repeat(`<tr><td>Row</td></tr>`, 30)+`</tbody></table><div style="height:500px"></div></div>`)
			scroll := incrementalHTMLNode(t, world, owner, "scroll")
			area, _ := scroll.ScrollContainer()
			world.GetMut[scene.UIScroll](area).Position[1] = -200
			SyncScrollContent(world)
			target := incrementalHTMLNode(t, world, owner, "target")
			entity := htmlCullRole(world, owner, target, htmlRoleSurface)
			_, y := htmlStickyPosition(t, world, entity)
			bounds, _ := scroll.Bounds()
			if y != bounds.Y {
				t.Fatalf("sticky %s header y=%v, want %v", tag, y, bounds.Y)
			}
			table, _ := incrementalHTMLNode(t, world, owner, "table").Bounds()
			header, _ := target.Bounds()
			world.GetMut[scene.UIScroll](area).Position[1] = -(table.Height + table.Y - bounds.Y - 5)
			SyncScrollContent(world)
			_, y = htmlStickyPosition(t, world, entity)
			if math.Abs(float64(y-bounds.Y-(5-header.Height))) > .001 {
				t.Fatalf("sticky %s escaped table: y=%v header height=%v", tag, y-bounds.Y, header.Height)
			}
		})
	}
}

func TestHTMLStickyCreatesStackingAndAbsoluteContainingBlock(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div style="height:30px"></div><div id="target" style="position:sticky;top:10px;height:80px"><div id="absolute" style="position:absolute;top:5px;width:10px;height:10px"></div></div><div id="next" style="height:20px"></div>`)
	state := world.GetRef[htmlRenderState](owner)
	box := state.BoxesByNode[incrementalHTMLNode(t, world, owner, "target").node]
	if !htmlBoxCreatesStackContext(box) || box.Style.ZIndex != style.Keyword("auto") {
		t.Fatal("sticky with auto z-index must create a stacking context")
	}
	absolute, _ := incrementalHTMLNode(t, world, owner, "absolute").Bounds()
	next, _ := incrementalHTMLNode(t, world, owner, "next").Bounds()
	if absolute.Y != 35 || next.Y != 110 {
		t.Fatalf("sticky changed normal flow or containing block: absolute=%+v next=%+v", absolute, next)
	}
}
