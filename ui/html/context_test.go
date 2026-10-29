package html

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/html/layout"
	"github.com/diiyw/onix/scene"
)

func assertHTMLContextMatchesFresh(t *testing.T, world *ecs.World, owner ecs.Entity, source string) {
	t.Helper()
	assertHTMLPartialMatchesFresh(t, world, owner, source)
	fresh, freshOwner := incrementalHTMLFixture(t, source)
	got, _ := world.Get[htmlRenderState](owner)
	want, _ := fresh.Get[htmlRenderState](freshOwner)
	extents := make(map[*layout.Box]htmlOverflowExtent)
	htmlOverflowContentExtents(got.Boxes, extents)
	for box, expected := range extents {
		if got.OverflowExtents[box] != expected {
			t.Fatalf("cached descendant extent differs for %s", box.Node.Data)
		}
	}
	for node, binding := range got.OverflowBindings {
		id, _ := node.Attr("id")
		freshNode := incrementalHTMLNode(t, fresh, freshOwner, id)
		expected := want.OverflowBindings[freshNode.node]
		scroll, hasScroll := world.Get[scene.UIScroll](binding.entity)
		other, otherHasScroll := fresh.Get[scene.UIScroll](expected.entity)
		if hasScroll != otherHasScroll || scroll.ViewportSize != other.ViewportSize || scroll.ContentSize != other.ContentSize || scroll.Horizontal != other.Horizontal || scroll.Vertical != other.Vertical {
			t.Fatalf("scroll geometry differs for %s: got=%+v want=%+v", id, scroll, other)
		}
	}
	for _, entity := range got.Children {
		content, ok := world.Get[ScrollContent](entity)
		if !ok {
			continue
		}
		binding, exists := got.OverflowEntities[content.Area]
		node := htmlEntityNode(world, entity)
		if !exists || got.OverflowRoutes[node].entity != binding.entity {
			t.Fatal("stale scroll route")
		}
		transform, _ := world.Get[scene.UITransform](entity)
		if math.Abs(float64(content.X-transform.Translation.X.Value)) > 1e-5 || math.Abs(float64(content.Y-transform.Translation.Y.Value)) > 1e-5 {
			t.Fatal("scroll content base differs from refreshed geometry")
		}
	}
}

func TestHTMLContextLocalScrollTextAndFragmentChanges(t *testing.T) {
	for _, mode := range []string{"hidden", "auto", "scroll", "clip"} {
		t.Run(mode, func(t *testing.T) {
			source := func(text, extra string) string {
				return `<div id="scroller" style="height:40px;overflow:` + mode + `"><div id="target" style="width:50px;` + extra + `">` + text + `</div><div style="height:100px">Following</div></div><div>Unchanged</div>`
			}
			world, owner := incrementalHTMLFixture(t, source("Before", ""))
			target := incrementalHTMLNode(t, world, owner, "target")
			scroller := incrementalHTMLNode(t, world, owner, "scroller")
			state, _ := world.Get[htmlRenderState](owner)
			clip := state.OverflowBindings[scroller.node].entity
			if scroll := world.GetMut[scene.UIScroll](clip); scroll != nil {
				scroll.Position[1] = -20
			}
			for _, step := range []struct{ text, extra string }{{"Longer wrapping text", ""}, {"Longer wrapping text", "background:red;text-decoration:line-through"}, {" ", ""}, {"After", ""}} {
				target.SetText(step.text)
				target.SetAttribute("style", "width:50px;"+step.extra)
				incrementalSync(t, world)
				state, _ = world.Get[htmlRenderState](owner)
				if state.SyncStats.PartialReconciles != 1 || state.OverflowBindings[scroller.node].entity != clip {
					t.Fatalf("stable clip fell back or was replaced: %+v", state.SyncStats)
				}
				if scroll, ok := world.Get[scene.UIScroll](clip); ok && scroll.Position[1] != -20 {
					t.Fatal("scroll position was not preserved")
				}
				assertHTMLContextMatchesFresh(t, world, owner, source(step.text, step.extra))
			}
		})
	}
}

