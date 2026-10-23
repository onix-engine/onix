package html

import (
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/internal/dom"
	"github.com/diiyw/onix/internal/style"
	"github.com/diiyw/onix/scene"
)

func htmlAnimationRole(t *testing.T, world *ecs.World, owner ecs.Entity, id string, role htmlRenderRole) ecs.Entity {
	t.Helper()
	node := incrementalHTMLNode(t, world, owner, id)
	state, _ := ecs.Get[htmlRenderState](world, owner)
	if rendered := state.RenderNodes[node.node]; rendered != nil {
		for _, entity := range rendered.Entities {
			if child := ecs.GetRef[htmlChild](world, entity); (child != nil) && child.role == role {
				return entity
			}
		}
	}
	t.Fatalf("missing role %d for %s", role, id)
	return 0
}

func TestHTMLCSSColorAnimationRetainsTransparentPaint(t *testing.T) {
	world, owner, clock := htmlAnimationFixture(t, `<style>
	#item {width:30px;height:20px;background:transparent;border:2px solid transparent;animation:paint 1s linear infinite alternate}
	@keyframes paint {from {background:transparent;border-color:transparent} to {background:red;border-top-color:blue;border-right-color:red}}
	</style><div id="item"></div>`)
	background := htmlAnimationRole(t, world, owner, "item", htmlRoleSurface)
	top := htmlAnimationRole(t, world, owner, "item", htmlRoleBorderTop)
	right := htmlAnimationRole(t, world, owner, "item", htmlRoleBorderRight)
	initial, _ := ecs.Get[htmlRenderState](world, owner)
	children := slices.Clone(initial.Children)
	node := incrementalHTMLNode(t, world, owner, "item")
	computed := initial.Styles[node.node]
	htmlAnimationStep(t, world, clock, 500*time.Millisecond)
	fill, _ := ecs.Get[scene.Sprite2D](world, background)
	borderTop, _ := ecs.Get[scene.Sprite2D](world, top)
	borderRight, _ := ecs.Get[scene.Sprite2D](world, right)
	if fill.Shape.Fill != (graphics.ColorRGBA{R: 1, A: .5}) || borderTop.Shape.Fill != (graphics.ColorRGBA{B: 1, A: .5}) || borderRight.Shape.Fill != (graphics.ColorRGBA{R: 1, A: .5}) {
		t.Fatalf("premultiplied color animation: fill=%v top=%v right=%v", fill.Shape.Fill, borderTop.Shape.Fill, borderRight.Shape.Fill)
	}
	after, _ := ecs.Get[htmlRenderState](world, owner)
	if after.Styles[node.node] != computed || computed.BackgroundColor.Color.A != 0 || initial.Boxes != after.Boxes || !slices.Equal(children, after.Children) || after.SyncStats.Layouts != 0 || after.SyncStats.Spawned != 0 {
		t.Fatalf("paint animation mutated styles/rebuilt geometry: %+v", after.SyncStats)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		clock.Set(16*time.Millisecond, clock.Elapsed+16*time.Millisecond, clock.Frame+1)
		if err := Sync(world); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("paint animation allocated %v objects", allocations)
	}
}

