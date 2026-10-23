package graphics

import (
	"image"
	"strconv"
	"testing"

	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

func TestSpriteTextureSlotsPreserveOrder(t *testing.T) {
	for _, test := range []struct {
		name     string
		textures []int
		slots    []float32
		counts   []uint32
		custom   bool
	}{
		{"reuse", []int{0, 1, 2, 3, 0, 2}, []float32{0, 1, 2, 3, 0, 2}, []uint32{6}, false},
		{"overflow", []int{0, 1, 2, 3, 4, 0, 4, 1}, []float32{0, 1, 2, 3, 0, 1, 0, 2}, []uint32{4, 4}, false},
		{"atlas", []int{0, 5, 1, 5}, []float32{0, 0, 1, 0}, []uint32{4}, false},
		{"custom", []int{0, 0, 1, 0}, []float32{0, 0, 0, 0}, []uint32{2, 1, 1}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			driver := &canvasTestDriver{viewport: render.Rect{Width: 320, Height: 240}}
			installCanvasTestDriver(t, driver)
			canvas := &Canvas2D{device: render.ActiveDevice(), width: 320, height: 240, viewMatrix: mgl32.Ident4(), blend: render.PremultipliedAlphaBlend}
			var textures [6]*Texture
			for i := range 5 {
				textures[i] = CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, i+1, i+2)))
			}
			textures[5] = NewSubTexture(textures[0], 1, 1, 0, 0, 0.5, 0.5, 0)
			if test.custom {
				shader, err := NewMaterial2DShader(render.ShaderDescriptor{WebGL2FragmentSource: "fragment"})
				if err != nil {
					t.Fatal(err)
				}
				canvas.BindMaterial2D(shader)
			}
			// Repeat after a flush to catch stale slots in reused instance storage.
			for range 2 {
				for index, texture := range test.textures {
					canvas.BindTexture(textures[texture])
					canvas.SetMaterial2DParameters([4][4]float32{{float32(index)}})
					canvas.DrawMatrices(mgl32.Translate3D(float32(index), 0, 0), mgl32.Scale2D(0.5, 0.75), 0.5)
				}
				if err := canvas.FinishFrame(); err != nil {
					t.Fatal(err)
				}
				batch := driver.spriteBatches[len(driver.spriteBatches)-1]
				if len(batch.Draws) != len(test.counts) {
					t.Fatalf("draws = %+v, want counts %v", batch.Draws, test.counts)
				}
				var first uint32
				for index, draw := range batch.Draws {
					if draw.InstanceCount != test.counts[index] || draw.FirstInstance != first || draw.Shader.Valid() != test.custom {
						t.Fatalf("draw %d = %+v", index, draw)
					}
					for instance := first; instance < first+draw.InstanceCount; instance++ {
						if draw.TextureAt(int(test.slots[instance])) != textures[test.textures[instance]].RenderHandle() {
							t.Fatalf("instance %d samples wrong texture", instance)
						}
					}
					first += draw.InstanceCount
				}
				for index, instance := range batch.Instances {
					if instance.TextureSlot != test.slots[index] || instance.Model[2] != float32(index) || instance.Custom != ([4][4]float32{{float32(index)}}) || instance.TextureModel != ([6]float32{0.5, 0, 0, 0, 0.75, 0}) || instance.Color != ([4]float32{1, 1, 1, 0.5}) {
						t.Fatalf("instance %d was reordered or corrupted: %+v", index, instance)
					}
				}
			}
		})
	}
}