func TestHTMLContextNestedClipResizeAndScrollClamp(t *testing.T) {
	source := func(height int) string {
		return fmt.Sprintf(`<div id="outer" style="height:60px;overflow:auto"><div id="inner" style="height:80px;overflow:auto"><div id="target" style="height:%dpx">Content</div></div></div><div>Unchanged</div>`, height)
	}
	world, owner := incrementalHTMLFixture(t, source(200))
	target := incrementalHTMLNode(t, world, owner, "target")
	inner := incrementalHTMLNode(t, world, owner, "inner")
	state, _ := world.Get[htmlRenderState](owner)
	clip := state.OverflowBindings[inner.node].entity
	scroll := world.GetMut[scene.UIScroll](clip)
	scroll.Position[1] = -100
	for _, height := range []int{140, 30, 200} {
		target.SetAttribute("style", fmt.Sprintf("height:%dpx", height))
		incrementalSync(t, world)
		state, _ = world.Get[htmlRenderState](owner)
		if state.SyncStats.PartialReconciles != 1 || state.SyncStats.UpdatedClips != 1 {
			t.Fatalf("nested clip update work: %+v", state.SyncStats)
		}
		assertHTMLContextMatchesFresh(t, world, owner, source(height))
		scroll = world.GetMut[scene.UIScroll](clip)
		if height == 140 && scroll.Position[1] != -60 || height <= 80 && scroll.Position[1] != 0 {
			t.Fatalf("scroll not clamped after content shrink: %+v", scroll)
		}
	}
	inner.SetAttribute("style", "height:100px;overflow:auto")
	incrementalSync(t, world)
	state, _ = world.Get[htmlRenderState](owner)
	if state.SyncStats.PartialReconciles != 1 {
		t.Fatal("clip viewport resize fell back")
	}
	assertHTMLContextMatchesFresh(t, world, owner, strings.Replace(source(200), "height:80px", "height:100px", 1))
}

func TestHTMLContextNestedOverflowDoesNotExpandAncestorScroll(t *testing.T) {
	source := func(mode string, width int) string {
		return fmt.Sprintf(`<div id="outer" style="width:220px;height:120px;overflow:auto"><div id="inner" style="position:relative;width:100px;height:50px;margin:10px;border:2px solid black;overflow:%s"><span id="guide" style="position:absolute;left:100px;top:50px;width:%dpx;height:130px;transform:rotate(150deg)"></span></div></div>`, mode, width)
	}
	world, owner := incrementalHTMLFixture(t, source("visible", 180))
	inner := incrementalHTMLNode(t, world, owner, "inner")
	guide := incrementalHTMLNode(t, world, owner, "guide")
	outer := incrementalHTMLNode(t, world, owner, "outer")
	for _, mode := range []string{"hidden", "clip", "auto", "scroll", "visible", "hidden"} {
		inner.SetAttribute("style", "position:relative;width:100px;height:50px;margin:10px;border:2px solid black;overflow:"+mode)
		for _, width := range []int{180, 220} {
			guide.SetAttribute("style", fmt.Sprintf("position:absolute;left:100px;top:50px;width:%dpx;height:130px;transform:rotate(150deg)", width))
			incrementalSync(t, world)
			state, _ := world.Get[htmlRenderState](owner)
			scroll, _ := world.Get[scene.UIScroll](state.OverflowBindings[outer.node].entity)
			if mode != "visible" && (scroll.Horizontal || scroll.Vertical || scroll.ContentSize != scroll.ViewportSize) {
				t.Fatalf("%s leaked clipped descendant overflow to its ancestor: %+v", mode, scroll)
			}
			if mode == "visible" && (!scroll.Horizontal || !scroll.Vertical) {
				t.Fatalf("visible overflow was not propagated: %+v", scroll)
			}
			if extent := state.OverflowExtents[state.BoxRenderings[inner.node].Box]; extent.width < float64(100+width) || extent.height < 180 {
				t.Fatalf("nested box lost its own scrollable content: %+v", extent)
			}
			assertHTMLContextMatchesFresh(t, world, owner, source(mode, width))
		}
	}
}

