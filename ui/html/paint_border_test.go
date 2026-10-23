package html

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/html/layout"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
)

func TestHTMLRectangularBorderEdgesAlignDuringColorTransition(t *testing.T) {
	// Match the Color transition reversal case in examples/animations.
	world, owner, clock := htmlAnimationFixture(t, `<style>
	#box {margin:12px;width:150px;height:44px;border:3px solid #267d50;background:white;transition:background-color 1s linear,border-color 1s linear}
	#box:hover {background:#c7383b;border-color:#234d95}
	</style><div id="box"></div>`)
	defer Dispose(world)
	node := incrementalHTMLNode(t, world, owner, "box")
	visual := htmlAnimationRole(t, world, owner, "box", htmlRoleSurface)
	var edges [4]ecs.Entity
	for i := range edges {
		edges[i] = htmlAnimationRole(t, world, owner, "box", htmlRoleBorderTop+htmlRenderRole(i))
	}
	check := func(t *testing.T) {
		t.Helper()
		state := ecs.GetRef[htmlRenderState](world, owner)
		box := state.BoxesByNode[node.node]
		r := box.BorderBox()
		want := [...]layout.Rect{
			{X: r.X, Y: r.Y, Width: r.Width, Height: box.Border.Top},
			{X: r.X + r.Width - box.Border.Right, Y: r.Y, Width: box.Border.Right, Height: r.Height},
			{X: r.X, Y: r.Y + r.Height - box.Border.Bottom, Width: r.Width, Height: box.Border.Bottom},
			{X: r.X, Y: r.Y, Width: box.Border.Left, Height: r.Height},
		}
		for i, entity := range edges {
			if htmlAnimationRole(t, world, owner, "box", htmlRoleBorderTop+htmlRenderRole(i)) != entity {
				t.Fatal("color transition replaced a border edge")
			}
			transform := ecs.GetRef[scene.UITransform](world, entity)
			sprite := ecs.GetRef[scene.Sprite2D](world, entity)
			got := layout.Rect{X: float64(transform.Translation.X.Value), Y: float64(transform.Translation.Y.Value), Width: float64(sprite.Width), Height: float64(sprite.Height)}
			if got != want[i] {
				t.Errorf("border edge %d = %+v, want %+v", i, got, want[i])
			}
		}
	}
	t.Run("initial", check)
	SetVisualHover(world, visual, true)
	htmlAnimationStep(t, world, clock, 400*time.Millisecond)
	t.Run("hover", check)
	SetVisualHover(world, visual, false)
	htmlAnimationStep(t, world, clock, 200*time.Millisecond)
	t.Run("reversing", check)
	htmlAnimationStep(t, world, clock, 200*time.Millisecond)
	t.Run("returned", check)
}

func TestHTMLTransparentRoundedElbowsCreatePaintSurface(t *testing.T) {
	for _, test := range []struct {
		name, widths, radii string
		arc, cut            image.Point
	}{
		{"bottom-left", "0 0 2px 2px", "0 0 0 9px", image.Pt(2, 4), image.Pt(0, 8)},
		{"top-right", "2px 2px 0 0", "0 9px 0 0", image.Pt(9, 4), image.Pt(11, 0)},
		{"bottom-right", "0 2px 2px 0", "0 0 9px 0", image.Pt(9, 4), image.Pt(11, 8)},
		{"top-left", "2px 0 0 2px", "9px 0 0 0", image.Pt(2, 4), image.Pt(0, 0)},
	} {
		t.Run(test.name, func(t *testing.T) {
			world, owner := incrementalHTMLFixture(t, fmt.Sprintf(`<div style="color:#84ccff;position:relative;width:48px;height:32px"><span id="elbow" style="position:absolute;left:11px;top:16px;box-sizing:border-box;width:12px;height:9px;background:transparent;border:0 solid currentColor;border-width:%s;border-radius:%s"></span></div>`, test.widths, test.radii))
			defer Dispose(world)
			node := incrementalHTMLNode(t, world, owner, "elbow")
			state := ecs.GetRef[htmlRenderState](world, owner)
			paint := state.Textures.paint[htmlTextureKey{node: node.node, variant: "background"}]
			if paint == nil || htmlCullRole(world, owner, node, htmlRoleSurface) == 0 {
				t.Fatal("transparent rounded border was culled before its arc could be painted")
			}
			if pixel := paint.image.RGBAAt(test.arc.X, test.arc.Y); pixel.A == 0 || pixel.R != 132 || pixel.G != 204 || pixel.B != 255 {
				t.Fatalf("missing currentColor arc at %v: %v", test.arc, pixel)
			}
			if paint.image.RGBAAt(test.cut.X, test.cut.Y).A != 0 {
				t.Fatal("rounded border painted outside its contour")
			}
		})
	}
}

