package html

import (
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/html/style"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func htmlAnimationFixture(t *testing.T, source string) (*ecs.World, ecs.Entity, *scene.Time) {
	t.Helper()
	world := ecs.NewWorld()
	clock := scene.NewTime()
	ecs.InsertResource(world, clock)
	value := New(source)
	value.Width, value.Height = 240, 160
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, value)
	owner := world.Spawn(bundle)
	incrementalSync(t, world)
	return world, owner, clock
}

func htmlAnimationVisual(t *testing.T, world *ecs.World, owner ecs.Entity, id string) ecs.Entity {
	t.Helper()
	node := incrementalHTMLNode(t, world, owner, id)
	state, _ := ecs.Get[htmlRenderState](world, owner)
	rendered := state.RenderNodes[node.node]
	if rendered != nil {
		for _, entity := range rendered.Entities {
			if ecs.Has[scene.Sprite2D](world, entity) {
				return entity
			}
		}
	}
	t.Fatalf("no visual for %q", id)
	return 0
}

func htmlAnimationStep(t *testing.T, world *ecs.World, clock *scene.Time, delta time.Duration) {
	t.Helper()
	clock.Set(delta, clock.Elapsed+delta, clock.Frame+1)
	incrementalSync(t, world)
}

func TestHTMLCSSKeyframesAdvanceRetainedVisualsWithoutAllocations(t *testing.T) {
	world, owner, clock := htmlAnimationFixture(t, `<style>
 #spin {width:20px;height:20px;background:red;animation:spin 1s linear infinite}
 @keyframes spin {from{transform:rotate(0deg)}to{transform:rotate(360deg)}}
 </style><div id="spin"></div>`)
	visual := htmlAnimationVisual(t, world, owner, "spin")
	before, _ := ecs.Get[htmlRenderState](world, owner)
	children := slices.Clone(before.Children)
	if !Animating(world) {
		t.Fatal("CSS animation did not keep frame scheduling active")
	}
	htmlAnimationStep(t, world, clock, 250*time.Millisecond)
	transform, _ := ecs.Get[scene.UITransform](world, visual)
	if math.Abs(float64(transform.Rotation)-math.Pi/2) > 1e-6 {
		t.Fatalf("quarter turn = %v", transform.Rotation)
	}
	incrementalSync(t, world)
	same, _ := ecs.Get[scene.UITransform](world, visual)
	if same != transform {
		t.Fatal("same-frame sync advanced animation")
	}
	after, _ := ecs.Get[htmlRenderState](world, owner)
	if before.Boxes != after.Boxes || !slices.Equal(children, after.Children) || after.SyncStats.Layouts != 0 || after.SyncStats.Spawned != 0 || after.SyncStats.Styled != 0 {
		t.Fatalf("animation rebuilt HTML: %+v", after.SyncStats)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		clock.Set(16*time.Millisecond, clock.Elapsed+16*time.Millisecond, clock.Frame+1)
		if err := Sync(world); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("animation frame allocated %v objects", allocations)
	}
}

func TestHTMLCSSAnimationPauseMutationAndRestart(t *testing.T) {
	world, owner, clock := htmlAnimationFixture(t, `<style>
 #item {width:20px;height:20px;background:red;animation:move 1s linear forwards}
 .wide {width:40px !important}
 @keyframes move {to{transform:translateX(100px)}}
 </style><div id="item"></div>`)
	visual := htmlAnimationVisual(t, world, owner, "item")
	node := incrementalHTMLNode(t, world, owner, "item")
	htmlAnimationStep(t, world, clock, 250*time.Millisecond)
	initial, _ := ecs.Get[scene.UITransform](world, visual)
	node.SetAttribute("class", "wide")
	incrementalSync(t, world)
	mutated, _ := ecs.Get[scene.UITransform](world, visual)
	if initial.Translation != mutated.Translation {
		t.Fatalf("layout mutation restarted animation: %v -> %v", initial.Translation, mutated.Translation)
	}
	node.SetAttribute("style", "animation-play-state:paused")
	incrementalSync(t, world)
	if Animating(world) {
		t.Fatal("paused animation keeps scheduling frames")
	}
	htmlAnimationStep(t, world, clock, 500*time.Millisecond)
	paused, _ := ecs.Get[scene.UITransform](world, visual)
	if paused.Translation.X.Value != 25 {
		t.Fatalf("paused translation = %v", paused.Translation)
	}
	node.SetAttribute("style", "animation-play-state:running")
	incrementalSync(t, world)
	htmlAnimationStep(t, world, clock, 250*time.Millisecond)
	resumed, _ := ecs.Get[scene.UITransform](world, visual)
	if resumed.Translation.X.Value != 50 {
		t.Fatalf("resumed translation = %v", resumed.Translation)
	}
	htmlAnimationStep(t, world, clock, 500*time.Millisecond)
	finished, _ := ecs.Get[scene.UITransform](world, visual)
	if finished.Translation.X.Value != 100 || Animating(world) {
		t.Fatalf("forwards end = %v; active=%v", finished.Translation, Animating(world))
	}
	node.SetAttribute("style", "animation:none")
	incrementalSync(t, world)
	removed, _ := ecs.Get[scene.UITransform](world, visual)
	if removed.Translation.X.Value != 0 {
		t.Fatalf("removed animation retained fill: %v", removed.Translation)
	}
	node.SetAttribute("style", "")
	incrementalSync(t, world)
	htmlAnimationStep(t, world, clock, 250*time.Millisecond)
	restarted, _ := ecs.Get[scene.UITransform](world, visual)
	if restarted.Translation.X.Value != 25 {
		t.Fatalf("restart translation = %v", restarted.Translation)
	}
}

