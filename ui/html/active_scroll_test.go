package html

import (
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestHTMLInactiveAnimationsRetainTimeAcrossDurationAndPauseChanges(t *testing.T) {
	world, owner, clock := htmlAnimationFixture(t, `<style>
 div {width:10px;height:10px;background:red}
 #finished {animation:move .25s linear forwards}
 #paused {animation:move 1s linear paused both}
 @keyframes move {to{transform:translateX(100px)}}
 </style><div id="finished"></div><div id="paused"></div>`)
	finished := htmlAnimationVisual(t, world, owner, "finished")
	paused := htmlAnimationVisual(t, world, owner, "paused")
	htmlAnimationStep(t, world, clock, 250*time.Millisecond)
	if got := world.GetRef[scene.UITransform](finished).Translation.X.Value; got != 100 || Animating(world) {
		t.Fatalf("completed frame = %v, animating=%v", got, Animating(world))
	}
	// Other engine work can advance frames while every CSS animation is idle.
	htmlAnimationStep(t, world, clock, 750*time.Millisecond)
	if got := world.GetRef[scene.UITransform](paused).Translation.X.Value; got != 0 {
		t.Fatalf("paused animation advanced: %v", got)
	}
	completed := incrementalHTMLNode(t, world, owner, "finished")
	completed.SetAttribute("style", "animation-duration:2s")
	incrementalSync(t, world)
	if got := world.GetRef[scene.UITransform](finished).Translation.X.Value; got != 50 || !Animating(world) {
		t.Fatalf("extended animation lost original timeline: %v, animating=%v", got, Animating(world))
	}
	stopped := incrementalHTMLNode(t, world, owner, "paused")
	stopped.SetAttribute("style", "animation-play-state:running")
	incrementalSync(t, world)
	htmlAnimationStep(t, world, clock, 250*time.Millisecond)
	if got := world.GetRef[scene.UITransform](paused).Translation.X.Value; got != 25 {
		t.Fatalf("resuming counted paused time: %v", got)
	}
	if got := world.GetRef[scene.UITransform](finished).Translation.X.Value; got != 62.5 {
		t.Fatalf("extended animation did not continue: %v", got)
	}
	completed.Remove()
	incrementalSync(t, world)
	if !Animating(world) {
		t.Fatal("removing one node stopped another active animation")
	}
	world.GetMut[Surface](owner).Value = `<div>replacement</div>`
	incrementalSync(t, world)
	htmlAnimationStep(t, world, clock, time.Second)
	if Animating(world) {
		t.Fatal("document replacement retained active animations")
	}
}

func TestHTMLScrollContentUpdatesOnlyChangedContainers(t *testing.T) {
	world := ecs.NewWorld()
	areas := world.NewSpawner1[scene.UIScroll]()
	left, right := areas.Spawn(scene.UIScroll{}), areas.Spawn(scene.UIScroll{})
	contents := world.NewSpawner2[ScrollContent, scene.UITransform]()
	a := contents.Spawn(ScrollContent{Area: left, X: 10, Y: 20}, scene.NewUITransform())
	b := contents.Spawn(ScrollContent{Area: right, X: 30, Y: 40}, scene.NewUITransform())
	SyncScrollContent(world)
	scroll := world.GetMut[scene.UIScroll](left)
	scroll.Position = mgl32.Vec2{-5, -10}
	SyncScrollContent(world)
	if got := world.GetRef[scene.UITransform](a).Translation; got != scene.Px2(5, 10) {
		t.Fatalf("changed container translation = %v", got)
	}
	if got := world.GetRef[scene.UITransform](b).Translation.Y.Value; got != 40 {
		t.Fatalf("unrelated container moved: %v", got)
	}
	// Input can mutate a retained scroll pointer again within the same pass.
	scroll.Position[1] = -15
	SyncScrollContent(world)
	if got := world.GetRef[scene.UITransform](a).Translation.Y.Value; got != 5 {
		t.Fatalf("same-pass scroll update was lost: %v", got)
	}
	// Re-emitting a retained primitive restores its authored position before
	// scroll synchronization, even though its ScrollContent value is unchanged.
	world.GetMut[scene.UITransform](a).Translation.Y.Value = 20
	SyncScrollContent(world)
	if got := world.GetRef[scene.UITransform](a).Translation.Y.Value; got != 5 {
		t.Fatalf("re-emitted transform lost its scroll offset: %v", got)
	}
	if allocs := testing.AllocsPerRun(100, func() {
		scroll.Position[1] = -25 - scroll.Position[1]
		world.GetMut[scene.UITransform](a).Translation.Y.Value = 20
		SyncScrollContent(world)
	}); allocs != 0 {
		t.Fatalf("warm scroll sync allocated %g times", allocs)
	}
}

func TestHTMLScrollContentRefreshesBindingsMembershipAndScopes(t *testing.T) {
	world := ecs.NewWorld()
	areas := world.NewSpawner1[scene.UIScroll]()
	left := areas.Spawn(scene.UIScroll{Position: mgl32.Vec2{0, -10}})
	right := areas.Spawn(scene.UIScroll{Position: mgl32.Vec2{0, -20}})
	contents := world.NewSpawner2[ScrollContent, scene.UITransform]()
	a := contents.Spawn(ScrollContent{Area: left, Y: 100}, scene.NewUITransform())
	b := contents.Spawn(ScrollContent{Area: right, Y: 200}, scene.NewUITransform())
	var first, second ecs.QueryScope
	first.Reset([]ecs.Entity{a})
	second.Reset([]ecs.Entity{b})
	world.SetQueryScope(&first)
	SyncScrollContent(world)
	world.GetMut[scene.UIScroll](left).Position[1] = -30
	world.SetQueryScope(&second)
	SyncScrollContent(world)
	if got := world.GetRef[scene.UITransform](a).Translation.Y.Value; got != 90 {
		t.Fatalf("scroll sync escaped query scope: %v", got)
	}
	world.SetQueryScope(&first)
	SyncScrollContent(world)
	if got := world.GetRef[scene.UITransform](a).Translation.Y.Value; got != 70 {
		t.Fatalf("restored scope lost pending scroll: %v", got)
	}
	// Resetting the same scope must replace its cached consumer list.
	first.Reset([]ecs.Entity{b})
	SyncScrollContent(world)
	content := world.GetMut[ScrollContent](b)
	content.Area, content.Y = left, 300
	SyncScrollContent(world)
	if got := world.GetRef[scene.UITransform](b).Translation.Y.Value; got != 270 {
		t.Fatalf("rebinding was not synchronized: %v", got)
	}
	world.Remove[scene.UITransform](b)
	SyncScrollContent(world)
	world.Insert(b, scene.NewUITransform())
	SyncScrollContent(world)
	if got := world.GetRef[scene.UITransform](b).Translation.Y.Value; got != 270 {
		t.Fatalf("restored transform was not synchronized: %v", got)
	}
	world.SetQueryScope(nil)
	newChild := contents.Spawn(ScrollContent{Area: left, Y: 400}, scene.NewUITransform())
	world.Despawn(a)
	SyncScrollContent(world)
	if got := world.GetRef[scene.UITransform](newChild).Translation.Y.Value; got != 370 {
		t.Fatalf("new scroll consumer was not synchronized: %v", got)
	}
}

func TestHTMLScrollSyncKeepsEnclosingTickGeometryWrites(t *testing.T) {
	world := ecs.NewWorld()
	areas := world.NewSpawner1[scene.UIScroll]()
	area := areas.Spawn(scene.UIScroll{Position: mgl32.Vec2{0, -10}})
	contents := world.NewSpawner2[ScrollContent, scene.UITransform]()
	child := contents.Spawn(ScrollContent{Area: area, Y: 100}, scene.NewUITransform())
	system := ecs.SystemConfig{Name: "scroll_then_reemit", System: func(context *ecs.SystemContext) error {
		SyncScrollContent(context.World)
		context.World.GetMut[scene.UITransform](child).Translation.Y.Value = 100
		return nil
	}}
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	if err := (ecs.SystemConfig{Name: "next_scroll", System: func(context *ecs.SystemContext) error {
		SyncScrollContent(context.World)
		return nil
	}}).Run(world); err != nil {
		t.Fatal(err)
	}
	if got := world.GetRef[scene.UITransform](child).Translation.Y.Value; got != 90 {
		t.Fatalf("missed geometry written after sync in the same tick: %v", got)
	}
}

func TestHTMLScrollbarIndexRefreshesPartsAndScopes(t *testing.T) {
	world := ecs.NewWorld()
	areas := world.NewSpawner1[scene.UIScroll]()
	area := areas.Spawn(scene.UIScroll{
		ViewportSize: mgl32.Vec2{100, 100}, ContentSize: mgl32.Vec2{200, 200},
		Horizontal: true, Vertical: true, ScrollbarSize: 10,
	})
	parts := world.NewSpawner4[ScrollbarPart, scene.UITransform, scene.Size2D, scene.Sprite2D]()
	a := parts.Spawn(ScrollbarPart{Area: area, Axis: ScrollbarVertical, Kind: ScrollbarThumb}, scene.NewUITransform(), scene.Size2D{}, scene.NewSprite2D())
	var scope ecs.QueryScope
	scope.Reset([]ecs.Entity{a})
	world.SetQueryScope(&scope)
	SyncScrollbars(world)
	b := parts.Spawn(ScrollbarPart{Area: area, Axis: ScrollbarHorizontal, Kind: ScrollbarThumb}, scene.NewUITransform(), scene.Size2D{}, scene.NewSprite2D())
	SyncScrollbars(world)
	if got := world.GetRef[scene.Size2D](b); *got != (scene.Size2D{}) {
		t.Fatal("scrollbar sync escaped query scope")
	}
	scope.Reset([]ecs.Entity{b})
	SyncScrollbars(world)
	if got := world.GetRef[scene.Size2D](b); got.Width == 0 || got.Height == 0 {
		t.Fatal("newly scoped scrollbar was not synchronized")
	}
	world.GetMut[ScrollbarPart](b).Kind = ScrollbarTrack
	SyncScrollbars(world)
	if got := world.GetRef[scene.Size2D](b); got.Width != 100 || got.Height != 10 {
		t.Fatalf("changed scrollbar role was not synchronized: %+v", got)
	}
	world.Remove[scene.Sprite2D](b)
	SyncScrollbars(world)
	world.Insert(b, scene.NewSprite2D())
	SyncScrollbars(world)
	if got := world.GetRef[scene.Sprite2D](b); got.Width != 100 || got.Height != 10 {
		t.Fatalf("restored scrollbar sprite was not synchronized: %+v", got)
	}
	world.SetQueryScope(nil)
	world.Insert(area, scene.Size2D{Width: 100, Height: 100})
	SyncScrollbars(world)
	world.Insert(b, scene.ClipExempt2D{ParentOnly: true})
	world.Insert(b, scene.ClipRect2D{})
	world.Insert(b, scene.RoundedClip2D{Radius: 4})
	SyncScrollbars(world)
	if got := world.GetRef[scene.ClipRect2D](b); *got != (scene.ClipRect2D{Y: -100, Width: 100, Height: 100}) {
		t.Fatalf("new gutter clipping was not synchronized: %+v", got)
	}
	world.GetMut[scene.Size2D](area).Height = 120
	SyncScrollbars(world)
	if got := world.GetRef[scene.RoundedClip2D](b); got.Y != -100 || got.Height != 120 || got.Radius != 4 {
		t.Fatalf("resized rounded gutter clipping was not synchronized: %+v", got)
	}
	if allocs := testing.AllocsPerRun(100, func() {
		world.GetMut[scene.UITransform](b).Translation.Y.Value = 0
		SyncScrollbars(world)
	}); allocs != 0 {
		t.Fatalf("warm scrollbar geometry recovery allocated %g times", allocs)
	}
}