func TestHTMLCSSAnimatedColorInheritanceAndCurrentColor(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<style>
	#parent {color:red;animation:ink 1s linear both}
	span {display:inline-block;background:currentColor;border:1px solid currentColor;text-decoration-line:line-through}
	#fixed {color:red} #current {color:currentColor}
	@keyframes ink {to{color:blue}}
	</style><div id="parent"><span id="inherited">inherit</span><span id="fixed">fixed</span><span id="current">current</span></div>`)
	clock := scene.NewTime()
	ecs.InsertResource(world, clock)
	incrementalSync(t, world)
	htmlAnimationStep(t, world, clock, 500*time.Millisecond)
	for _, c := range []struct {
		id   string
		want graphics.ColorRGBA
	}{
		{"inherited", graphics.ColorRGBA{R: .5, B: .5, A: 1}},
		{"fixed", graphics.ColorRGBA{R: 1, A: 1}},
		{"current", graphics.ColorRGBA{R: .5, B: .5, A: 1}},
	} {
		node := incrementalHTMLNode(t, world, owner, c.id)
		texts, decorations, surfaces := 0, 0, 0
		for _, access := range ecs.Query2[htmlChild, scene.Text2D](world).All {
			child := access.A.Ref()
			text := access.B.Ref()

			if child.Node != nil && child.Node.Type == dom.TextNode && child.Node.Parent == node.node {
				texts++
				if text.Color != c.want {
					t.Errorf("%s text color=%v; want %v", c.id, text.Color, c.want)
				}
			}
		}
		for _, access := range ecs.Query2[htmlChild, scene.Sprite2D](world).All {
			child := access.A.Ref()
			sprite := access.B.Ref()

			if child.Node == node.node || child.Node != nil && child.Node.Parent == node.node {
				if child.role == htmlRoleDecoration {
					decorations++
					if sprite.Shape.Fill != c.want {
						t.Errorf("%s decoration color=%v", c.id, sprite.Shape.Fill)
					}
				} else if child.role == htmlRoleSurface {
					surfaces++
					if sprite.Shape.Fill != c.want {
						t.Errorf("%s currentColor background=%v", c.id, sprite.Shape.Fill)
					}
				}
			}
		}
		if texts == 0 || decorations == 0 || surfaces == 0 {
			t.Fatalf("%s missing text/decoration/background: %d/%d/%d", c.id, texts, decorations, surfaces)
		}
	}
	if allocations := testing.AllocsPerRun(100, func() {
		clock.Set(time.Millisecond, clock.Elapsed+time.Millisecond, clock.Frame+1)
		if err := Sync(world); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("inherited color frame allocated %v objects", allocations)
	}
}

func TestHTMLCSSColorTransitionReversesAndCancels(t *testing.T) {
	world, owner, clock := htmlAnimationFixture(t, `<style>
	#item {width:30px;height:20px;background:red;border:2px solid red;transition:background 1s linear, border 1s linear, transform 1s linear, opacity 1s linear}
	#item:hover {background:blue;border-color:blue;transform:translateX(100px);opacity:.5}
	</style><div id="item"></div>`)
	visual := htmlAnimationRole(t, world, owner, "item", htmlRoleSurface)
	SetVisualHover(world, visual, true)
	htmlAnimationStep(t, world, clock, 400*time.Millisecond)
	value, _ := ecs.Get[scene.Sprite2D](world, visual)
	if value.Shape.Fill != (graphics.ColorRGBA{R: .6, B: .4, A: 1}) {
		t.Fatalf("transition color=%v", value.Shape.Fill)
	}
	SetVisualHover(world, visual, false)
	state, _ := ecs.Get[htmlRenderState](world, owner)
	node := incrementalHTMLNode(t, world, owner, "item")
	animation := state.Animations[node.node]
	if math.Abs(animation.Transitions[style.AnimationBackground+2].Config.Duration-.4) > 1e-8 {
		t.Fatalf("reverse duration=%v", animation.Transitions[style.AnimationBackground+2].Config.Duration)
	}
	htmlAnimationStep(t, world, clock, 200*time.Millisecond)
	value, _ = ecs.Get[scene.Sprite2D](world, visual)
	if value.Shape.Fill != (graphics.ColorRGBA{R: .8, B: .2, A: 1}) {
		t.Fatalf("shortened reverse halfway=%v", value.Shape.Fill)
	}
	htmlAnimationStep(t, world, clock, 200*time.Millisecond)
	if Animating(world) {
		t.Fatal("reversed transition did not finish at shortened duration")
	}
	SetVisualHover(world, visual, true)
	htmlAnimationStep(t, world, clock, 250*time.Millisecond)
	node.SetAttribute("style", "transition-property:none")
	incrementalSync(t, world)
	visual = htmlAnimationRole(t, world, owner, "item", htmlRoleSurface)
	value, _ = ecs.Get[scene.Sprite2D](world, visual)
	transform, _ := ecs.Get[scene.UITransform](world, visual)
	state, _ = ecs.Get[htmlRenderState](world, owner)
	current := state.Animations[node.node].Current
	if Animating(world) || current.Colors[style.AnimationBackground].Color() != (style.Color{B: 255, A: 255}) || current.Opacity != .5 || transform.Translation.X.Value != 100 {
		t.Fatalf("removed transition property did not cancel: current=%+v active=%v", current, Animating(world))
	}
}

func TestHTMLCSSTransitionDurationChangeKeepsRunningTransition(t *testing.T) {
	world, owner, clock := htmlAnimationFixture(t, `<style>
	#item {width:20px;height:20px;background:red;transition:opacity 1s linear}
	#item:hover {opacity:0}
	</style><div id="item"></div>`)
	visual := htmlAnimationVisual(t, world, owner, "item")
	SetVisualHover(world, visual, true)
	htmlAnimationStep(t, world, clock, 250*time.Millisecond)
	node := incrementalHTMLNode(t, world, owner, "item")
	node.SetAttribute("style", "transition-duration:0s")
	incrementalSync(t, world)
	htmlAnimationStep(t, world, clock, 250*time.Millisecond)
	visual = htmlAnimationVisual(t, world, owner, "item")
	value, _ := ecs.Get[scene.Sprite2D](world, visual)
	if value.Alpha != .5 || !Animating(world) {
		t.Fatalf("duration-only mutation canceled transition: alpha=%v", value.Alpha)
	}
}

func TestHTMLCSSInheritedColorTransitionUsesDisplayedColor(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<style>
	#parent {color:red} #parent.blue {color:blue}
	#child {transition:color 2s linear}
	</style><div id="parent"><span id="child">text</span></div>`)
	clock := scene.NewTime()
	ecs.InsertResource(world, clock)
	incrementalSync(t, world)
	parent := incrementalHTMLNode(t, world, owner, "parent")
	child := incrementalHTMLNode(t, world, owner, "child")
	parent.SetAttribute("class", "blue")
	incrementalSync(t, world)
	htmlAnimationStep(t, world, clock, time.Second)
	found := false
	for _, access := range ecs.Query2[htmlChild, scene.Text2D](world).All {
		retained := access.A.Ref()
		text := access.B.Ref()

		if retained.Node != nil && retained.Node.Parent == child.node {
			found = true
			if text.Color != (graphics.ColorRGBA{R: .5, B: .5, A: 1}) {
				t.Errorf("inherited transition=%v", text.Color)
			}
		}
	}
	if !found || !Animating(world) {
		t.Fatal("inherited color did not start a transition")
	}
}

