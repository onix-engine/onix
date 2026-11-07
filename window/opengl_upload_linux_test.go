//go:build linux && cgo && !android && !onix_drm

package window

import (
	"os"
	"runtime"
	"testing"

	"github.com/onix-engine/onix/render"
)

func TestNativeOpenGLStreamBatches(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("requires an X display; run under xvfb-run")
	}
	if render.CompiledBackend != render.BackendOpenGL {
		t.Skip("requires OpenGL")
	}
	context := NewContext(64, 64, "OpenGL upload regression", Options{})
	defer runtime.UnlockOSThread()
	defer context.Destroy()
	driver := context.Device.Driver()
	targets := driver.(render.RenderTargetDriver)
	readback := driver.(render.RenderTargetReadbackDriver)
	target, _, err := targets.CreateRenderTarget(render.RenderTargetDescriptor{Width: 8, Height: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer targets.DestroyRenderTarget(target)
	texture, err := driver.CreateTexture(render.TextureDescriptor{Width: 1, Height: 1}, []byte{255, 255, 255, 255})
	if err != nil {
		t.Fatal(err)
	}
	defer driver.DestroyTexture(texture)
	shaders := driver.(render.Material2DShaderDriver)
	shader, err := shaders.CreateMaterial2DShader(render.ShaderDescriptor{
		WebGL2FragmentSource: "#version 300 es\nprecision highp float; in vec4 _vs2fs_location1; out vec4 color; void main(){color=_vs2fs_location1;}",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer shaders.DestroyMaterial2DShader(shader)
	identity := [16]float32{0: 1, 5: 1, 10: 1, 15: 1}
	model := [6]float32{2, 0, -1, 0, 2, -1}
	sprite := render.Instance2D{Model: model, TextureModel: [6]float32{1, 0, 0, 0, 1, 0}, Color: [4]float32{1, 0, 0, 1}}
	sprites := render.SpriteBatch{
		Projection: identity,
		Instances:  []render.Instance2D{sprite, sprite},
		Draws: []render.SpriteDraw{
			{Texture: texture, InstanceCount: 1},
			{Shader: shader, Texture: texture, FirstInstance: 1, InstanceCount: 1},
		},
	}
	shapes := render.ShapeBatch{Projection: identity, Instances: []render.ShapeInstance2D{{
		Model: model, LocalModel: [6]float32{1, 0, 0, 0, 1, 0},
		Fill: [4]float32{0, 1, 0, 1}, Params: [4]float32{8, 8}, Kind: render.ShapeRoundedRect,
	}}}
	pixel := make([]byte, 4)
	check := func(green bool) {
		t.Helper()
		if err := targets.EndRenderTarget(); err != nil {
			t.Fatal(err)
		}
		if err := readback.ReadRenderTarget(target, render.RenderTargetRegion{X: 4, Y: 4, Width: 1, Height: 1}, pixel); err != nil {
			t.Fatal(err)
		}
		channel := 0
		if green {
			channel = 1
		}
		if pixel[channel] != 255 || pixel[1-channel] != 0 || pixel[2] != 0 || pixel[3] != 255 {
			t.Fatalf("green=%v, pixel=%v", green, pixel)
		}
		if err := targets.BeginRenderTarget(target, render.RenderTargetClear{}); err != nil {
			t.Fatal(err)
		}
	}
	frame := func() {
		if err := driver.BeginFrame(render.Color{A: 1}); err != nil {
			t.Fatal(err)
		}
		if err := targets.BeginRenderTarget(target, render.RenderTargetClear{ClearColor: true}); err != nil {
			t.Fatal(err)
		}
		for range 600 {
			if err := driver.DrawSprites(sprites); err != nil {
				t.Fatal(err)
			}
			if err := driver.DrawShapes(shapes); err != nil {
				t.Fatal(err)
			}
		}
		check(true)
		if err := driver.DrawSprites(sprites); err != nil {
			t.Fatal(err)
		}
		check(false)
		if err := targets.EndRenderTarget(); err != nil {
			t.Fatal(err)
		}
		if err := driver.Present(); err != nil {
			t.Fatal(err)
		}
	}
	for range 6 {
		frame()
	}
	if allocations := testing.AllocsPerRun(6, frame); allocations != 0 {
		t.Fatalf("OpenGL frame allocations = %v, want 0", allocations)
	}
	t.Logf("%s: compact/custom sprites and shapes share growing stream blocks, pixel readbacks and zero warm Go allocations passed", driver.RendererInfo().Device)
}