func TestHTMLRasterRoundedBorderKeepsCornerArcs(t *testing.T) {
	for _, test := range []struct {
		name, radius string
	}{
		{"toolbar-left", "5px 0 0 5px"},
		{"elliptical-left", "8px 0 0 8px / 5px 0 0 5px"},
	} {
		t.Run(test.name, func(t *testing.T) {
			world, owner := incrementalHTMLFixture(t, `<button id="button" style="width:36px;height:24px;background:transparent;border:1px solid #84ccff;border-radius:`+test.radius+`"></button>`)
			defer Dispose(world)
			node := incrementalHTMLNode(t, world, owner, "button")
			state := ecs.GetRef[htmlRenderState](world, owner)
			computed := state.Styles[node.node]
			var raster image.RGBA
			rasterHTMLBoxPaintPrepared(&raster, computed, 36, 24, 1, nil)
			// These arc pixels lie beyond both one-pixel straight edge strips.
			if raster.RGBAAt(1, 2).A == 0 || raster.RGBAAt(1, 21).A == 0 {
				t.Fatal("rounded border has gaps between its straight edges")
			}
			if raster.RGBAAt(0, 0).A != 0 || raster.RGBAAt(0, 23).A != 0 || raster.RGBAAt(18, 12).A != 0 {
				t.Fatal("border filled outside the outer contour or inside the padding box")
			}
			if raster.RGBAAt(35, 0).A != 255 || raster.RGBAAt(35, 23).A != 255 || raster.RGBAAt(0, 12).A != 255 {
				t.Fatal("rounded border changed the square corners or straight edges")
			}
			partial := 0
			for y := range 12 {
				for x := range 36 {
					pixel := raster.RGBAAt(x, y)
					if pixel != raster.RGBAAt(x, 23-y) {
						t.Fatalf("border is asymmetric at %d,%d", x, y)
					}
					if pixel.A > 0 && pixel.A < 255 {
						partial++
					}
				}
			}
			if partial == 0 {
				t.Fatal("border arcs have no antialiasing")
			}
			if allocations := testing.AllocsPerRun(100, func() {
				rasterHTMLBoxPaintPrepared(&raster, computed, 36, 24, 1, nil)
			}); allocations != 0 {
				t.Fatalf("retained border paint allocated %g times", allocations)
			}
		})
	}
}

func TestHTMLRasterRoundedBorderUsesAdjoiningWidthsAndColors(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="box" style="width:36px;height:24px;background:transparent;border-style:solid;border-width:3px 2px;border-color:red blue;border-radius:8px 0 0 8px"></div>`)
	defer Dispose(world)
	node := incrementalHTMLNode(t, world, owner, "box")
	state := ecs.GetRef[htmlRenderState](world, owner)
	var raster image.RGBA
	rasterHTMLBoxPaintPrepared(&raster, state.Styles[node.node], 36, 24, 1, nil)
	if pixel := raster.RGBAAt(3, 4); pixel.R != 255 || pixel.B != 0 || pixel.A == 0 {
		t.Fatalf("top arc lost its thicker red border: %v", pixel)
	}
	if pixel := raster.RGBAAt(1, 6); pixel.R != 0 || pixel.B != 255 || pixel.A == 0 {
		t.Fatalf("left arc lost its blue border: %v", pixel)
	}
	if raster.RGBAAt(8, 3).A != 0 || raster.RGBAAt(2, 12).A != 0 {
		t.Fatal("border widths extended into the padding box")
	}
}

func TestHTMLRasterRoundedBorderPreservesFractionalWidthAndOpacity(t *testing.T) {
	for _, test := range []struct {
		name, border string
		alpha        uint8
	}{
		{"half-pixel", ".5px solid #84ccff", 128},
		{"translucent", ".5px solid rgba(132,204,255,.5)", 64},
		{"thicker-than-radius", "7px solid #84ccff", 255},
	} {
		t.Run(test.name, func(t *testing.T) {
			world, owner := incrementalHTMLFixture(t, `<button id="button" style="width:36px;height:24px;background:transparent;border:`+test.border+`;border-radius:5px 0 0 5px"></button>`)
			defer Dispose(world)
			node := incrementalHTMLNode(t, world, owner, "button")
			state := ecs.GetRef[htmlRenderState](world, owner)
			var raster image.RGBA
			rasterHTMLBoxPaintPrepared(&raster, state.Styles[node.node], 36, 24, 1, nil)
			for _, point := range []image.Point{{0, 12}, {18, 0}, {35, 12}, {18, 23}} {
				if pixel := raster.RGBAAt(point.X, point.Y); pixel.A != test.alpha {
					t.Fatalf("border coverage at %v = %d, want %d", point, pixel.A, test.alpha)
				}
			}
			if raster.RGBAAt(0, 0).A != 0 || raster.RGBAAt(18, 12).A != 0 {
				t.Fatal("border filled outside its contour")
			}
		})
	}
}

func TestHTMLToolbarRoundedBorderUpdatesRetainedPaint(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<style>button{width:36px;height:24px;background:transparent;border:1px solid transparent;border-radius:5px 0 0 5px}button.active{border-color:#84ccff;background:#3b5266}</style><button id="button" class="active"></button>`)
	defer Dispose(world)
	node := incrementalHTMLNode(t, world, owner, "button")
	state := ecs.GetRef[htmlRenderState](world, owner)
	raster := state.Textures.paint[htmlTextureKey{node: node.node, variant: "background"}]
	if raster == nil {
		t.Fatal("toolbar button has no retained paint")
	}
	texture := raster.texture
	node.SetAttribute("class", "")
	incrementalSync(t, world)
	if raster.image.RGBAAt(1, 2).A != 0 {
		t.Fatal("transparent toolbar border kept its active corner")
	}
	node.SetAttribute("class", "active")
	incrementalSync(t, world)
	if raster.texture != texture {
		t.Fatal("toolbar state change replaced its paint texture")
	}
	if pixel := raster.image.RGBAAt(1, 2); pixel.B <= 102 || pixel.A != 255 {
		t.Fatalf("active toolbar corner only painted its background: %v", pixel)
	}
	if raster.image.RGBAAt(18, 12) != (color.RGBA{R: 59, G: 82, B: 102, A: 255}) {
		t.Fatal("border changed the toolbar background")
	}
	entity := htmlNodeInputEntity(world, ecs.GetRef[htmlRenderState](world, owner), node.node)
	sprite := ecs.GetRef[scene.Sprite2D](world, entity)
	if sprite == nil || sprite.Texture != texture {
		t.Fatal("toolbar button did not retain the corrected paint texture")
	}
}