func TestHTMLCSSStepsDelayAndReverseFill(t *testing.T) {
	world, owner, clock := htmlAnimationFixture(t, `<style>
	#item {width:20px;height:20px;background:red;animation:move 1s steps(4,start) 1s both}
	@keyframes move {from{transform:translateX(0)}to{transform:translateX(100px)}}
	</style><div id="item"></div>`)
	visual := htmlAnimationVisual(t, world, owner, "item")
	initial, _ := ecs.Get[scene.UITransform](world, visual)
	if initial.Translation.X.Value != 0 {
		t.Fatalf("backwards-filled step jumped during delay: %v", initial.Translation)
	}
	htmlAnimationStep(t, world, clock, time.Second)
	start, _ := ecs.Get[scene.UITransform](world, visual)
	if start.Translation.X.Value != 25 {
		t.Fatalf("step-start failed at active start: %v", start.Translation)
	}
	node := incrementalHTMLNode(t, world, owner, "item")
	node.SetAttribute("style", "animation-direction:reverse")
	incrementalSync(t, world)
	htmlAnimationStep(t, world, clock, time.Second)
	end, _ := ecs.Get[scene.UITransform](world, visual)
	if end.Translation.X.Value != 0 || Animating(world) {
		t.Fatalf("reverse forwards-fill retained first step: %v", end.Translation)
	}
}

func TestHTMLCSSTransitionReverseScalesNegativeDelay(t *testing.T) {
	world, owner, clock := htmlAnimationFixture(t, `<style>
	#item {width:20px;height:20px;background:red;transition:transform 1s linear -.2s}
	#item:hover {transform:translateX(100px)}
	</style><div id="item"></div>`)
	visual := htmlAnimationVisual(t, world, owner, "item")
	SetVisualHover(world, visual, true)
	htmlAnimationStep(t, world, clock, 200*time.Millisecond)
	SetVisualHover(world, visual, false)
	node := incrementalHTMLNode(t, world, owner, "item")
	state, _ := ecs.Get[htmlRenderState](world, owner)
	transition := state.Animations[node.node].Transitions[0]
	if math.Abs(transition.Config.Duration-.4) > 1e-8 || math.Abs(transition.Config.Delay+.08) > 1e-8 {
		t.Fatalf("reverse timing = %+v", transition)
	}
	value, _ := ecs.Get[scene.UITransform](world, visual)
	if math.Abs(float64(value.Translation.X.Value)-32) > 1e-6 {
		t.Fatalf("reverse negative delay progress=%v", value.Translation)
	}
	htmlAnimationStep(t, world, clock, 320*time.Millisecond)
	value, _ = ecs.Get[scene.UITransform](world, visual)
	if value.Translation.X.Value != 0 || Animating(world) {
		t.Fatalf("reverse did not finish: %v", value.Translation)
	}
}

