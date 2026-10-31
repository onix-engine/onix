package html

import (
	"bytes"
	"image"
	"testing"

	"github.com/diiyw/onix/html/layout"
	"github.com/diiyw/onix/html/style"
	"github.com/diiyw/onix/render"
)

func TestHTMLOpacityCachesOnlyGroupsAndDirectSubtrees(t *testing.T) {
	root := roundTwoOpacityTree(16)
	cache := htmlTextureCache{}
	htmlOpacityRasterFor(&cache, root)
	retained := 0
	for _, raster := range cache.rasters {
		retained += cap(raster.image.Pix)
	}
	if retained != 2*128*128*4 {
		t.Fatalf("nested wrappers retain %d image bytes, want two images", retained)
	}
	child := root.Children[0]
	childVersion := cache.rasters[child.Node].version
	root.Style.BackgroundColor.Color.R++
	htmlOpacityRasterFor(&cache, root)
	if cache.rasters[child.Node].version != childVersion {
		t.Fatal("parent-only paint invalidated the static subtree cache")
	}
	group := child.Children[0].Children[0]
	for _, opacity := range []float64{.5, 1, .25, 1} {
		group.Style.Opacity = style.Number(opacity)
		got := htmlOpacityRasterFor(&cache, root)
		rebuilt := htmlOpacityRasterFor(&htmlTextureCache{}, root)
		if !bytes.Equal(got.image.Pix, rebuilt.image.Pix) || got.rect != rebuilt.rect {
			t.Fatal("changing group boundaries left a stale composite")
		}
		if opacity == 1 && cap(cache.rasters[group.Node].image.Pix) != 0 {
			t.Fatal("ordinary wrapper retained its former effect image")
		}
	}
}

// Keep allocation measurements separate from race checks: the race runtime
// deliberately discards sync.Pool entries, even without a garbage collection.
func TestHTMLOpacityDescendantWarmAllocations(t *testing.T) {
	root := roundTwoOpacityTree(16)
	cache := htmlTextureCache{}
	htmlOpacityRasterFor(&cache, root)
	leaf := root
	for len(leaf.Children) != 0 {
		leaf = leaf.Children[0]
	}
	if allocations := testing.AllocsPerRun(50, func() {
		leaf.Style.BackgroundColor.Color.R ^= 1
		htmlOpacityRasterFor(&cache, root)
	}); allocations != 0 {
		t.Fatalf("warm descendant repaint allocated %g times", allocations)
	}
}

func TestHTMLFormerCompositeRootReleasesImageWithoutOverwritingTexture(t *testing.T) {
	driver := &sharedRasterDriver{}
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	state := htmlRenderState{}
	defer state.Textures.dispose()
	root := roundTwoOpacityTree(8)
	previousRoot := root.Children[0].Children[0].Children[0]
	texture := htmlOpacityTexture(&state, previousRoot)
	raster := state.Textures.rasters[previousRoot.Node]
	pixels := raster.image.Pix
	want := bytes.Clone(pixels)
	htmlOpacityRasterFor(&state.Textures, root)
	if cap(raster.image.Pix) != 0 || !texture.Valid() || !bytes.Equal(pixels, want) {
		t.Fatal("adopting a former root retained its image or overwrote live texture pixels")
	}
	replacement := htmlOpacityTexture(&state, previousRoot)
	if replacement == texture || !replacement.RenderHandle().Valid() || driver.source != &raster.image.Pix[0] {
		t.Fatal("restoring a composite root did not share its new raster pixels")
	}
}

func TestHTMLOrdinaryWrappersDoNotChangePainting(t *testing.T) {
	root := roundTwoOpacityTree(1)
	first := root.Children[0]
	first.Style.BackgroundColor = style.ColorValue(style.Color{})
	red, blue := roundTwoOpacityTree(0), roundTwoOpacityTree(0)
	red.Style.Opacity, blue.Style.Opacity = style.Number(1), style.Number(1)
	red.Style.BackgroundColor = style.ColorValue(style.Color{R: 193, G: 17, A: 97})
	blue.Style.BackgroundColor = style.ColorValue(style.Color{B: 231, G: 57, A: 139})
	red.Content = layout.Rect{X: 3.25, Y: 4.75, Width: 20, Height: 20}
	blue.Content = layout.Rect{X: 7.75, Y: 9.25, Width: 20, Height: 20}
	first.Children = []*layout.Box{red, blue}
	want := bytes.Clone(htmlOpacityRasterFor(&htmlTextureCache{}, root).image.Pix)
	wrappers := roundTwoOpacityTree(6).Children[0]
	current := wrappers
	for {
		current.Style.BackgroundColor = style.ColorValue(style.Color{})
		if len(current.Children) == 0 {
			break
		}
		current = current.Children[0]
	}
	current.Children = first.Children
	first.Children = []*layout.Box{wrappers}
	got := htmlOpacityRasterFor(&htmlTextureCache{}, root)
	if !bytes.Equal(got.image.Pix, want) {
		t.Fatal("ordinary wrappers changed fractional placement or overlapping alpha")
	}
}

func TestHTMLShadowWorkingSetBudget(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 1024, 1024))
	var scratch htmlFilterScratch
	htmlBlurShadowMask(dst, 8, &scratch)
	if bytes := (cap(scratch.mask) + cap(scratch.maskLine)) * 8; bytes > 9<<20 {
		t.Fatalf("shadow working buffers retain %d bytes, budget is 9 MiB", bytes)
	}
}
