package html

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestHTMLOverflowModesMatchFresh(t *testing.T) {
	modes := []string{"visible", "auto", "scroll", "hidden", "clip", "visible clip", "clip visible", "visible auto", "clip scroll", "hidden auto"}
	for _, initial := range modes {
		for _, outer := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/outer=%v", initial, outer), func(t *testing.T) {
				prefix := `<div style="height:200px">`
				if outer {
					prefix = `<div id="outer" style="height:200px;overflow:auto">`
				}
				source := func(mode string) string {
					return prefix + `<div id="target" tabindex="0" style="position:relative;width:120px;height:40px;overflow:` + mode + `"><div id="inner" style="overflow:auto;height:30px"><input value="Value"/></div><div style="position:absolute;top:100px;width:180px">Absolute</div><span> </span></div></div><div><input id="other" value="Unchanged"/></div>`
				}
				world, owner := incrementalHTMLFixture(t, source(initial))
				target := incrementalHTMLNode(t, world, owner, "target")
				other := incrementalControl(t, world, owner, "other")
				before, _ := ecs.Get[Control](world, other)
				for _, mode := range modes {
					target.SetAttribute("style", "position:relative;width:120px;height:40px;overflow:"+mode)
					incrementalSync(t, world)
					state, _ := ecs.Get[htmlRenderState](world, owner)
					// Equal computed modes can be an intentional no-op.
					if state.SyncStats.PartialReconciles == 0 && state.SyncStats.ReconciledBoxes != 0 {
						t.Fatalf("%s fell back: %+v", mode, state.SyncStats)
					}
					after, _ := ecs.Get[Control](world, other)
					if after.RenderRevision != before.RenderRevision {
						t.Fatal("unrelated control rebuilt")
					}
					assertHTMLContextMatchesFresh(t, world, owner, source(mode))
					for _, entity := range state.Children {
						if parent, ok := ecs.Get[scene.Parent](world, entity); ok && !world.IsAlive(parent.Entity) {
							t.Fatal("retired parent")
						}
						if content, ok := ecs.Get[ScrollContent](world, entity); ok && !ecs.Has[scene.UIScroll](world, content.Area) {
							t.Fatal("stale scroll binding")
						}
					}
				}
			})
		}
	}
}

func TestHTMLScrollbarWidthNoneKeepsScrollWithoutZeroChildren(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="target" style="width:40px;height:20px;overflow:auto;scrollbar-width:none"><div style="height:100px">Scrollable</div></div>`)
	state, _ := ecs.Get[htmlRenderState](world, owner)
	target := incrementalHTMLNode(t, world, owner, "target")
	binding, ok := state.OverflowBindings[target.node]
	if !ok {
		t.Fatal("missing overflow binding")
	}
	if entity, ok := target.ScrollContainer(); !ok || entity != binding.entity {
		t.Fatalf("node scroll container = %v, %v; want %v", entity, ok, binding.entity)
	}
	if len(binding.parts) != 0 {
		t.Fatalf("hidden scrollbar emitted %d visual parts", len(binding.parts))
	}
	for index, entity := range state.Children {
		if entity == 0 {
			t.Fatalf("children[%d] is the zero entity", index)
		}
	}
	scroll, ok := ecs.Get[scene.UIScroll](world, binding.entity)
	if !ok || !scroll.Vertical || scroll.ScrollbarSize != 0 {
		t.Fatalf("hidden scrollbar scroll state = %#v, ok=%v", scroll, ok)
	}
}

func TestHTMLOverflowScrollbarsOmitArrowParts(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="target" style="width:40px;height:20px;overflow:scroll"><div style="width:100px;height:100px">Scrollable</div></div>`)
	state, _ := ecs.Get[htmlRenderState](world, owner)
	target := incrementalHTMLNode(t, world, owner, "target")
	binding, ok := state.OverflowBindings[target.node]
	if !ok {
		t.Fatal("missing overflow binding")
	}
	if len(binding.parts) != 4 {
		t.Fatalf("scrollbar emitted %d visual parts, want one track and thumb per axis", len(binding.parts))
	}
	seen := [2][2]bool{}
	for _, entity := range binding.parts {
		part, ok := ecs.Get[ScrollbarPart](world, entity)
		if !ok {
			t.Fatalf("scrollbar visual %v is missing its part metadata", entity)
		}
		if part.Axis > ScrollbarVertical || part.Kind > ScrollbarThumb {
			t.Fatalf("unexpected scrollbar part axis=%d kind=%d", part.Axis, part.Kind)
		}
		seen[part.Axis][part.Kind] = true
	}
	for axis := range seen {
		if !seen[axis][ScrollbarTrack] || !seen[axis][ScrollbarThumb] {
			t.Fatalf("axis %d did not emit exactly track and thumb: %#v", axis, seen[axis])
		}
	}
}

