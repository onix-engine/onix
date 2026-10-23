package html

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/internal/style"
	"github.com/diiyw/onix/scene"
)

func TestHTMLFiltersApplyInOrderAndClampEachFunction(t *testing.T) {
	for _, test := range []struct {
		name    string
		filters []style.Filter
		want    color.RGBA
	}{
		{"brightness then invert", []style.Filter{{Kind: style.FilterBrightness, Amount: 2}, {Kind: style.FilterInvert, Amount: 1}}, color.RGBA{0, 191, 255, 255}},
		{"invert then brightness", []style.Filter{{Kind: style.FilterInvert, Amount: 1}, {Kind: style.FilterBrightness, Amount: 2}}, color.RGBA{254, 255, 255, 255}},
		{"opacity", []style.Filter{{Kind: style.FilterOpacity, Amount: .25}}, color.RGBA{128, 32, 0, 64}},
		{"grayscale", []style.Filter{{Kind: style.FilterGrayscale, Amount: 1}}, color.RGBA{50, 50, 50, 255}},
		{"contrast zero", []style.Filter{{Kind: style.FilterContrast, Amount: 0}}, color.RGBA{128, 128, 128, 255}},
	} {
		t.Run(test.name, func(t *testing.T) {
			dst := image.NewRGBA(image.Rect(0, 0, 1, 1))
			dst.SetRGBA(0, 0, color.RGBA{128, 32, 0, 255})
			var scratch htmlFilterScratch
			applyHTMLFilters(dst, test.filters, &scratch)
			if got := dst.RGBAAt(0, 0); got != test.want {
				t.Fatalf("filtered pixel = %v, want %v", got, test.want)
			}
		})
	}
}

func TestHTMLBlurPreservesColorAcrossTransparentPixelsAndReusesBuffers(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 11, 11))
	var scratch htmlFilterScratch
	filters := []style.Filter{{Kind: style.FilterBlur, Amount: 1}}
	paint := func() {
		clear(dst.Pix)
		dst.SetRGBA(5, 5, color.RGBA{255, 0, 0, 255})
		applyHTMLFilters(dst, filters, &scratch)
	}
	paint()
	center, neighbor := dst.RGBAAt(5, 5), dst.RGBAAt(6, 5)
	if center.R != 255 || neighbor.R != 255 || center.A <= neighbor.A || neighbor.A == 0 {
		t.Fatalf("blur introduced a dark fringe or lost coverage: center=%v neighbor=%v", center, neighbor)
	}
	if dst.RGBAAt(0, 0).A != 0 {
		t.Fatal("blur leaked beyond its support")
	}
	if allocations := testing.AllocsPerRun(50, paint); allocations != 0 {
		t.Fatalf("warm blur allocations = %g", allocations)
	}
}

func TestHTMLDropShadowUsesAlphaMaskAndFractionalOffset(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 9, 9))
	var scratch htmlFilterScratch
	filters := []style.Filter{{Kind: style.FilterDropShadow, Shadow: style.Shadow{OffsetX: style.LengthPx(2.5), OffsetY: style.LengthPx(1), Color: style.Color{B: 255, A: 255}}}}
	paint := func() {
		clear(dst.Pix)
		dst.SetRGBA(2, 2, color.RGBA{255, 0, 0, 255})
		applyHTMLFilters(dst, filters, &scratch)
	}
	paint()
	if original := dst.RGBAAt(2, 2); original != (color.RGBA{255, 0, 0, 255}) {
		t.Fatalf("shadow changed the source: %v", original)
	}
	for _, x := range []int{4, 5} {
		if shadow := dst.RGBAAt(x, 3); shadow != (color.RGBA{0, 0, 255, 128}) {
			t.Fatalf("fractional shadow at %d = %v", x, shadow)
		}
	}
	if allocations := testing.AllocsPerRun(50, paint); allocations != 0 {
		t.Fatalf("warm drop-shadow allocations = %g", allocations)
	}
}

func TestHTMLCompositeEffectsIncludeTextAndRefreshDescendantPaint(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="group" style="width:80px;height:40px;opacity:.5;filter:invert(1)"><span id="text" style="color:red">Alpha</span><div id="child" style="width:20px;height:10px;background:red"></div></div>`)
	defer Dispose(world)
	group := incrementalHTMLNode(t, world, owner, "group")
	state, _ := ecs.Get[htmlRenderState](world, owner)
	raster := state.Textures.rasters[group.node]
	if raster == nil {
		t.Fatal("missing effect group raster")
	}
	cyan := 0
	for index := 0; index < len(raster.image.Pix); index += 4 {
		if raster.image.Pix[index] == 0 && raster.image.Pix[index+1] == 255 && raster.image.Pix[index+2] == 255 && raster.image.Pix[index+3] > 0 {
			cyan++
		}
	}
	if cyan <= 200 {
		t.Fatalf("text missing from inverted composite: %d cyan pixels", cyan)
	}
	version := raster.version
	incrementalHTMLNode(t, world, owner, "text").SetAttribute("style", "color:blue")
	incrementalHTMLNode(t, world, owner, "child").SetAttribute("style", "width:20px;height:10px;background:blue")
	incrementalSync(t, world)
	if raster.version <= version {
		t.Fatal("descendant repaint did not invalidate group")
	}
	yellow := 0
	for index := 0; index < len(raster.image.Pix); index += 4 {
		if raster.image.Pix[index] == 255 && raster.image.Pix[index+1] == 255 && raster.image.Pix[index+2] == 0 && raster.image.Pix[index+3] > 0 {
			yellow++
		}
	}
	if yellow <= 200 {
		t.Fatalf("updated text missing from composite: %d yellow pixels", yellow)
	}
}

func TestHTMLCompositeOpacityAppliesAfterOverlappingChildren(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="group" style="position:relative;width:20px;height:20px;opacity:.5"><div style="position:absolute;inset:0;background:red"></div><div style="position:absolute;inset:0;background:blue;opacity:.5"></div></div>`)
	defer Dispose(world)
	state, _ := ecs.Get[htmlRenderState](world, owner)
	node := incrementalHTMLNode(t, world, owner, "group")
	pixel := state.Textures.rasters[node.node].image.RGBAAt(10, 10)
	if pixel.A != 128 || math.Abs(float64(pixel.R)-127) > 1 || math.Abs(float64(pixel.B)-128) > 1 {
		t.Fatalf("opacity was applied before subtree compositing: %v", pixel)
	}
}

