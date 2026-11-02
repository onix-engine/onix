//go:build js && wasm && (wechat || onix_webgl) && !onix_directx && !onix_metal && !onix_vulkan && !onix_opengl && !onix_webgpu

package render

import (
	"syscall/js"
	"testing"
)

func TestWebGLInstanceStorageRetention(t *testing.T) {
	gl := js.Global().Get("Function").New(`return {
		stores: 0, uploads: 0, bindBuffer() {},
		bufferData(_, size) { this.stores++; this.storage = new Uint8Array(size); },
		bufferSubData(_, offset, data, start, length) {
			this.uploads++;
			if (offset + length > this.storage.length) throw new Error("upload out of bounds");
			this.storage.set(data.subarray(start, start + length), offset);
		}
	}`).Invoke()
	d := webGL2Driver{gl: gl}
	geometry := webGLGeometry{instanceBuffer: webGLObject()}
	small := js.Global().Get("Uint8Array").New(128)
	large := js.Global().Get("Uint8Array").New(1 << 20)
	for range bufferTrimDelay * 2 {
		d.beginInstanceUploads(&geometry)
		d.uploadInstances(&geometry, large, 1<<20)
		d.uploadInstances(&geometry, small, 128)
		if geometry.instanceBytes != 1<<20 {
			t.Fatal("last small batch hid the frame peak")
		}
	}
	// The last high frame is observed first; the following window is all small.
	for range bufferTrimDelay + 1 {
		d.beginInstanceUploads(&geometry)
		d.uploadInstances(&geometry, small, 128)
	}
	if geometry.instanceBytes != 1024 {
		t.Fatalf("low-load capacity = %d", geometry.instanceBytes)
	}
	for frame := range 32 {
		d.beginInstanceUploads(&geometry)
		stores := gl.Get("stores").Int()
		var prior js.Value
		for batch := range 900 {
			small.SetIndex(0, (frame+batch)%251)
			d.uploadInstances(&geometry, small, 128)
			if batch == 0 {
				prior = gl.Get("storage")
			}
		}
		if prior.Index(0).Int() != frame || gl.Get("stores").Int()-stores != 900 {
			t.Fatal("orphaning failed to preserve prior draws")
		}
		if geometry.instanceBytes != 1024 {
			t.Fatal("steady batches grew retained capacity")
		}
	}
	d.uploadInstances(&geometry, large, 1<<20)
	d.beginInstanceUploads(&geometry)
	for range bufferTrimDelay {
		d.beginInstanceUploads(&geometry)
	}
	if geometry.instanceBytes != 1024 || gl.Get("storage").Get("byteLength").Int() != 1024 {
		t.Fatal("idle view retained actual GPU storage at its peak")
	}
	t.Log("1 MiB -> 1 KiB GPU capacity, largest-batch tracking, idle release and 900 interleaved orphaned uploads passed")
}

