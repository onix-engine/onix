//go:build linux && cgo && !android && !onix_drm

package html

import (
	"image"
	"image/color"
	"os"
	"runtime"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/layout"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/window"
)

// Readback synchronizes each sample with execution. This measures a complete
// offscreen draw plus readback, rather than asynchronous CPU submission alone.
func BenchmarkNativeHTMLGPUBlurCache(b *testing.B) {
	if os.Getenv("DISPLAY") == "" {
		b.Skip("requires Xvfb or an X display")
	}
	const size = 256
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
	if !htmlGPUBlurAvailable(world, 0, cache, &layout.Box{}, 4) {
		b.Fatal("GPU blur shader unavailable")
	}
	source := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 64; y < 192; y++ {
		for x := 64; x < 192; x++ {
			source.SetRGBA(x, y, color.RGBA{255, 91, 37, 255})
		}
	}
	texture := graphics.CreateTextureBySharedImage(source, false, graphics.ResourceUsageDefault)
	key := htmlTextureKey{node: &dom.Node{}}
	cache.add(key, texture)
	target, err := render.NewRenderTarget(render.RenderTargetDescriptor{Width: size, Height: size})
	if err != nil {
		b.Fatal(err)
	}
	defer target.Dispose()
	for _, dirty := range []bool{false, true} {
		name := "static"
		if dirty {
			name = "refresh"
		}
		b.Run(name, func(b *testing.B) {
			version := uint64(1)
			draw := func() {
				blurred := cache.blurTexture(canvas, key, texture, version, [4][4]float32{{0, 0, 4, 1}})
				if blurred == nil {
					b.Fatal("blur failed")
				}
				if err := canvas.DrawTo(target, render.RenderTargetClear{ClearColor: true}, func() error {
					canvas.BindMaterial2D(cache.blurShader)
					canvas.SetMaterial2DParameters(htmlGPUBlurCompositeParameters(context.Device))
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
				var pixel [4]byte
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
				if dirty {
					version++
				}
				draw()
			}
		})
	}
}
