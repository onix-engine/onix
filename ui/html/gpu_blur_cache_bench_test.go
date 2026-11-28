//go:build linux && cgo && !android && !onix_drm

package html

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"runtime"
	"slices"
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/graphics"
	"github.com/onix-engine/onix/html/layout"
	"github.com/onix-engine/onix/render"
	uinode "github.com/onix-engine/onix/scene/node"
	"github.com/onix-engine/onix/window"
)

// Readback waits for each offscreen draw to finish before the next sample.
func BenchmarkNativeHTMLGPUBlurCache(b *testing.B) {
	b.Run("static", func(b *testing.B) { benchmarkNativeHTMLGPUBlurCache(b, "static", 4) })
	for _, sigma := range []float32{4, 8, htmlGPUBlurMaxSigma} {
		for _, backend := range []string{"gpu", "cpu"} {
			b.Run(fmt.Sprintf("refresh/sigma%g/%s", sigma, backend), func(b *testing.B) {
				benchmarkNativeHTMLGPUBlurCache(b, backend, sigma)
			})
		}
	}
}

func benchmarkNativeHTMLGPUBlurCache(b *testing.B, mode string, sigma float32) {
	if os.Getenv("DISPLAY") == "" {
		b.Skip("requires Xvfb or an X display")
	}
	const size = 256
	// Each sub-benchmark creates and uses its context on the same OS thread.
	context := window.NewContext(size, size, "blur cache benchmark", window.Options{})
	defer runtime.UnlockOSThread()
	defer context.Destroy()
	context.Device.Activate()
	b.Logf("renderer: %+v", context.Device.RendererInfo())
	canvas := graphics.NewCanvas2D(context.Device, size, size)
	world := ecs.NewWorld()
	graphics.BindFrame(world, canvas)
	cache := &htmlTextureCache{}
	defer cache.dispose()
	if mode != "cpu" && !htmlGPUBlurAvailable(world, 0, cache, &layout.Box{}, float64(sigma)) {
		b.Fatal("GPU blur shader unavailable")
	}
	source := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 64; y < 192; y++ {
		for x := 64; x < 192; x++ {
			source.SetRGBA(x, y, color.RGBA{255, 91, 37, 255})
		}
	}
	texture := graphics.CreateTextureBySharedImage(source, false, graphics.ResourceUsageDefault)
	key := htmlTextureKey{node: &uinode.Node{}}
	cache.add(key, texture)
	target, err := render.NewRenderTarget(render.RenderTargetDescriptor{Width: size, Height: size})
	if err != nil {
		b.Fatal(err)
	}
	defer target.Dispose()
	version := uint64(1)
	original := slices.Clone(source.Pix)
	updates := []graphics.TextureRegionUpdate{{Width: size, Height: size, Pixels: source.Pix}}
	var scratch htmlFilterScratch
	var pixel [4]byte
	draw := func() {
		if mode != "static" {
			copy(source.Pix, original)
			if mode == "cpu" {
				htmlGaussianBlur(source, float64(sigma), &scratch)
			}
			texture.UpdateRegions(updates)
			version++
		}
		blurred := texture
		if mode != "cpu" {
			blurred = cache.blurTexture(canvas, key, texture, version, [4][4]float32{{0, 0, sigma, 1}})
			if blurred == nil {
				b.Fatal("blur failed")
			}
		}
		if err := canvas.DrawTo(target, render.RenderTargetClear{ClearColor: true}, func() error {
			if mode != "cpu" {
				canvas.BindMaterial2D(cache.blurShader)
				canvas.SetMaterial2DParameters(htmlGPUBlurCompositeParameters(context.Device, blurred))
			}
			canvas.BindTexture(blurred)
			canvas.DrawAffine([6]float32{size, 0, 0, 0, size, 0}, [6]float32{1, 0, 0, 0, 1, 0}, 1)
			canvas.DrawFrame()
			canvas.BindMaterial2D(nil)
			canvas.SetMaterial2DParameters([4][4]float32{})
			return nil
		}); err != nil {
			b.Fatal(err)
		}
		if err := canvas.FinishFrame(); err != nil {
			b.Fatal(err)
		}
		if err := context.Device.Present(); err != nil {
			b.Fatal(err)
		}
		if err := target.Read(render.RenderTargetRegion{X: size / 2, Y: size / 2, Width: 1, Height: 1}, pixel[:]); err != nil {
			b.Fatal(err)
		}
		if pixel[3] == 0 {
			b.Fatal("empty blur output")
		}
	}
	draw()
	b.ReportAllocs()
	for b.Loop() {
		draw()
	}
}