func TestHTMLContextSkipsUnrelatedClipBranches(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="scroller" style="height:40px;overflow:auto"><div id="target">Before</div></div>`+strings.Repeat(`<div style="height:40px;overflow:auto"><div>Unchanged</div></div>`, 1000))
	target := incrementalHTMLNode(t, world, owner, "target")
	target.SetText("After")
	incrementalSync(t, world)
	state, _ := world.Get[htmlRenderState](owner)
	if state.SyncStats.PartialReconciles != 1 || state.SyncStats.UpdatedClips != 0 || state.SyncStats.RoutedEntities != 1 || state.SyncStats.ExtentBoxes != 0 || state.SyncStats.ReconciledBoxes != 1 {
		t.Fatalf("unrelated contexts were updated: %+v", state.SyncStats)
	}
}

func TestHTMLContextStableEventTargetsRemainLocal(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="scroller" style="height:40px;overflow:auto"><div id="target">Before</div></div><div id="other" style="background:blue">Other</div>`)
	target := incrementalHTMLNode(t, world, owner, "target")
	other := incrementalHTMLNode(t, world, owner, "other")
	clicks, otherClicks := 0, 0
	target.OnClick(func(*PointerEvent) { clicks++ })
	other.OnClick(func(*PointerEvent) { otherClicks++ })
	target.SetText("After")
	incrementalSync(t, world) // Establish the changed event membership once.
	state, _ := world.Get[htmlRenderState](owner)
	unchangedTarget := state.EventTargets[other.node]
	for _, css := range []string{"background:red", "", "width:120px"} {
		target.SetAttribute("style", css)
		target.SetText("Before")
		incrementalSync(t, world)
		state, _ = world.Get[htmlRenderState](owner)
		if state.SyncStats.PartialReconciles != 1 || state.EventTargets[other.node] != unchangedTarget {
			t.Fatal("stable event binding forced a full update")
		}
		scene.DispatchInputEvent(world, scene.InputEvent{Entity: state.EventTargets[target.node], Kind: scene.InputEventTap})
		scene.DispatchInputEvent(world, scene.InputEvent{Entity: unchangedTarget, Kind: scene.InputEventTap})
	}
	if clicks != 3 || otherClicks != 3 {
		t.Fatalf("event callback routing changed: %d, %d", clicks, otherClicks)
	}
}