func TestHTMLScrollbarTransparentColors(t *testing.T) {
	const baseStyle = "width:40px;height:20px;overflow:scroll;scrollbar-width:thin;"
	world, owner := incrementalHTMLFixture(t, `<div id="target" style="`+baseStyle+`"><div style="width:100px;height:100px">Scrollable</div></div>`)
	target := incrementalHTMLNode(t, world, owner, "target")
	for _, test := range []struct {
		colors string
		track  graphics.ColorRGBA
		thumb  graphics.ColorRGBA
	}{
		{"#65708280 transparent", graphics.ColorRGBA{}, graphics.ColorFromRGBA(101, 112, 130, 128)},
		{"transparent #65708280", graphics.ColorFromRGBA(101, 112, 130, 128), graphics.ColorRGBA{}},
		{"transparent transparent", graphics.ColorRGBA{}, graphics.ColorRGBA{}},
		{"auto", graphics.ColorFromRGB(252, 252, 252), graphics.ColorFromRGB(139, 139, 139)},
	} {
		t.Run(test.colors, func(t *testing.T) {
			target.SetAttribute("style", baseStyle+"scrollbar-color:"+test.colors)
			incrementalSync(t, world)
			SyncScrollbars(world)
			state, _ := ecs.Get[htmlRenderState](world, owner)
			binding := state.OverflowBindings[target.node]
			scroll, ok := ecs.Get[scene.UIScroll](world, binding.entity)
			if !ok || !scroll.Horizontal || !scroll.Vertical || scroll.ScrollbarSize != 10 {
				t.Fatalf("transparent colors changed scroll geometry: %+v", scroll)
			}
			seen := [2][2]bool{}
			for _, entity := range binding.parts {
				part, _ := ecs.Get[ScrollbarPart](world, entity)
				sprite, _ := ecs.Get[scene.Sprite2D](world, entity)
				want := test.track
				if part.Kind == ScrollbarThumb {
					want = test.thumb
				}
				if want.A == 0 || sprite.Shape.Fill != want {
					t.Fatalf("axis %d part %d color = %v, want %v", part.Axis, part.Kind, sprite.Shape.Fill, want)
				}
				seen[part.Axis][part.Kind] = true
			}
			for axis := range seen {
				if seen[axis][ScrollbarTrack] != (test.track.A > 0) || seen[axis][ScrollbarThumb] != (test.thumb.A > 0) {
					t.Fatalf("axis %d parts = %v", axis, seen[axis])
				}
			}
			for _, entity := range state.Children {
				if entity == 0 {
					t.Fatal("transparent scrollbar emitted a zero child entity")
				}
			}
		})
	}
}

func TestHTMLOverlayScrollbarKeepsContentAndPaintsAboveDescendants(t *testing.T) {
	source := func(sectionStyle string) string {
		return `<div id="frame" style="width:100px;height:80px;overflow:hidden"><div id="area" style="position:relative;width:120px;height:100px;overflow-y:auto;overflow-x:hidden;scrollbar-width:thin;scrollbar-color:#65708280 transparent"><div id="section" style="` + sectionStyle + `"><button id="menu" style="position:absolute;right:0;width:26px;height:22px">...</button></div></div></div>`
	}
	const sectionStyle = "position:relative;height:300px;background:#20232b"
	world, owner := incrementalHTMLFixture(t, source(sectionStyle))
	section := incrementalHTMLNode(t, world, owner, "section")
	area := incrementalHTMLNode(t, world, owner, "area")
	for _, css := range []string{sectionStyle, sectionStyle + ";z-index:10", sectionStyle + ";z-index:0", sectionStyle} {
		section.SetAttribute("style", css)
		incrementalSync(t, world)
		SyncScrollbars(world)
		if err := scene.SyncUIGlobalTransformSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		state := ecs.GetRef[htmlRenderState](world, owner)
		binding := state.OverflowBindings[area.node]
		scroll, _ := ecs.Get[scene.UIScroll](world, binding.entity)
		clip, _ := ecs.Get[scene.ClipRect2D](world, binding.entity)
		if !scroll.ScrollbarOverlay || scroll.ViewportSize != (mgl32.Vec2{120, 100}) || clip.Width != 120 || clip.Height != 100 {
			t.Fatalf("overlay scrollbar reserved content space: scroll=%+v clip=%+v", scroll, clip)
		}
		menu, _ := ControlByID(world, owner, "menu")
		menuNode := incrementalHTMLNode(t, world, owner, "menu")
		menuRect := state.BoxesByNode[menuNode.node].BorderBox()
		if menuRect.X+menuRect.Width != 120 {
			t.Fatalf("right-aligned menu moved: %+v", menuRect)
		}
		if len(binding.parts) != 1 {
			t.Fatalf("transparent overlay emitted %d parts, want one thumb", len(binding.parts))
		}
		thumb := binding.parts[0]
		sprite, _ := ecs.Get[scene.Sprite2D](world, thumb)
		if sprite.Shape.Fill != graphics.ColorFromRGBA(101, 112, 130, 128) {
			t.Fatalf("overlay thumb lost transparency: %v", sprite.Shape.Fill)
		}
		position, _ := ecs.Get[scene.UITransform](world, thumb)
		size, _ := ecs.Get[scene.Size2D](world, thumb)
		if position.Translation.X.Value != 112 || size.Width != 6 {
			t.Fatalf("overlay thumb does not lie inside the viewport: position=%+v size=%+v", position, size)
		}
		thumbStack, _ := ecs.Get[scene.UIStackIndex](world, thumb)
		menuStack, _ := ecs.Get[scene.UIStackIndex](world, menu)
		if thumbStack <= menuStack+4 {
			t.Fatalf("descendants cover the overlay thumb: thumb=%d menu=%d", thumbStack, menuStack)
		}
		global, _ := ecs.Get[scene.UIGlobalTransform](world, thumb)
		inheritedClip, ok := global.ClipRect()
		if !ok || inheritedClip.Width != 100 || inheritedClip.Height != 80 {
			t.Fatalf("overlay scrollbar escaped its ancestor clip: %+v, ok=%v", inheritedClip, ok)
		}
		fresh, freshOwner := incrementalHTMLFixture(t, source(css))
		SyncScrollbars(fresh)
		freshState, _ := ecs.Get[htmlRenderState](fresh, freshOwner)
		if !reflect.DeepEqual(htmlPrimitiveSnapshot(world, *state), htmlPrimitiveSnapshot(fresh, freshState)) {
			t.Fatalf("overlay scrollbar update differs from fresh render for %s", css)
		}
		Dispose(fresh)
	}
}

