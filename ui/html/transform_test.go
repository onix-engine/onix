package html

import (
	"math"
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestHTMLTransformOriginSurvivesContainerPaintUpdates(t *testing.T) {
	containers := []struct{ name, open, close string }{
		{"hidden", `<div style="position:absolute;left:20px;top:40px;width:140px;height:100px;padding:3px;border:2px solid black;overflow:hidden">`, `</div>`},
		{"scroll", `<div style="position:absolute;left:20px;top:40px;width:140px;height:100px;overflow:auto">`, `</div>`},
		{"nested", `<div style="position:absolute;left:20px;top:40px;width:140px;height:100px;overflow:hidden"><div style="position:absolute;left:10px;top:10px;width:80px;height:60px;overflow:hidden">`, `</div></div>`},
		{"control", `<button style="position:absolute;left:20px;top:40px;width:140px;height:100px">`, `</button>`},
	}
	for _, container := range containers {
		for _, inherited := range []bool{false, true} {
			name := "own"
			if inherited {
				name = "inherited"
			}
			t.Run(container.name+"/"+name, func(t *testing.T) {
				visualStyle := "position:absolute;left:10px;top:10px;width:16px;height:16px;background:white"
				open, close := container.open, container.close
				if inherited {
					open += `<div style="position:absolute;left:4px;top:4px;width:36px;height:36px;transform:rotate(-90deg)">`
					close = `</div>` + close
				} else {
					visualStyle += ";transform:rotate(-90deg)"
				}
				world, owner, _ := htmlAnimationFixture(t, open+`<div id="visual" style="`+visualStyle+`"></div>`+close)
				visual := htmlAnimationVisual(t, world, owner, "visual")
				node := incrementalHTMLNode(t, world, owner, "visual")
				bounds, _ := node.Bounds()
				want := mgl32.Vec4{float32(bounds.X + 8), float32(bounds.Y + 8), 0, 1}
				for _, color := range []string{"white", "blue", "red"} {
					node.SetAttribute("style", visualStyle+";color:"+color)
					incrementalSync(t, world)
					if htmlAnimationVisual(t, world, owner, "visual") != visual {
						t.Fatal("paint update replaced the retained visual")
					}
					if err := scene.SyncUIGlobalTransformSystem("").Run(world); err != nil {
						t.Fatal(err)
					}
					global, _ := world.Get[scene.UIGlobalTransform](visual)
					center := global.Matrix.Mul4x1(mgl32.Vec4{8, 8, 0, 1})
					if center.Sub(want).Len() > .001 {
						t.Fatalf("color=%s: center=%v, want %v", color, center, want)
					}
				}
			})
		}
	}
}

func TestHTMLTransformRotatesAroundCenterWithTransition(t *testing.T) {
	world := ecs.NewWorld()
	clock := scene.NewTime()
	world.InsertResource(clock)
	html := New(`<div id="arrow"></div><style>
#arrow { display: block; width: 20px; height: 20px; background: #fff; transform: rotate(0deg); transition: transform 100ms linear; }
#arrow:hover { transform: rotate(90deg); }
</style>`)
	html.Width, html.Height = 100, 60
	bundle := ecs.NewBundle(1)
	bundle.Put(html)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}

	arrow, ok := NodeByID(world, owner, "arrow")
	if !ok {
		t.Fatal("arrow element was not rendered")
	}
	state, ok := world.Get[htmlRenderState](owner)
	if !ok || state.RenderNodes[arrow.node] == nil || len(state.RenderNodes[arrow.node].Entities) == 0 {
		t.Fatal("arrow has no retained visual")
	}
	visual := state.RenderNodes[arrow.node].Entities[0]
	transform, ok := world.Get[scene.UITransform](visual)
	if !ok || math.Abs(float64(transform.Rotation)) > 1e-6 {
		t.Fatalf("initial arrow rotation = %+v, want 0", transform)
	}

	if !SetVisualHover(world, visual, true) {
		t.Fatal("hover state was not applied")
	}
	transform, _ = world.Get[scene.UITransform](visual)
	if math.Abs(float64(transform.Pivot.X()-10)) > 1e-5 || math.Abs(float64(transform.Pivot.Y()-10)) > 1e-5 {
		t.Fatalf("arrow transform pivot = %v, want center (10, 10)", transform.Pivot)
	}
	clock.Set(50*time.Millisecond, 50*time.Millisecond, 1)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	transform, _ = world.Get[scene.UITransform](visual)
	if math.Abs(float64(transform.Rotation)-math.Pi/4) > 1e-5 {
		t.Fatalf("halfway arrow rotation = %v, want %v radians", transform.Rotation, math.Pi/4)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	transform, _ = world.Get[scene.UITransform](visual)
	if math.Abs(float64(transform.Rotation)-math.Pi/4) > 1e-5 {
		t.Fatalf("repeated same-frame sync advanced rotation to %v, want %v radians", transform.Rotation, math.Pi/4)
	}

	clock.Set(50*time.Millisecond, 100*time.Millisecond, 2)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	transform, _ = world.Get[scene.UITransform](visual)
	if math.Abs(float64(transform.Rotation)-math.Pi/2) > 1e-5 {
		t.Fatalf("final arrow rotation = %v, want %v radians", transform.Rotation, math.Pi/2)
	}
}

func TestHTMLActiveTranslateYUpdatesAndRestoresControl(t *testing.T) {
	world := ecs.NewWorld()
	html := New(`<button id="open">打开项目目录</button><style>
#open { display: block; width: 120px; height: 24px; transform: translateY(0); }
#open:active { transform: translateY(1px); }
</style>`)
	html.Width, html.Height = 180, 60
	bundle := ecs.NewBundle(1)
	bundle.Put(html)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}

	button, ok := NodeByID(world, owner, "open")
	if !ok {
		t.Fatal("button was not rendered")
	}
	state, ok := world.Get[htmlRenderState](owner)
	if !ok {
		t.Fatal("missing HTML render state")
	}
	var control ecs.Entity
	for _, entity := range state.RenderNodes[button.node].Entities {
		if world.Has[Control](entity) {
			control = entity
			break
		}
	}
	if control == 0 {
		t.Fatal("button control was not retained")
	}
	transform, _ := world.Get[scene.UITransform](control)
	initialY := transform.Translation.Y.Value
	if !SetControlActive(world, control, true) {
		t.Fatal("active state was not applied")
	}
	transform, _ = world.Get[scene.UITransform](control)
	if got := transform.Translation.Y.Value; got != initialY+1 {
		t.Fatalf("active translation y = %v, want %v", got, initialY+1)
	}
	if !SetControlActive(world, control, false) {
		t.Fatal("active state was not cleared")
	}
	transform, _ = world.Get[scene.UITransform](control)
	if got := transform.Translation.Y.Value; got != initialY {
		t.Fatalf("released translation y = %v, want %v", got, initialY)
	}
	for range 5 {
		SetControlActive(world, control, true)
		SetControlActive(world, control, false)
	}
	// Interning allocates one instance per new distinct style, so each
	// activate/release direction costs one.
	if allocations := testing.AllocsPerRun(100, func() {
		SetControlActive(world, control, true)
		SetControlActive(world, control, false)
	}); allocations > 2 {
		t.Fatalf("active transform allocated %v objects", allocations)
	}
}
