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
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	"github.com/diiyw/onix/window"
)

func TestNativeHTMLGPUBlurAndShadow(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("requires Xvfb or an X display")
	}
	context := window.NewContext(64, 64, "html gpu effects", window.Options{})
	defer runtime.UnlockOSThread()
	defer context.Destroy()
	context.Device.Activate()
	canvas := graphics.NewCanvas2D(context.Device, 64, 64)
	world := ecs.NewWorld()
	graphics.BindFrame(world, canvas)
	cache := &htmlTextureCache{}
	defer cache.dispose()
	if !htmlGPUBlurAvailable(world, 0, cache, &layout.Box{}, 2) {
		t.Fatal("GPU blur shader was unavailable")
	}
	const size = 25
	target, err := render.NewRenderTarget(render.RenderTargetDescriptor{Width: size, Height: size})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Dispose()

	for _, shadow := range []bool{false, true} {
		source := image.NewRGBA(image.Rect(0, 0, size, size))
		if shadow {
			for y := 9; y < 16; y++ {
				for x := 9; x < 16; x++ {
					source.SetRGBA(x, y, color.RGBA{0, 255, 0, 255})
				}
			}
		} else {
			source.SetRGBA(12, 12, color.RGBA{255, 0, 0, 255})
		}
		texture := graphics.CreateTextureBySharedImage(source, false, graphics.ResourceUsageDefault)
		key := htmlTextureKey{node: &dom.Node{}}
		cache.add(key, texture)
		parameters := [4][4]float32{{0, 0, 2, 1}}
		if shadow {
			parameters[0][3] = 2
			parameters[1] = [4]float32{9, 9, 7, 7}
			parameters[2] = [4]float32{3, 3, 3, 3}
			parameters[3] = [4]float32{3, 3, 3, 3}
		}
		blurred := cache.blurTexture(canvas, key, texture, 1, parameters)
		if blurred == nil || cache.blurTexture(canvas, key, texture, 1, parameters) != blurred {
			t.Fatal("complete blur target was not retained")
		}
		parameters = htmlGPUBlurCompositeParameters(context.Device)
		if err := canvas.DrawTo(target, render.RenderTargetClear{ClearColor: true}, func() error {
			canvas.BindMaterial2D(cache.blurShader)
			canvas.SetMaterial2DParameters(parameters)
			canvas.BindTexture(blurred)
			canvas.DrawAffine([6]float32{size, 0, 0, 0, size, 0}, [6]float32{1, 0, 0, 0, 1, 0}, 1)
			canvas.DrawFrame()
			canvas.BindMaterial2D(nil)
			canvas.SetMaterial2DParameters([4][4]float32{})
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := canvas.FinishFrame(); err != nil {
			t.Fatal(err)
		}
		if err := context.Device.Present(); err != nil {
			t.Fatal(err)
		}
		pixels := make([]byte, size*size*4)
		if err := target.Read(render.RenderTargetRegion{Width: size, Height: size}, pixels); err != nil {
			t.Fatal(err)
		}
		at := func(x, y int) color.RGBA {
			i := (y*size + x) * 4
			return color.RGBA{pixels[i], pixels[i+1], pixels[i+2], pixels[i+3]}
		}
		if shadow {
			if at(12, 12).A != 0 || at(9, 9).A == 0 || at(8, 12).G == 0 || at(0, 0).A != 0 {
				t.Fatalf("GPU shadow knockout or blur failed: center=%v corner=%v near=%v far=%v", at(12, 12), at(9, 9), at(8, 12), at(0, 0))
			}
		} else if max(at(12, 12).R, at(12, 12).B) == 0 || max(at(13, 12).R, at(13, 12).B) == 0 || at(13, 12).G != 0 || at(0, 0).A != 0 {
			t.Fatalf("GPU blur lost the source color or support: center=%v near=%v far=%v", at(12, 12), at(13, 12), at(0, 0))
		}
	}
}

func TestNativeHTMLGPUEffectsBindToDOM(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("requires Xvfb or an X display")
	}
	context := window.NewContext(128, 96, "html gpu dom", window.Options{})
	defer runtime.UnlockOSThread()
	defer context.Destroy()
	context.Device.Activate()
	world := ecs.NewWorld()
	graphics.BindFrame(world, graphics.NewCanvas2D(context.Device, 128, 96))
	html := New(`<div id="blur" style="width:32px;height:20px;background:red;filter:blur(2px)"></div><div id="shadow" style="width:32px;height:20px;background:white;box-shadow:0 0 4px black"></div>`)
	html.Width, html.Height, html.Font = 128, 96, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	bundle.Put(html)
	owner := world.Spawn(bundle)
	defer Dispose(world)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	blur := incrementalHTMLNode(t, world, owner, "blur")
	shadow := incrementalHTMLNode(t, world, owner, "shadow")
	blurEntity := htmlCullRole(world, owner, blur, htmlRoleSurface)
	shadowEntity := htmlCullRole(world, owner, shadow, htmlRoleShadow)
	blurShader, blurOK := world.Get[scene.Shader[graphics.Material2DShader]](blurEntity)
	shadowShader, shadowOK := world.Get[scene.Shader[graphics.Material2DShader]](shadowEntity)
	if !blurOK || !shadowOK || blurShader.Custom != htmlGPUBlurCompositeParameters(context.Device) || shadowShader.Custom != htmlGPUBlurCompositeParameters(context.Device) {
		t.Fatalf("HTML GPU effects were not bound: blur=%v shadow=%v", blurOK, shadowOK)
	}
	state := world.GetRef[htmlRenderState](owner)
	if source := state.Textures.entries[htmlTextureKey{node: blur.node}]; source == nil || !source.texture.ValidFor(context.Device) {
		t.Fatal("blur source was released while its GPU output was in use")
	}
	if source := state.Textures.entries[htmlTextureKey{node: shadow.node, variant: "shadow-mask-0"}]; source == nil || !source.texture.ValidFor(context.Device) {
		t.Fatal("shadow mask was released while its GPU output was in use")
	}
	blurKey := htmlTextureKey{node: blur.node}
	shadowKey := htmlTextureKey{node: shadow.node, variant: "shadow-mask-0"}
	beforeBlur := state.Textures.gpuBlur[blurKey]
	beforeShadow := state.Textures.gpuBlur[shadowKey]
	blurVersion, shadowVersion := beforeBlur.version, beforeShadow.version
	blur.SetAttribute("style", "width:32px;height:20px;background:blue;filter:blur(2px)")
	shadow.SetAttribute("style", "width:32px;height:20px;background:white;box-shadow:0 0 4px green")
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	state = world.GetRef[htmlRenderState](owner)
	if current := state.Textures.gpuBlur[blurKey]; current == nil || current.output != beforeBlur.output || current.version <= blurVersion {
		t.Fatal("blur content change did not redraw the retained GPU target")
	}
	if current := state.Textures.gpuBlur[shadowKey]; current == nil || current.output != beforeShadow.output || current.version <= shadowVersion {
		t.Fatal("shadow content change did not redraw the retained GPU target")
	}
	blurVersion = state.Textures.gpuBlur[blurKey].version
	blur.SetAttribute("style", "width:32px;height:20px;background:blue;filter:blur(2px);opacity:.5")
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	state = world.GetRef[htmlRenderState](owner)
	if state.Textures.gpuBlur[blurKey].version != blurVersion {
		t.Fatal("opacity-only change redrew the blur source")
	}
	blurEntity = htmlCullRole(world, owner, blur, htmlRoleSurface)
	if sprite, ok := world.Get[scene.Sprite2D](blurEntity); !ok || sprite.Alpha != .5 {
		t.Fatal("opacity-only change did not update the final composite alpha")
	}
}