func TestHTMLOverflowModeFlexLayoutMatchesFresh(t *testing.T) {
	source := func(mode string) string {
		return `<div style="display:flex;width:80px"><div id="target" style="overflow:` + mode + `"><div style="width:200px">Wide content</div></div><div>Sibling</div></div>`
	}
	world, owner := incrementalHTMLFixture(t, source("visible"))
	target := incrementalHTMLNode(t, world, owner, "target")
	for _, mode := range []string{"auto", "visible", "hidden", "clip", "scroll"} {
		target.SetAttribute("style", "overflow:"+mode)
		incrementalSync(t, world)
		state, _ := ecs.Get[htmlRenderState](world, owner)
		if state.SyncStats.PartialReconciles != 1 {
			t.Fatal("flex mode change fell back")
		}
		assertHTMLContextMatchesFresh(t, world, owner, source(mode))
	}
}

func TestHTMLOverflowModeSkipsUnrelatedControls(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div style="height:120px"><div id="target" style="overflow:auto;height:40px"><div><input value="Value"/></div></div></div>`+strings.Repeat(`<div style="overflow:auto;height:40px"><input value="Unchanged"/></div>`, 1000))
	target := incrementalHTMLNode(t, world, owner, "target")
	for _, mode := range []string{"hidden", "visible", "auto", "scroll"} {
		target.SetAttribute("style", "height:40px;overflow:"+mode)
		incrementalSync(t, world)
		state, _ := ecs.Get[htmlRenderState](world, owner)
		if state.SyncStats.PartialReconciles != 1 || state.SyncStats.UpdatedControls != 1 || state.SyncStats.SnapshottedControls != 1 || state.SyncStats.UpdatedClips > 1 {
			t.Fatalf("nonlocal mode transition: %+v", state.SyncStats)
		}
	}
}

func BenchmarkHTMLOverflowMode(b *testing.B) {
	for _, count := range []int{10, 1000} {
		for _, mode := range []string{"hidden", "visible"} {
			b.Run(fmt.Sprintf("%d/%s", count, mode), func(b *testing.B) {
				world, owner := incrementalHTMLFixture(b, `<style>#target{height:40px;overflow:auto}#target.changed{overflow:`+mode+`}</style><div style="height:120px"><div id="target"><div><input value="Value"/></div></div></div>`+strings.Repeat(`<div style="overflow:auto;height:40px"><input value="Unchanged"/></div>`, count))
				target := incrementalHTMLNode(b, world, owner, "target")
				for _, class := range []string{"changed", "", "changed", ""} {
					target.SetAttribute("class", class)
					incrementalSync(b, world)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if i%2 == 0 {
						target.SetAttribute("class", "changed")
					} else {
						target.SetAttribute("class", "")
					}
					incrementalSync(b, world)
				}
			})
		}
	}
}