func BenchmarkHTMLScrollTextMutation(b *testing.B) {
	for _, count := range []int{10, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world, owner := incrementalHTMLFixture(b, `<div id="scroller" style="height:40px;overflow:auto"><div id="target">Before</div></div>`+strings.Repeat(`<div style="height:40px;overflow:auto"><div>Unchanged</div></div>`, count))
			target := incrementalHTMLNode(b, world, owner, "target")
			for _, value := range []string{"After", "Before"} {
				target.SetText(value)
				incrementalSync(b, world)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if i%2 == 0 {
					target.SetText("After")
				} else {
					target.SetText("Before")
				}
				if err := Sync(world); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestHTMLContextMovedClipRefreshesAbsoluteDescendants(t *testing.T) {
	source := func(margin int) string {
		return fmt.Sprintf(`<div style="position:relative;height:200px"><div id="scroller" style="margin-left:%dpx;height:50px;overflow:auto"><div>Normal</div><div style="position:absolute;left:15px;top:10px">Fixed position</div><div id="inner" style="height:20px;overflow:hidden"><div>Nested</div></div></div></div>`, margin)
	}
	world, owner := incrementalHTMLFixture(t, source(0))
	scroller := incrementalHTMLNode(t, world, owner, "scroller")
	for _, margin := range []int{20, 40, 0} {
		scroller.SetAttribute("style", fmt.Sprintf("margin-left:%dpx;height:50px;overflow:auto", margin))
		incrementalSync(t, world)
		state, _ := world.Get[htmlRenderState](owner)
		if state.SyncStats.PartialReconciles != 1 {
			t.Fatalf("moved paint-only clip context fell back: %+v", state.SyncStats)
		}
		assertHTMLContextMatchesFresh(t, world, owner, source(margin))
	}
}

func TestHTMLContextClipTopologyRemainsLocal(t *testing.T) {
	source := func(css string) string {
		return `<div id="scroller" style="` + css + `"><div id="target">Before</div></div>`
	}
	world, owner := incrementalHTMLFixture(t, source("height:40px;overflow:auto"))
	scroller := incrementalHTMLNode(t, world, owner, "scroller")
	target := incrementalHTMLNode(t, world, owner, "target")
	for _, css := range []string{"height:0px;overflow:auto", "height:40px;overflow:auto", "height:40px;overflow:hidden", "height:40px;overflow:visible", "height:40px;overflow:auto"} {
		scroller.SetAttribute("style", css)
		incrementalSync(t, world)
		state, _ := world.Get[htmlRenderState](owner)
		if state.SyncStats.PartialReconciles != 1 {
			t.Fatal("clip membership/mode change used the wrong reconciliation path")
		}
		assertHTMLContextMatchesFresh(t, world, owner, source(css))
	}
	target.SetText("After")
	incrementalSync(t, world)
	state, _ := world.Get[htmlRenderState](owner)
	if state.SyncStats.PartialReconciles != 1 {
		t.Fatal("local updates did not resume after clip topology refresh")
	}
}

func TestHTMLContextWarmScrollTextDoesNotAllocate(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="scroller" style="height:40px;overflow:auto"><div id="target">Before</div></div>`+strings.Repeat(`<div style="height:40px;overflow:auto"><div>Unchanged</div></div>`, 10))
	target := incrementalHTMLNode(t, world, owner, "target")
	target.OnClick(func(*PointerEvent) {})
	update := func() {
		target.SetText("After")
		incrementalSync(t, world)
		target.SetText("Before")
		incrementalSync(t, world)
	}
	for range 10 {
		update()
	}
	if allocations := testing.AllocsPerRun(100, update); allocations != 0 {
		t.Fatalf("warm scroll text update allocated %v times", allocations)
	}
}

func TestHTMLContextPreservesControlWhenMoved(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="scroller" style="height:40px;overflow:auto"><div id="target">Before</div><div><input id="field" value="initial"/></div></div>`)
	target := incrementalHTMLNode(t, world, owner, "target")
	field := incrementalHTMLNode(t, world, owner, "field")
	scroller := incrementalHTMLNode(t, world, owner, "scroller")
	state, _ := world.Get[htmlRenderState](owner)
	var control ecs.Entity
	for key, group := range state.EntityGroups {
		if key.Node == field.node && key.Role == htmlRoleControl {
			control = group.Bindings[0].Entity
			break
		}
	}
	if control == 0 {
		t.Fatal("missing control")
	}
	SetControlValue(world, control, "typed")
	live := world.GetMut[Control](control)
	live.SelectionAnchor, live.SelectionFocus = 1, 3
	revision := live.RenderRevision
	target.SetText("After")
	incrementalSync(t, world)
	state, _ = world.Get[htmlRenderState](owner)
	live = world.GetMut[Control](control)
	if state.SyncStats.PartialReconciles != 1 || live.RenderRevision != revision || live.Value != "typed" || live.SelectionAnchor != 1 || live.SelectionFocus != 3 {
		t.Fatal("local scroll text update rewrote unrelated control")
	}
	scroller.SetAttribute("style", "height:40px;overflow:auto;margin-left:10px")
	incrementalSync(t, world)
	state, _ = world.Get[htmlRenderState](owner)
	live = world.GetMut[Control](control)
	if state.SyncStats.PartialReconciles != 1 || live.Value != "typed" || live.SelectionAnchor != 1 || live.SelectionFocus != 3 {
		t.Fatal("moving control context did not preserve live control state locally")
	}
}

func TestHTMLContextEventTargetDisappearsAndReturns(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="scroller" style="height:40px;overflow:auto"><div id="target">Before</div></div>`)
	target := incrementalHTMLNode(t, world, owner, "target")
	clicks := 0
	target.OnClick(func(*PointerEvent) { clicks++ })
	target.SetText("After")
	incrementalSync(t, world)
	state, _ := world.Get[htmlRenderState](owner)
	old := state.EventTargets[target.node]
	target.SetText("")
	incrementalSync(t, world)
	state, _ = world.Get[htmlRenderState](owner)
	if state.SyncStats.PartialReconciles != 1 || state.EventTargets[target.node] != 0 || world.IsAlive(old) {
		t.Fatal("empty box retained event target")
	}
	target.SetText("Before")
	incrementalSync(t, world)
	state, _ = world.Get[htmlRenderState](owner)
	entity := state.EventTargets[target.node]
	if state.SyncStats.PartialReconciles != 1 || !world.IsAlive(entity) {
		t.Fatal("event target did not return locally")
	}
	scene.DispatchInputEvent(world, scene.InputEvent{Entity: entity, Kind: scene.InputEventTap})
	if clicks != 1 {
		t.Fatal("restored event target lost callback")
	}
}
