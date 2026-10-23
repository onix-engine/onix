//go:build js && wasm

package render

import (
	"syscall/js"
	"testing"
)

func TestBrowserSpriteMultiTexturePixels(t *testing.T) {
	if !js.Global().Get("document").Truthy() {
		t.Skip("requires a browser")
	}
	canvas := js.Global().Get("document").Call("createElement", "canvas")
	canvas.Set("width", 16)
	canvas.Set("height", 16)
	js.Global().Get("document").Get("body").Call("appendChild", canvas)
	defer canvas.Call("remove")
	driver, descriptors, check := newSpriteBrowserDriver(t, canvas)
	defer driver.Destroy()
	shaderDriver := driver.(Material2DShaderDriver)
	var custom [2]Material2DShaderHandle
	for index, descriptor := range descriptors {
		shader, err := shaderDriver.CreateMaterial2DShader(descriptor)
		if err != nil {
			t.Fatal(err)
		}
		custom[index] = shader
		defer shaderDriver.DestroyMaterial2DShader(shader)
	}
	check()
	targetDriver := driver.(RenderTargetDriver)
	target, _, err := targetDriver.CreateRenderTarget(RenderTargetDescriptor{Width: 16, Height: 16, Depth: true})
	if err != nil {
		t.Fatal(err)
	}
	defer targetDriver.DestroyRenderTarget(target)
	var textures [SpriteTextureSlots]TextureHandle
	colors := [4][4]byte{{255, 0, 0, 255}, {0, 255, 0, 128}, {0, 0, 255, 255}, {128}}
	for index := range textures {
		format, width := TextureFormatRGBA8, index+1
		if index == 3 {
			format, width = TextureFormatR8, 3
		}
		texelBytes := format.TexelBytes()
		pixels := make([]byte, width*width*texelBytes)
		for offset := 0; offset < len(pixels); offset += texelBytes {
			copy(pixels[offset:offset+texelBytes], colors[index][:])
		}
		textures[index], err = driver.CreateTexture(TextureDescriptor{Width: width, Height: width, Format: format, Filter: TextureFilter(index % 2)}, pixels)
		if err != nil {
			t.Fatal(err)
		}
		defer driver.DestroyTexture(textures[index])
	}
	if err := driver.SetBlend(PremultipliedAlphaBlend); err != nil {
		t.Fatal(err)
	}
	check()
	for frame := range 8 {
		if err := driver.BeginFrame(Color{}); err != nil {
			t.Fatal(err)
		}
		if err := targetDriver.BeginRenderTarget(target, RenderTargetClear{ClearColor: true, ClearDepth: true}); err != nil {
			t.Fatal(err)
		}
		batch := SpriteBatch{Projection: [16]float32{0: 0.125, 5: -0.125, 10: 1, 12: -1, 13: 1, 15: 1}, Instances: make([]Instance2D, 8), Draws: []SpriteDraw{
			{Texture: textures[frame%4], InstanceCount: 6},
			{Shader: custom[frame%2], Texture: textures[3], FirstInstance: 6, InstanceCount: 1},
			{Texture: textures[0], FirstInstance: 7, InstanceCount: 1},
		}}
		for slot := 1; slot < 4; slot++ {
			batch.Draws[0].AdditionalTextures[slot-1] = textures[(frame+slot)%4]
		}
		for index := range batch.Instances {
			batch.Instances[index] = Instance2D{Model: [6]float32{4, 0, float32(index%4) * 4, 0, 8, float32(index/4) * 8}, TextureModel: [6]float32{1, 0, 0, 0, 1, 0}, Color: [4]float32{1, 1, 1, 1}}
			if index < 6 {
				batch.Instances[index].TextureSlot = float32((index%4 + 4 - frame%4) % 4)
			}
		}
		batch.Instances[3].Color[1] = 0
		batch.Instances[5].Model[2] = 0
		batch.Instances[6].Custom[0] = [4]float32{0.5, 0.5, 0.5, 0.5}
		if err := driver.DrawSprites(batch); err != nil {
			t.Fatal(err)
		}
		if err := targetDriver.EndRenderTarget(); err != nil {
			t.Fatal(err)
		}
		if err := driver.Present(); err != nil {
			t.Fatal(err)
		}
		check()
		expected := [8][4]byte{{255, 0, 0, 255}, {0, 128, 0, 128}, {0, 0, 255, 255}, {128, 0, 128, 128}, {127, 128, 0, 255}, {}, {128, 128, 128, 128}, {255, 0, 0, 255}}
		for index, want := range expected {
			var pixel [4]byte
			if err := driver.(RenderTargetReadbackDriver).ReadRenderTarget(target, RenderTargetRegion{X: index%4*4 + 2, Y: index/4*8 + 4, Width: 1, Height: 1}, pixel[:]); err != nil {
				t.Fatal(err)
			}
			for channel := range pixel {
				if difference := int(pixel[channel]) - int(want[channel]); difference < -1 || difference > 1 {
					t.Fatalf("frame %d pixel %d = %v, want %v", frame, index, pixel, want)
				}
			}
		}
		check()
	}
	t.Log("RGBA/R8, four rotating texture slots, painter order and custom fallback passed")
}
