package html

import (
	"bytes"
	"fmt"
	"image"
	"sync"
	"testing"

	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/layout"
	"github.com/diiyw/onix/html/style"
	"github.com/diiyw/onix/render"
)

func TestHTMLRasterScratchBudgetAndIndependentBorrowers(t *testing.T) {
	first, second := acquireHTMLRasterScratch(), acquireHTMLRasterScratch()
	if first == second {
		t.Fatal("active paint operations share scratch")
	}
	releaseHTMLRasterScratch(second)
	first.mask = make([]float64, htmlRasterScratchByteLimit/8+1)
	first.paint = *image.NewRGBA(image.Rect(0, 0, 16, 16))
	releaseHTMLRasterScratch(first)
	if first.mask != nil || first.paint.Pix != nil {
		t.Fatal("oversized raster scratch was retained")
	}
}

func TestHTMLConcurrentRasterScratchDoesNotOverwriteResults(t *testing.T) {
	const count = 8
	var expected [count][]byte
	paint := func(index int) *image.RGBA {
		computed := &style.ComputedStyle{BackgroundColor: style.ColorValue(style.Color{R: byte(19 * index), G: 97, B: 231, A: 192}), Opacity: style.Number(.7)}
		computed.Filters = []style.Filter{{Kind: style.FilterBlur, Amount: float64(index%3 + 1)}}
		box := &layout.Box{Node: &dom.Node{}, Style: computed, Content: layout.Rect{Width: float64(11 + index), Height: float64(8 + index)}}
		cache := htmlTextureCache{}
		return &htmlOpacityRasterFor(&cache, box).image
	}
	for index := range count {
		expected[index] = bytes.Clone(paint(index).Pix)
	}
	var wait sync.WaitGroup
	for index := range count {
		wait.Go(func() {
			for range 8 {
				result := paint(index)
				// Another operation may immediately reuse this operation's scratch.
				_ = paint((index + 1) % count)
				if !bytes.Equal(result.Pix, expected[index]) {
					t.Errorf("paint %d changed after scratch reuse", index)
					return
				}
			}
		})
	}
	wait.Wait()
}

type sharedRasterDriver struct {
	textureTestDriver
	pixels []byte
	source *byte
}

func (d *sharedRasterDriver) CreateTexture(descriptor render.TextureDescriptor, pixels []byte) (render.TextureHandle, error) {
	d.source = &pixels[0]
	d.pixels = append(d.pixels[:0], pixels...)
	return d.textureTestDriver.CreateTexture(descriptor, pixels)
}

func TestHTMLRasterTextureSharesPixelsAndDetachesOnResize(t *testing.T) {
	driver := &sharedRasterDriver{}
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	state := htmlRenderState{}
	defer state.Textures.dispose()
	computed := &style.ComputedStyle{BackgroundColor: style.ColorValue(style.Color{R: 231, A: 255}), Opacity: style.Number(.5)}
	box := &layout.Box{Node: &dom.Node{}, Style: computed, Content: layout.Rect{Width: 32, Height: 24}}
	first := htmlOpacityTexture(&state, box)
	if !first.RenderHandle().Valid() || driver.source != &state.Textures.rasters[box.Node].image.Pix[0] {
		t.Fatal("opacity texture copied its retained raster")
	}
	oldPixels := state.Textures.rasters[box.Node].image.Pix
	want := bytes.Clone(oldPixels)
	box.Content.Width, box.Content.Height = 8, 6
	computed.BackgroundColor.Color.R = 17
	second := htmlOpacityTexture(&state, box)
	if second == first || !bytes.Equal(oldPixels, want) {
		t.Fatal("resizing overwrote the old texture's CPU pixels")
	}
	if !second.RenderHandle().Valid() || driver.source != &state.Textures.rasters[box.Node].image.Pix[0] || bytes.Equal(driver.pixels, want) {
		t.Fatal("replacement opacity texture lost its new pixels")
	}
	computed.BackgroundImage = "linear-gradient(red,blue)"
	for _, shadow := range []bool{false, true} {
		var variant string
		if shadow {
			variant = "shadow-0"
			computed.BoxShadow = []style.Shadow{{BlurRadius: style.LengthPx(8), Color: style.Color{A: 128}}}
		} else {
			variant = "background"
		}
		paint := func(size float32) {
			var texture = htmlPaintTextureFor(&state, box.Node, computed, size, size)
			if shadow {
				texture = htmlBoxShadowTextureFor(&state, box.Node, computed, 0, size, size, size-8, size-8, 4)
			}
			if !texture.RenderHandle().Valid() || driver.source != &state.Textures.paint[htmlTextureKey{node: box.Node, variant: variant}].image.Pix[0] {
				t.Fatal("paint texture copied its retained raster")
			}
		}
		paint(32)
		old := state.Textures.paint[htmlTextureKey{node: box.Node, variant: variant}].image.Pix
		copyOfOld := bytes.Clone(old)
		paint(16)
		if !bytes.Equal(old, copyOfOld) {
			t.Fatal("paint resize overwrote the previous texture pixels")
		}
	}
}

func BenchmarkHTMLRasterPopulation(b *testing.B) {
	for _, count := range []int{1, 16} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			computed := &style.ComputedStyle{BackgroundColor: style.ColorValue(style.Color{R: 97, A: 255})}
			computed.BoxShadow = []style.Shadow{{BlurRadius: style.LengthPx(16), Color: style.Color{A: 128}}}
			nodes := make([]dom.Node, count)
			b.ReportAllocs()
			for b.Loop() {
				state := htmlRenderState{}
				for index := range nodes {
					htmlBoxShadowTextureFor(&state, &nodes[index], computed, 0, 128, 128, 96, 96, 16)
				}
				state.Textures.dispose()
			}
		})
	}
}
