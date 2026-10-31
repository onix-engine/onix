package html

import (
	_ "embed"
	"math"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/html/layout"
	"github.com/diiyw/onix/html/style"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
)

//go:embed shaders/blur.spv
var htmlBlurSPIRV []byte

//go:embed shaders/blur.metal
var htmlBlurMetal string

//go:embed shaders/blur.glsl
var htmlBlurGLSL string

//go:embed shaders/blur.wgsl
var htmlBlurWGSL string

type htmlGPUBlur struct {
	target     *render.RenderTarget
	source     *htmlOwnedTexture
	output     *htmlOwnedTexture
	device     *render.Device
	version    uint64
	parameters [4][4]float32
}

type htmlGPUBlurScratch struct {
	target  *render.RenderTarget
	texture *graphics.Texture
}

func (scratch *htmlGPUBlurScratch) dispose() {
	if scratch.target != nil {
		_ = scratch.texture.Dispose()
		_ = scratch.target.Dispose()
		*scratch = htmlGPUBlurScratch{}
	}
}

// Cached render targets contain premultiplied pixels. Sampling a GL attachment
// also needs a vertical flip; CPU image textures keep their existing origin.
func htmlGPUBlurCompositeParameters(device *render.Device) [4][4]float32 {
	mode := float32(3)
	if device.RendererInfo().RenderTargetBottomLeft {
		mode = -mode
	}
	return [4][4]float32{{0, 0, 0, mode}}
}

func htmlGPUBlurSigma(world *ecs.World, owner ecs.Entity, cache *htmlTextureCache, box *layout.Box) (float32, bool) {
	if box == nil || box.Style == nil || len(box.Style.Filters) != 1 || box.Style.Filters[0].Kind != style.FilterBlur {
		return 0, false
	}
	sigma := box.Style.Filters[0].Amount
	if !htmlGPUBlurAvailable(world, owner, cache, box, sigma) {
		return 0, false
	}
	return float32(sigma), true
}

func htmlGPUBlurAvailable(world *ecs.World, owner ecs.Entity, cache *htmlTextureCache, box *layout.Box, sigma float64) bool {
	if sigma <= 0 || sigma > 4 || math.IsNaN(sigma) || graphics.CanvasFor(world) == nil || graphics.DeviceFor(world) == nil {
		return false
	}
	for parent := box.Parent; parent != nil; parent = parent.Parent {
		if parent.Style != nil && htmlOverflowClipsStyle(parent.Style) {
			r := parent.BorderBox()
			if htmlOverflowClipRadius(parent.Style, r.Width, r.Height) > 0 {
				return false
			}
		}
	}
	for entity := owner; entity != 0; {
		if world.Has[scene.RoundedClip2D](entity) {
			return false
		}
		parent, ok := world.Get[scene.Parent](entity)
		if !ok || parent.Entity == entity {
			break
		}
		entity = parent.Entity
	}
	if cache.blurShader == nil {
		shader, err := graphics.NewMaterial2DShader(render.ShaderDescriptor{
			VulkanFragmentSPIRV: htmlBlurSPIRV, MetalSource: htmlBlurMetal,
			WebGL2FragmentSource: htmlBlurGLSL, WebGL2Generated: true,
			WebGPUFragmentSource: htmlBlurWGSL,
		})
		if err != nil {
			return false
		}
		cache.blurShader = shader
	}
	if !cache.blurShader.RenderHandleFor(graphics.DeviceFor(world)).Valid() || cache.blurShader.UploadError() != nil {
		return false
	}
	return true
}

func (cache *htmlTextureCache) blurTexture(canvas *graphics.Canvas2D, key htmlTextureKey, source *graphics.Texture, version uint64, parameters [4][4]float32) *graphics.Texture {
	if canvas == nil || source == nil || cache.entries == nil {
		return nil
	}
	w, h := source.Width, source.Height
	base := cache.entries[key]
	if w <= 0 || h <= 0 || base == nil {
		return nil
	}
	if cache.gpuBlur == nil {
		cache.gpuBlur = make(map[htmlTextureKey]*htmlGPUBlur)
	}
	effect := cache.gpuBlur[key]
	if effect == nil || effect.target.Descriptor().Width != w || effect.target.Descriptor().Height != h {
		target, err := render.NewRenderTarget(render.RenderTargetDescriptor{Width: w, Height: h})
		if err != nil {
			return nil
		}
		texture, err := graphics.CreateTextureFromRenderTarget(target)
		if err != nil {
			_ = target.Dispose()
			return nil
		}
		entry := cache.add(htmlTextureKey{node: key.node, variant: "gpu-" + key.variant}, texture)
		entry.target = target
		effect = &htmlGPUBlur{target: target, source: base, output: entry}
		cache.gpuBlur[key] = effect
	}
	parameters[0][0], parameters[0][1] = 0, 1/float32(h)
	if canvas.Device().RendererInfo().RenderTargetBottomLeft {
		parameters[0][3] = -parameters[0][3]
	}
	if effect.version == version && effect.source == base && effect.device == canvas.Device() && effect.parameters == parameters {
		return effect.output.texture
	}
	// One shared intermediate serves all effects in this document. Only final
	// two-dimensional results are retained per effect.
	scratch := &cache.blurScratch
	if scratch.target == nil {
		target, err := render.NewRenderTarget(render.RenderTargetDescriptor{Width: w, Height: h})
		if err != nil {
			return nil
		}
		texture, err := graphics.CreateTextureFromRenderTarget(target)
		if err != nil {
			_ = target.Dispose()
			return nil
		}
		scratch.target, scratch.texture = target, texture
	} else {
		scratch.target.Resize(w, h)
	}
	horizontal := [4][4]float32{{1 / float32(w), 0, parameters[0][2], 0}}
	if err := cache.drawBlurPass(canvas, scratch.target, source, horizontal); err != nil {
		return nil
	}
	if err := cache.drawBlurPass(canvas, effect.target, scratch.texture, parameters); err != nil {
		return nil
	}
	effect.source, effect.version, effect.device, effect.parameters = base, version, canvas.Device(), parameters
	return effect.output.texture
}

func (cache *htmlTextureCache) drawBlurPass(canvas *graphics.Canvas2D, target *render.RenderTarget, source *graphics.Texture, parameters [4][4]float32) error {
	w, h := target.Size()
	return canvas.DrawTo(target, render.RenderTargetClear{ClearColor: true}, func() error {
		canvas.BindMaterial2D(cache.blurShader)
		canvas.SetMaterial2DParameters(parameters)
		canvas.BindTexture(source)
		canvas.DrawAffine([6]float32{float32(w), 0, 0, 0, float32(h), 0}, [6]float32{1, 0, 0, 0, 1, 0}, 1)
		canvas.DrawFrame()
		canvas.BindMaterial2D(nil)
		canvas.SetMaterial2DParameters([4][4]float32{})
		return nil
	})
}
