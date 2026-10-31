//go:build js && wasm && wechat

package render

import (
	"fmt"
	"syscall/js"
	"testing"
)

func TestWebGLTextureUpdatesDeduplicateAndClearScratch(t *testing.T) {
	gl := js.Global().Get("Function").New(`return {
		bound: null, generated: [], uploads: 0,
		bindTexture(_, texture) { this.bound = texture; },
		texSubImage2D() { this.uploads++; },
		generateMipmap() { this.generated.push(this.bound); }
	}`).Invoke()
	d := webGL2Driver{gl: gl}
	first := d.textures.Insert(webGLTexture{value: js.Global().Get("Object").New(), filter: TextureFilterLinearMipmapLinear})
	second := d.textures.Insert(webGLTexture{value: js.Global().Get("Object").New(), filter: TextureFilterLinearMipmapLinear})
	linear := d.textures.Insert(webGLTexture{value: js.Global().Get("Object").New()})
	update := TextureUpdate{Texture: first, Region: TextureRegion{Width: 1, Height: 1}, Pixels: []byte{1, 2, 3, 4}}
	other, unfiltered, invalid := update, update, update
	other.Texture, unfiltered.Texture, invalid.Texture = second, linear, TextureHandle{}
	if err := d.UpdateTextures([]TextureUpdate{update, other, update, unfiltered, other}); err != nil {
		t.Fatal(err)
	}
	if gl.Get("generated").Length() != 2 || gl.Get("uploads").Int() != 5 || len(d.mipmapUpdates) != 0 {
		t.Fatal("mipmap updates were not deduplicated or scratch retained JS references")
	}
	if d.frame.TextureUploads != 5 || d.frame.TextureUploadBytes != 20 {
		t.Fatalf("incorrect upload statistics: %+v", d.frame)
	}
	if err := d.UpdateTextures([]TextureUpdate{update, invalid}); err == nil || len(d.mipmapUpdates) != 0 {
		t.Fatal("failed batch retained queued mipmaps")
	}
	if d.frame.TextureUploads != 5 || d.frame.TextureUploadBytes != 20 {
		t.Fatal("failed batch committed statistics")
	}
	if err := d.UpdateTextures([]TextureUpdate{other}); err != nil {
		t.Fatal(err)
	}
	if gl.Get("generated").Length() != 3 || len(d.mipmapUpdates) != 0 {
		t.Fatal("failed batch leaked mipmaps into the next upload")
	}
	if err := d.UpdateTextures(nil); err != nil || gl.Get("generated").Length() != 3 {
		t.Fatal("empty batch submitted mipmaps")
	}
}

func TestWebGLTextureUpdatesBatchKeepsPixelsAndOrder(t *testing.T) {
	gl := js.Global().Get("Function").New(`return {
		bound: null, uploads: [], generated: [],
		activeTexture(unit) { this.activeUnit = unit; },
		bindTexture(_, texture) { this.bound = texture; },
		texSubImage2D(_, _level, x, _y, _width, _height, _format, _type, pixels, offset) {
			this.uploads.push([this.bound, x, pixels[offset]]);
		},
		generateMipmap() { this.generated.push(this.bound); }
	}`).Invoke()
	d := webGL2Driver{gl: gl}
	first := d.insertTexture(webGLTexture{value: js.Global().Get("Object").New(), filter: TextureFilterLinearMipmapLinear, format: TextureFormatRGBA8})
	second := d.insertTexture(webGLTexture{value: js.Global().Get("Object").New(), filter: TextureFilterLinearMipmapLinear, format: TextureFormatRGBA8})
	updates := make([]TextureUpdate, 8)
	for i := range updates {
		handle := first
		if i%2 != 0 {
			handle = second
		}
		updates[i] = TextureUpdate{Texture: handle, Region: TextureRegion{X: i, Width: 1, Height: 1}, Pixels: []byte{byte(i + 1), 0, 0, 255}}
	}
	if err := d.UpdateTextures(updates); err != nil {
		t.Fatal(err)
	}
	uploads := gl.Get("uploads")
	if !d.textureBatchCall.Truthy() || uploads.Length() != 8 || gl.Get("generated").Length() != 2 {
		t.Fatalf("batch count=%d mipmaps=%d", uploads.Length(), gl.Get("generated").Length())
	}
	if got := gl.Get("activeUnit").Int(); got != webGLTexture0 {
		t.Fatalf("batch uploaded on texture unit %d", got)
	}
	for i := range updates {
		entry := uploads.Index(i)
		if entry.Index(1).Int() != i || entry.Index(2).Int() != i+1 {
			t.Fatalf("upload %d lost region order or pixels", i)
		}
	}
	if d.frame.TextureUploads != 8 || d.frame.TextureUploadBytes != 32 {
		t.Fatalf("batch statistics = %+v", d.frame)
	}
	invalid := append([]TextureUpdate(nil), updates...)
	invalid[len(invalid)-1].Texture = TextureHandle{}
	if err := d.UpdateTextures(invalid); err == nil || uploads.Length() != 8 || d.frame.TextureUploads != 8 {
		t.Fatal("invalid batch reached WebGL or changed upload statistics")
	}
}

func BenchmarkWebGLTextureUpdates(b *testing.B) {
	for _, textures := range []int{1, 64} {
		b.Run(fmt.Sprintf("textures=%d", textures), func(b *testing.B) {
			gl := js.Global().Get("Function").New(`return {bindTexture() {}, texSubImage2D() {}, generateMipmap() {}}`).Invoke()
			d := webGL2Driver{gl: gl}
			updates := make([]TextureUpdate, textures*4)
			for i := range textures {
				handle := d.textures.Insert(webGLTexture{value: js.Global().Get("Object").New(), filter: TextureFilterLinearMipmapLinear})
				for j := range 4 {
					updates[i*4+j] = TextureUpdate{Texture: handle, Region: TextureRegion{Width: 1, Height: 1}, Pixels: []byte{1, 2, 3, 4}}
				}
			}
			if err := d.UpdateTextures(updates); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if err := d.UpdateTextures(updates); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