func TestHTMLRoundedBorderTextureFollowsCanvasDensity(t *testing.T) {
	canvas := graphics.NewCanvas2D(render.ActiveDevice(), 320, 200)
	defer graphics.SetCurrentContextForTesting(canvas)()
	world, owner := incrementalHTMLFixture(t, `<button id="button" style="box-sizing:border-box;padding:0;width:36px;height:24px;background:transparent;border:1px solid #84ccff;border-radius:5px 0 0 5px"></button>`)
	defer Dispose(world)
	node := incrementalHTMLNode(t, world, owner, "button")
	var previous *graphics.Texture
	for _, scale := range []float64{1, 1.25, 2, 3, 2, 1} {
		canvas.SetRasterScale(scale)
		incrementalSync(t, world)
		state := ecs.GetRef[htmlRenderState](world, owner)
		raster := state.Textures.paint[htmlTextureKey{node: node.node, variant: "background"}]
		texture := raster.texture
		if texture.Width != int(math.Ceil(36*scale)) || texture.Height != int(math.Ceil(24*scale)) {
			t.Fatalf("texture at density %g has size %dx%d", scale, texture.Width, texture.Height)
		}
		if texture.LogicalWidth() != 36 || texture.LogicalHeight() != 24 {
			t.Fatal("pixel density changed the texture's logical size")
		}
		entity := htmlNodeInputEntity(world, state, node.node)
		sprite := ecs.GetRef[scene.Sprite2D](world, entity)
		if sprite.Width != 36 || sprite.Height != 24 || sprite.Texture != texture {
			t.Fatal("pixel density changed the button layout or left a stale texture")
		}
		if previous != nil && previous.Valid() {
			t.Fatal("density change did not release the previous texture")
		}
		incrementalSync(t, world)
		if raster.texture != texture {
			t.Fatal("unchanged density replaced the retained texture")
		}
		if allocations := testing.AllocsPerRun(20, func() {
			rasterHTMLBoxPaintPrepared(&raster.image, state.Styles[node.node], 36, 24, scale, nil)
		}); allocations != 0 {
			t.Fatalf("retained paint at density %g allocated %g times", scale, allocations)
		}
		previous = texture
	}
}

func TestHTMLRoundedBorderFilteringPreservesColor(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<button id="button" style="width:36px;height:24px;background:transparent;border:1px solid #84ccff;border-radius:5px 0 0 5px"></button>`)
	defer Dispose(world)
	node := incrementalHTMLNode(t, world, owner, "button")
	state := ecs.GetRef[htmlRenderState](world, owner)
	var raster image.RGBA
	rasterHTMLBoxPaintPrepared(&raster, state.Styles[node.node], 36, 24, 2, nil)
	// Linear filtering must only change coverage; transparent neighbors must
	// not add black to the blue outline, on either side of the border.
	for _, point := range []image.Point{{0, 0}, {0, 24}, {2, 24}, {36, 24}} {
		pixel := raster.RGBAAt(point.X, point.Y)
		if pixel.R != 132 || pixel.G != 204 || pixel.B != 255 {
			t.Fatalf("border filtering can darken the outline at %v: %v", point, pixel)
		}
	}
	if raster.RGBAAt(0, 0).A != 0 || raster.RGBAAt(2, 24).A != 0 || raster.RGBAAt(0, 24).A != 255 {
		t.Fatal("color extension changed the border's coverage")
	}
}