func TestHTMLCSSAnimatedBackdropRemainsUnderTransparentChild(t *testing.T) {
	world, owner, clock := htmlAnimationFixture(t, `<style>
	#parent {width:30px;height:20px;background:red;overflow:hidden;border-radius:4px;animation:ink 1s linear both}
	#child {width:30px;height:20px;background:blue;animation:fade 1s linear both}
	@keyframes ink {to{background:blue}} @keyframes fade {from{opacity:0}to{opacity:1}}
	</style><div id="parent"><div id="child"></div></div>`)
	parent := htmlAnimationRole(t, world, owner, "parent", htmlRoleSurface)
	clip := htmlAnimationRole(t, world, owner, "parent", htmlRoleOverflow)
	initial, _ := ecs.Get[scene.Sprite2D](world, parent)
	if initial.Shape.Fill != (graphics.ColorRGBA{R: 1, A: 1}) {
		t.Fatalf("dynamic child incorrectly suppressed parent backdrop: %v", initial.Shape.Fill)
	}
	htmlAnimationStep(t, world, clock, 500*time.Millisecond)
	shape, _ := ecs.Get[scene.Sprite2D](world, parent)
	mask, _ := ecs.Get[scene.RoundedClip2D](world, clip)
	want := graphics.ColorRGBA{R: .5, B: .5, A: 1}
	if shape.Shape.Fill != want || mask.MaskColor != want {
		t.Fatalf("background/rounded mask did not animate together: %v / %v", shape.Shape.Fill, mask.MaskColor)
	}
}

func TestHTMLCSSAnimatedRasterPaintReusesTexture(t *testing.T) {
	world, owner, clock := htmlAnimationFixture(t, `<style>
	#item {width:30px;height:20px;background:transparent;border:2px solid red;border-radius:6px;animation:paint 1s linear infinite alternate}
	@keyframes paint {from{background:transparent;border-top-color:red} to{background:blue;border-top-color:blue}}
	</style><div id="item"></div>`)
	visual := htmlAnimationRole(t, world, owner, "item", htmlRoleSurface)
	initial, _ := ecs.Get[scene.Sprite2D](world, visual)
	if initial.Texture == nil {
		t.Fatal("rounded animated borders need a retained raster")
	}
	htmlAnimationStep(t, world, clock, 500*time.Millisecond)
	value, _ := ecs.Get[scene.Sprite2D](world, visual)
	state, _ := ecs.Get[htmlRenderState](world, owner)
	node := incrementalHTMLNode(t, world, owner, "item")
	raster := state.Textures.paint[htmlTextureKey{node: node.node, variant: "background"}]
	if value.Texture != initial.Texture || raster.image.RGBAAt(15, 10).B != 255 || raster.image.RGBAAt(15, 10).A != 128 || raster.image.RGBAAt(15, 0).R != 128 || raster.image.RGBAAt(15, 0).B != 128 {
		t.Fatalf("animated raster stale: center=%v border=%v texture reused=%v", raster.image.RGBAAt(15, 10), raster.image.RGBAAt(15, 0), value.Texture == initial.Texture)
	}
	if state.Styles[node.node].BackgroundColor.Color.A != 0 {
		t.Fatal("animated raster mutated interned style")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		clock.Set(16*time.Millisecond, clock.Elapsed+16*time.Millisecond, clock.Frame+1)
		if err := Sync(world); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("raster paint frame allocated %v objects", allocations)
	}
}

func BenchmarkHTMLCSSColorAnimationFrame(b *testing.B) {
	world := ecs.NewWorld()
	clock := scene.NewTime()
	ecs.InsertResource(world, clock)
	value := New(`<style>.item {width:10px;height:10px;background:transparent;border:1px solid transparent;animation:paint 1s linear infinite alternate}@keyframes paint {to{background:red;border-top-color:blue}}</style>` + strings.Repeat(`<div class="item"></div>`, 100))
	value.Width, value.Height = 240, 1600
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, value)
	world.Spawn(bundle)
	incrementalSync(b, world)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		clock.Set(16*time.Millisecond, clock.Elapsed+16*time.Millisecond, clock.Frame+1)
		if err := Sync(world); err != nil {
			b.Fatal(err)
		}
	}
}