func TestNativeHTMLGPUBlurCacheInvalidationAndOrientation(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("requires Xvfb or an X display")
	}
	context := window.NewContext(64, 64, "blur cache validation", window.Options{})
	defer runtime.UnlockOSThread()
	defer context.Destroy()
	context.Device.Activate()
	canvas := graphics.NewCanvas2D(context.Device, 64, 64)
	world := ecs.NewWorld()
	graphics.BindFrame(world, canvas)
	cache := &htmlTextureCache{}
	defer cache.dispose()
	if !htmlGPUBlurAvailable(world, 0, cache, &layout.Box{}, 1) {
		t.Fatal("GPU blur shader unavailable")
	}
	const width, height = 31, 27
	source := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 3; y < 12; y++ {
		for x := 4; x < 13; x++ {
			// HTML raster textures contain straight alpha, including nonzero
			// color in transparent pixels. The two passes must premultiply once.
			source.SetRGBA(x, y, color.RGBA{0, 255, 0, 128})
		}
	}
	texture := graphics.CreateTextureBySharedImage(source, false, graphics.ResourceUsageDefault)
	key := htmlTextureKey{node: &dom.Node{}}
	cache.add(key, texture)
	parameters := [4][4]float32{{0, 0, 1, 1}}
	blurred := cache.blurTexture(canvas, key, texture, 1, parameters)
	if blurred == nil {
		t.Fatal("blur failed")
	}
	before := context.Device.Statistics()
	if cache.blurTexture(canvas, key, texture, 1, parameters) != blurred || context.Device.Statistics() != before {
		t.Fatal("cache hit submitted GPU work or replaced the output")
	}
	target, err := render.NewRenderTarget(render.RenderTargetDescriptor{Width: width, Height: height})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Dispose()
	read := func() []byte {
		t.Helper()
		if err := cache.drawBlurPass(canvas, target, blurred, htmlGPUBlurCompositeParameters(context.Device)); err != nil {
			t.Fatal(err)
		}
		if err := canvas.FinishFrame(); err != nil {
			t.Fatal(err)
		}
		if err := context.Device.Present(); err != nil {
			t.Fatal(err)
		}
		pixels := make([]byte, width*height*4)
		if err := target.Read(render.RenderTargetRegion{Width: width, Height: height}, pixels); err != nil {
			t.Fatal(err)
		}
		return pixels
	}
	at := func(pixels []byte, x, y int) color.RGBA {
		i := (y*width + x) * 4
		return color.RGBA{pixels[i], pixels[i+1], pixels[i+2], pixels[i+3]}
	}
	pixels := read()
	center, reflected := at(pixels, 8, 7), at(pixels, 8, height-1-7)
	if center.A < 125 || center.A > 130 || center.G < 125 || center.G > 130 || center.R != 0 || center.B != 0 || reflected.A != 0 {
		t.Fatalf("orientation or premultiplication: center=%v reflected=%v", center, reflected)
	}
	parameters[0][2] = 3
	context.Device.ResetStatistics()
	if cache.blurTexture(canvas, key, texture, 1, parameters) != blurred || context.Device.Statistics().DrawCalls != 2 {
		t.Fatal("sigma-only change did not refresh both passes in place")
	}
	if wider := read(); at(wider, 8, 7).A >= center.A || at(wider, 8, 1).A <= at(pixels, 8, 1).A {
		t.Fatal("sigma-only change did not change the pixels")
	}
	parameters[0][3] = 2
	parameters[1] = [4]float32{4, 3, 9, 9}
	cache.blurTexture(canvas, key, texture, 1, parameters)
	if shadow := read(); at(shadow, 8, 7).A != 0 || at(shadow, 8, 2).A == 0 {
		t.Fatal("asymmetric shadow knockout was flipped or not invalidated")
	}
	scratch := cache.blurScratch.target
	smallKey := htmlTextureKey{node: &dom.Node{}}
	small := graphics.CreateTextureBySharedImage(image.NewRGBA(image.Rect(0, 0, 17, 19)), false, graphics.ResourceUsageDefault)
	cache.add(smallKey, small)
	if cache.blurTexture(canvas, smallKey, small, 1, [4][4]float32{{0, 0, 1, 1}}) == nil || cache.blurScratch.target != scratch {
		t.Fatal("effects did not share the intermediate target")
	}
	if w, h := scratch.Size(); w != 17 || h != 19 {
		t.Fatalf("shared intermediate was not resized: %dx%d", w, h)
	}
	cache.sweep()
	if len(cache.gpuBlur) != 0 || len(cache.textures) != 0 || cache.blurScratch.target != nil {
		t.Fatal("unused effects retained GPU resources")
	}
}