func TestHTMLCSSAnimationTransformsDescendantsOfUnpaintedParent(t *testing.T) {
	world, owner, clock := htmlAnimationFixture(t, `<style>
 #parent {width:80px;height:40px;animation:move 1s linear forwards}
 #child {width:20px;height:20px;background:red;transform:translateY(5px)}
 @keyframes move {to{transform:translateX(100px)}}
 </style><div id="parent"><div id="child"></div></div>`)
	visual := htmlAnimationVisual(t, world, owner, "child")
	initial, _ := ecs.Get[scene.UITransform](world, visual)
	htmlAnimationStep(t, world, clock, 500*time.Millisecond)
	moved, _ := ecs.Get[scene.UITransform](world, visual)
	if moved.Translation.X.Value != initial.Translation.X.Value+50 || moved.Translation.Y.Value != initial.Translation.Y.Value {
		t.Fatalf("parent animation did not move child: %+v -> %+v", initial, moved)
	}
}

func TestHTMLCSSOpacityAnimationReusesCompositeTexture(t *testing.T) {
	world, owner, clock := htmlAnimationFixture(t, `<style>
 #group {width:40px;height:20px;background:red;animation:fade 1s linear forwards}
 #child {width:20px;height:20px;background:blue}
 @keyframes fade {from{opacity:0}to{opacity:1}}
 </style><div id="group"><div id="child"></div></div>`)
	visual := htmlAnimationVisual(t, world, owner, "group")
	initial, _ := ecs.Get[scene.Sprite2D](world, visual)
	if !ecs.Has[htmlOpacitySurface](world, visual) || initial.Alpha != 0 || initial.Texture == nil {
		t.Fatalf("opacity group = %+v", initial)
	}
	htmlAnimationStep(t, world, clock, 500*time.Millisecond)
	halfway, _ := ecs.Get[scene.Sprite2D](world, visual)
	if halfway.Alpha != .5 || halfway.Texture != initial.Texture {
		t.Fatalf("animated alpha rerasterized: %+v", halfway)
	}
	state, _ := ecs.Get[htmlRenderState](world, owner)
	group := incrementalHTMLNode(t, world, owner, "group")
	raster := state.Textures.rasters[group.node]
	if pixel := raster.image.RGBAAt(5, 5); pixel.A != 255 || pixel.B != 255 {
		t.Fatalf("opacity was baked before overlapping content: %v", pixel)
	}
	if allocations := testing.AllocsPerRun(50, func() {
		clock.Set(time.Millisecond, clock.Elapsed+time.Millisecond, clock.Frame+1)
		if err := Sync(world); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("opacity animation frame allocated %v objects", allocations)
	}
}

func TestHTMLCSSTransitionInterpolatesTranslationScaleAndOpacity(t *testing.T) {
	world, owner, clock := htmlAnimationFixture(t, `<style>
 #item {width:20px;height:20px;background:red;transform-origin:left top;
 transition-property:transform,opacity;transition-duration:1s;transition-timing-function:linear}
 #item:hover {transform:translate(100px,40px) rotate(90deg) scale(2);opacity:.5}
 </style><div id="item"></div>`)
	visual := htmlAnimationVisual(t, world, owner, "item")
	initial, _ := ecs.Get[scene.UITransform](world, visual)
	if !SetVisualHover(world, visual, true) {
		t.Fatal("hover failed")
	}
	htmlAnimationStep(t, world, clock, 500*time.Millisecond)
	halfway, _ := ecs.Get[scene.UITransform](world, visual)
	sprite, _ := ecs.Get[scene.Sprite2D](world, visual)
	if halfway.Translation.X.Value != initial.Translation.X.Value+50 || halfway.Translation.Y.Value != initial.Translation.Y.Value+20 || halfway.Scale != (mgl32.Vec2{1.5, 1.5}) || math.Abs(float64(halfway.Rotation)-math.Pi/4) > 1e-6 || sprite.Alpha != .75 {
		t.Fatalf("halfway transition = %+v; opacity=%v", halfway, sprite.Alpha)
	}
}

func TestHTMLCSSAnimationZeroScaleAndImportant(t *testing.T) {
	world, owner, clock := htmlAnimationFixture(t, `<style>
 #item {width:20px;height:20px;background:red;opacity:.8 !important;animation:shrink 1s linear forwards}
 @keyframes shrink {from{transform:scale(1);opacity:1}to{transform:scale(0);opacity:0}}
 </style><div id="item"></div>`)
	htmlAnimationStep(t, world, clock, time.Second)
	visual := htmlAnimationVisual(t, world, owner, "item")
	transform, _ := ecs.Get[scene.UITransform](world, visual)
	matrix := transform.LocalMatrix(scene.UIResolveContext{})
	if matrix[0] != 0 || matrix[5] != 0 {
		t.Fatalf("scale(0) rendered full size: %v", matrix)
	}
	state, _ := ecs.Get[htmlRenderState](world, owner)
	item := incrementalHTMLNode(t, world, owner, "item")
	if state.Animations[item.node].Current.Opacity != .8 || state.Styles[item.node].AnimationImportant != style.AnimatedOpacity {
		t.Fatal("animation overrode important opacity")
	}
}

func TestHTMLCSSAnimationHiddenSubtreeAndConditionalKeyframes(t *testing.T) {
	world, owner, clock := htmlAnimationFixture(t, `<style>
 #parent {width:80px;height:40px}
 #child {width:20px;height:20px;background:red;animation:move 1s linear infinite}
 .hidden {display:none}
 @media (min-width:300px) {@keyframes move {to{transform:translateX(100px)}}}
 </style><div id="parent"><div id="child"></div></div>`)
	if Animating(world) {
		t.Fatal("inactive media query started animation")
	}
	value := ecs.GetMut[Surface](world, owner)
	value.Width = 360
	incrementalSync(t, world)
	if !Animating(world) {
		t.Fatal("resize did not resolve conditional keyframes")
	}
	visual := htmlAnimationVisual(t, world, owner, "child")
	htmlAnimationStep(t, world, clock, 250*time.Millisecond)
	moved, _ := ecs.Get[scene.UITransform](world, visual)
	if moved.Translation.X.Value != 25 {
		t.Fatalf("conditional animation = %v", moved.Translation)
	}
	parent := incrementalHTMLNode(t, world, owner, "parent")
	parent.SetAttribute("class", "hidden")
	incrementalSync(t, world)
	if Animating(world) {
		t.Fatal("hidden subtree kept animation alive")
	}
	parent.SetAttribute("class", "")
	incrementalSync(t, world)
	visual = htmlAnimationVisual(t, world, owner, "child")
	restarted, _ := ecs.Get[scene.UITransform](world, visual)
	if restarted.Translation.X.Value != 0 {
		t.Fatalf("showing subtree did not restart: %v", restarted.Translation)
	}
}

func TestHTMLCSSTransformCompositionMatchesAuthoredOrder(t *testing.T) {
	for _, c := range []struct {
		css  string
		want mgl32.Mat4
	}{
		{"rotate(90deg) translateY(100%)", mgl32.HomogRotate3DZ(math.Pi / 2).Mul4(mgl32.Translate3D(0, 50, 0))},
		{"scale(2, 1) rotate(45deg) translateX(10px)", mgl32.Scale3D(2, 1, 1).Mul4(mgl32.HomogRotate3DZ(math.Pi / 4)).Mul4(mgl32.Translate3D(10, 0, 0))},
	} {
		t.Run(c.css, func(t *testing.T) {
			world, owner, _ := htmlAnimationFixture(t, `<div id="item" style="width:100px;height:50px;background:red;transform-origin:0 0;transform:`+c.css+`"></div>`)
			visual := htmlAnimationVisual(t, world, owner, "item")
			transform, _ := ecs.Get[scene.UITransform](world, visual)
			got := transform.LocalMatrix(scene.UIResolveContext{})
			for i, value := range got {
				if math.Abs(float64(value-c.want[i])) > 1e-4 {
					t.Fatalf("transform order:\n%v\nwant\n%v", got, c.want)
				}
			}
		})
	}
	world, owner, _ := htmlAnimationFixture(t, `<div style="width:100px;height:50px;transform:scale(0,1);transform-origin:0 0"><div id="child" style="width:20px;height:20px;background:red;transform:rotate(45deg);transform-origin:0 0"></div></div>`)
	visual := htmlAnimationVisual(t, world, owner, "child")
	transform, _ := ecs.Get[scene.UITransform](world, visual)
	got := transform.LocalMatrix(scene.UIResolveContext{})
	want := mgl32.Scale3D(0, 1, 1).Mul4(mgl32.HomogRotate3DZ(math.Pi / 4))
	for i, value := range got {
		if math.Abs(float64(value-want[i])) > 1e-4 {
			t.Fatalf("singular ancestor composition:\n%v\nwant\n%v", got, want)
		}
	}
}

func BenchmarkHTMLCSSAnimationFrame(b *testing.B) {
	world := ecs.NewWorld()
	clock := scene.NewTime()
	ecs.InsertResource(world, clock)
	value := New(`<style>.item {width:10px;height:10px;background:red;animation:spin 1s linear infinite}@keyframes spin {to{transform:rotate(360deg)}}</style>` + strings.Repeat(`<div class="item"></div>`, 100))
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