func TestBrowserWebGLStreamPixels(t *testing.T) {
	if !js.Global().Get("document").Truthy() {
		t.Skip("requires a browser")
	}
	canvas := js.Global().Get("document").Call("createElement", "canvas")
	canvas.Set("width", 64)
	canvas.Set("height", 32)
	driver, _, check := newSpriteBrowserDriver(t, canvas)
	defer driver.Destroy()
	targets := driver.(RenderTargetDriver)
	target, _, err := targets.CreateRenderTarget(RenderTargetDescriptor{Width: 64, Height: 32})
	if err != nil {
		t.Fatal(err)
	}
	defer targets.DestroyRenderTarget(target)
	texture, err := driver.CreateTexture(TextureDescriptor{Width: 1, Height: 1}, []byte{255, 255, 255, 255})
	if err != nil {
		t.Fatal(err)
	}
	defer driver.DestroyTexture(texture)
	observer := js.Global().Get("Function").New("gl", `
		const original = gl.bufferData, stats = { calls: 0, bytes: 0 };
		gl.bufferData = function(target, data, usage) {
			stats.calls++; stats.bytes += typeof data === "number" ? data : data.byteLength;
			return original.call(gl, target, data, usage);
		};
		return { stats, restore() { gl.bufferData = original; } };
	`).Invoke(driver.(*webGL2Driver).gl)
	defer observer.Call("restore")
	projection := [16]float32{0: 1, 5: 1, 10: 1, 15: 1}
	model := [6]float32{2, 0, -1, 0, 2, -1}
	sprites := SpriteBatch{Projection: projection, Instances: []Instance2D{{Model: model,
		TextureModel: [6]float32{1, 0, 0, 0, 1, 0}, Color: [4]float32{1, 0, 0, 1}}},
		Draws: []SpriteDraw{{Texture: texture, InstanceCount: 1}}}
	shapes := ShapeBatch{Projection: projection, Instances: []ShapeInstance2D{{Model: [6]float32{4, 0, -2, 0, 4, -2},
		LocalModel: [6]float32{1, 0, 0, 0, 1, 0}, Fill: [4]float32{0, 1, 0, 1}, Params: [4]float32{32, 32}, Kind: ShapeRoundedRect}}}
	pixels := make([]byte, 64*32*4)
	start := js.Global().Get("performance").Call("now").Float()
	for frame := range 7 {
		if err := driver.BeginFrame(Color{}); err != nil {
			t.Fatal(err)
		}
		if err := targets.BeginRenderTarget(target, RenderTargetClear{ClearColor: true}); err != nil {
			t.Fatal(err)
		}
		driver.SetViewport(Rect{Width: 64, Height: 32})
		for batch := range 2048 {
			driver.SetScissor(&Rect{X: int32(batch % 64), Y: int32(batch / 64), Width: 1, Height: 1})
			if (batch+frame)%2 == 0 {
				err = driver.DrawSprites(sprites)
			} else {
				err = driver.DrawShapes(shapes)
			}
			if err != nil {
				t.Fatal(err)
			}
		}
		driver.SetScissor(nil)
		if err := targets.EndRenderTarget(); err != nil {
			t.Fatal(err)
		}
		if err := driver.Present(); err != nil {
			t.Fatal(err)
		}
		if err := driver.(RenderTargetReadbackDriver).ReadRenderTarget(target, RenderTargetRegion{Width: 64, Height: 32}, pixels); err != nil {
			t.Fatal(err)
		}
		for batch := range 2048 {
			pixel := pixels[batch*4 : batch*4+4]
			green := (batch + frame) % 2
			if pixel[green] != 255 || pixel[1-green] != 0 || pixel[2] != 0 || pixel[3] != 255 {
				t.Fatalf("frame %d tile %d: %v", frame, batch, pixel)
			}
		}
		check()
	}
	t.Logf("7 frames x 2048 interleaved sprite/shape draws: bufferData=%d, specified=%d KiB, elapsed=%.1f ms, all tile pixels passed",
		observer.Get("stats").Get("calls").Int(), observer.Get("stats").Get("bytes").Int()/1024,
		js.Global().Get("performance").Call("now").Float()-start)
	large := make([]ShapeInstance2D, 8192)
	large[0] = shapes.Instances[0]
	small := shapes.Instances
	d := driver.(*webGL2Driver)
	for frame := range (bufferTrimDelay + 4) * webGLFramesInFlight {
		if err := driver.BeginFrame(Color{}); err != nil {
			t.Fatal(err)
		}
		if frame < webGLFramesInFlight {
			shapes.Instances = large
		} else {
			shapes.Instances = small
		}
		if err := targets.BeginRenderTarget(target, RenderTargetClear{ClearColor: true}); err != nil {
			t.Fatal(err)
		}
		if err := driver.DrawShapes(shapes); err != nil {
			t.Fatal(err)
		}
		if err := targets.EndRenderTarget(); err != nil {
			t.Fatal(err)
		}
		if err := driver.Present(); err != nil {
			t.Fatal(err)
		}
	}
	var pixel [4]byte
	if err := driver.(RenderTargetReadbackDriver).ReadRenderTarget(target, RenderTargetRegion{X: 16, Y: 16, Width: 1, Height: 1}, pixel[:]); err != nil {
		t.Fatal(err)
	}
	if pixel != [4]byte{0, 255, 0, 255} {
		t.Fatalf("pixel after shrink = %v", pixel)
	}
	for i := range d.shapeGeometry {
		geometry := &d.shapeGeometry[i]
		d.bindArrayBuffer(geometry.instanceBuffer)
		if geometry.instanceBytes != 1024 || d.gl.Call("getBufferParameter", webGLArrayBuffer, 0x8764).Int() != 1024 {
			t.Fatalf("slot %d did not release its actual GPU peak", i)
		}
	}
	check()
	t.Log("real WebGL GPU storage shrank from 1 MiB to 1 KiB per shape slot; pixel after shrink passed")
}