func TestSpriteMultiTextureSteadyStateAllocations(t *testing.T) {
	driver := &canvasTestDriver{viewport: render.Rect{Width: 320, Height: 240}, discardBatches: true}
	installCanvasTestDriver(t, driver)
	canvas := &Canvas2D{device: render.ActiveDevice(), width: 320, height: 240, viewMatrix: mgl32.Ident4(), blend: render.PremultipliedAlphaBlend}
	var textures [5]*Texture
	for index := range textures {
		textures[index] = CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	}
	draw := func() {
		for index := range 1000 {
			canvas.BindTexture(textures[index%len(textures)])
			canvas.DrawMatrices(mgl32.Ident4(), mgl32.Ident3(), 1)
		}
		if err := canvas.FinishFrame(); err != nil {
			t.Fatal(err)
		}
	}
	draw()
	if allocations := testing.AllocsPerRun(100, draw); allocations != 0 {
		t.Fatalf("allocations/frame = %v, want zero", allocations)
	}
}

func TestSpriteMultiTextureStateBoundaries(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*Canvas2D)
	}{
		{"projection", func(c *Canvas2D) { c.SetProjectionMatrix(mgl32.Ident4()) }},
		{"view", func(c *Canvas2D) { c.SetViewMatrix(mgl32.Translate3D(1, 0, 0)) }},
		{"clip", func(c *Canvas2D) { c.PushClipRect(0, 0, 16, 16) }},
		{"blend", func(c *Canvas2D) {
			c.BlendSeparate(render.Blend{Src: render.BlendOne, Dst: render.BlendOne}, render.Blend{Src: render.BlendOne, Dst: render.BlendOne})
		}},
		{"shape", func(c *Canvas2D) {
			c.DrawShapeMatrices(mgl32.Ident4(), mgl32.Ident3(), 16, 16, render.ShapeRoundedRect, White, ColorRGBA{}, 0, 0, 1)
		}},
		{"flush", func(c *Canvas2D) { c.DrawFrame() }},
	} {
		t.Run(test.name, func(t *testing.T) {
			driver := &canvasTestDriver{viewport: render.Rect{Width: 320, Height: 240}}
			installCanvasTestDriver(t, driver)
			canvas := &Canvas2D{device: render.ActiveDevice(), width: 320, height: 240, viewMatrix: mgl32.Ident4(), blend: render.PremultipliedAlphaBlend}
			textures := [2]*Texture{CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 1, 1))), CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 1, 1)))}
			for phase := range 2 {
				if phase == 1 {
					test.change(canvas)
				}
				for _, texture := range textures {
					canvas.BindTexture(texture)
					canvas.DrawMatrices(mgl32.Ident4(), mgl32.Ident3(), 1)
				}
			}
			if err := canvas.FinishFrame(); err != nil {
				t.Fatal(err)
			}
			if len(driver.spriteBatches) != 2 {
				t.Fatalf("sprite batches = %d, want 2", len(driver.spriteBatches))
			}
			for _, batch := range driver.spriteBatches {
				if len(batch.Draws) != 1 || batch.Draws[0].InstanceCount != 2 || batch.Instances[0].TextureSlot != 0 || batch.Instances[1].TextureSlot != 1 {
					t.Fatalf("unexpected batch at state boundary: %+v", batch)
				}
			}
		})
	}
}

func BenchmarkCanvasSpriteMultiTexture1000(b *testing.B) {
	for _, count := range []int{1, 4, 5} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			driver := &canvasTestDriver{viewport: render.Rect{Width: 320, Height: 240}, discardBatches: true}
			render.Uninstall()
			if err := render.Install(driver); err != nil {
				b.Fatal(err)
			}
			b.Cleanup(render.Uninstall)
			canvas := &Canvas2D{device: render.ActiveDevice(), width: 320, height: 240, viewMatrix: mgl32.Ident4(), blend: render.PremultipliedAlphaBlend}
			textures := make([]*Texture, count)
			for index := range textures {
				textures[index] = CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 1, 1)))
			}
			draw := func() {
				for index := range 1000 {
					canvas.BindTexture(textures[index%count])
					canvas.DrawMatrices(mgl32.Ident4(), mgl32.Ident3(), 1)
				}
				if err := canvas.FinishFrame(); err != nil {
					b.Fatal(err)
				}
			}
			draw()
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				draw()
			}
		})
	}
}