func TestHTMLNestedTextOpacityMutationRefreshesComposite(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="group" style="width:80px;height:30px;background:white;opacity:.5"><div id="text">Alpha</div></div>`)
	defer Dispose(world)
	incrementalHTMLNode(t, world, owner, "text").SetAttribute("style", "opacity:.25")
	incrementalSync(t, world)
	state, _ := ecs.Get[htmlRenderState](world, owner)
	group := state.Textures.rasters[incrementalHTMLNode(t, world, owner, "group").node]
	child := state.Textures.rasters[incrementalHTMLNode(t, world, owner, "text").node]
	if child == nil || child.opacity != style.Number(.25) {
		t.Fatalf("stale nested text opacity: %v", child)
	}
	minRed := uint8(255)
	for offset := 0; offset < len(group.image.Pix); offset += 4 {
		if group.image.Pix[offset+3] > 0 {
			minRed = min(minRed, group.image.Pix[offset])
		}
	}
	if minRed < 190 || minRed > 192 {
		t.Fatalf("nested text was not composited at .25 opacity: darkest red=%d", minRed)
	}
}

func TestHTMLCompositedTextRetainsFocusAndInputBounds(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="group" style="width:80px;height:30px;filter:blur(2px)"><a id="link" href="#test">Link</a></div>`)
	defer Dispose(world)
	state, _ := ecs.Get[htmlRenderState](world, owner)
	group := incrementalHTMLNode(t, world, owner, "group")
	link := incrementalHTMLNode(t, world, owner, "link")
	if state.Textures.rasters[group.node] == nil {
		t.Fatal("missing blurred text raster")
	}
	foundFocus, foundBounds := false, false
	for entity, access := range ecs.Query2[htmlChild, scene.InputTarget2D](world).All {
		child := access.A.Get()
		target := access.B.Get()

		if child.Owner != owner {
			continue
		}
		if child.Node == link.node && ecs.Has[Focusable](world, entity) {
			foundFocus = true
		}
		if child.Node == group.node {
			if target.Width != 80 || target.Height != 30 {
				t.Fatalf("filter expanded hit bounds: %+v", target)
			}
			foundBounds = true
		}
	}
	if !foundFocus || !foundBounds {
		t.Fatalf("composite lost focus/input: focus=%v bounds=%v", foundFocus, foundBounds)
	}
	group.OnClick(func(*PointerEvent) {})
	incrementalSync(t, world)
	group.OnClick(nil)
	incrementalSync(t, world)
	if entity := htmlNodeInputEntity(world, &state, group.node); entity == 0 || !ecs.Has[scene.InputTarget2D](world, entity) {
		t.Fatal("event-only updates removed the compositor's input target")
	}
}

func TestHTMLBoxShadowBlursMaskAndExcludesBoxInterior(t *testing.T) {
	var dst image.RGBA
	var scratch htmlFilterScratch
	computed := &style.ComputedStyle{}
	shadow := style.Shadow{OffsetX: style.LengthPx(0), OffsetY: style.LengthPx(0), BlurRadius: style.LengthPx(4), Color: style.Color{A: 255}}
	paint := func() { rasterHTMLBoxShadow(&dst, computed, shadow, 20, 20, 9, &scratch) }
	paint()
	if dst.RGBAAt(19, 19).A != 0 {
		t.Fatal("outer shadow filled transparent box interior")
	}
	near, far := dst.RGBAAt(8, 19).A, dst.RGBAAt(4, 19).A
	if near == 0 || near == 255 || near <= far {
		t.Fatalf("shadow did not convolve the box mask: near=%d far=%d", near, far)
	}
	if allocations := testing.AllocsPerRun(50, paint); allocations != 0 {
		t.Fatalf("warm shadow allocations = %g", allocations)
	}
}

func TestHTMLFilterBoundsExpandSequentially(t *testing.T) {
	filters := []style.Filter{{Kind: style.FilterBlur, Amount: 2}, {Kind: style.FilterDropShadow, Shadow: style.Shadow{OffsetX: style.LengthPx(10), OffsetY: style.LengthPx(-4), BlurRadius: style.LengthPx(1)}}}
	x, y, right, bottom := htmlFilterBounds(filters, 0, 0, 20, 10)
	if x != -6 || y != -13 || right != 39 || bottom != 16 {
		t.Fatalf("effect bounds = %g,%g..%g,%g", x, y, right, bottom)
	}
}
