//go:build js && wasm && !wechat && !onix_directx && !onix_metal && !onix_vulkan && !onix_opengl && !onix_webgl

package render

import (
	"syscall/js"
	"testing"
)

func TestWebGPUFrameBufferPeakShrinkAndBindings(t *testing.T) {
	mock := js.Global().Get("Function").New(`
		const stats = { created: 0, destroyed: 0 };
		return { stats, device: { createBuffer(d) {
			stats.created++;
			return { size: d.size, destroy() { stats.destroyed++; } };
		} }, frame: { begin() {}, setViewport() {}, setScissorRect() {} } };
	`).Invoke()
	d := webGPUDriver{device: mock.Get("device"), frame: mock.Get("frame"), clearValue: webGPUObject(),
		width: 16, height: 16, sampleCount: 1, frameIndex: webGPUFramesInFlight - 1}
	for i := range d.frames {
		d.frames[i].buffer = d.createFrameBuffer(1 << 20)
		d.frames[i].generation = 1
	}
	step := func(used int) {
		t.Helper()
		if err := d.BeginFrame(Color{}); err != nil {
			t.Fatal(err)
		}
		d.frames[d.frameIndex].offset = used
		// This test has no command encoder; emulate submission after recording.
		d.frameActive = false
	}
	for range (bufferTrimDelay + 1) * webGPUFramesInFlight {
		step(600 << 10)
	}
	if mock.Get("stats").Get("created").Int() != webGPUFramesInFlight {
		t.Fatal("sustained frame demand recreated GPU buffers")
	}
	for range (bufferTrimDelay + 2) * webGPUFramesInFlight {
		step(1500)
	}
	for i := range d.frames {
		frame := &d.frames[i]
		if frame.buffer.capacity != 4096 || frame.generation != 2 {
			t.Fatalf("frame %d: capacity=%d generation=%d", i, frame.buffer.capacity, frame.generation)
		}
	}
	if mock.Get("stats").Get("destroyed").Int() != webGPUFramesInFlight {
		t.Fatal("retired peak buffers were not released after slot reuse")
	}
	created := mock.Get("stats").Get("created").Int()
	for range bufferTrimDelay * 2 * webGPUFramesInFlight {
		step(1500)
	}
	if mock.Get("stats").Get("created").Int() != created {
		t.Fatal("warm frames recreated storage")
	}
	d.reserve(2 << 20)
	if d.frames[d.frameIndex].generation != 3 || d.frames[d.frameIndex].buffer.capacity < 2<<20 {
		t.Fatal("regrowth failed to invalidate old buffer bindings")
	}
	for range (bufferTrimDelay*2 + 3) * webGPUFramesInFlight {
		step(0)
	}
	for i := range d.frames {
		if d.frames[i].buffer.capacity != 1024 {
			t.Fatal("idle frame retained peak GPU capacity")
		}
	}
	t.Log("frame GPU capacity 1 MiB -> 4 KiB, stable reuse, regrowth generations and idle release passed")
}

func TestBrowserWebGPUFrameRetentionPixels(t *testing.T) {
	if !js.Global().Get("document").Truthy() {
		t.Skip("requires a browser")
	}
	canvas := js.Global().Get("document").Call("createElement", "canvas")
	canvas.Set("width", 4)
	canvas.Set("height", 4)
	driver, _, check := newSpriteBrowserDriver(t, canvas)
	defer driver.Destroy()
	d := driver.(*webGPUDriver)
	targets := driver.(RenderTargetDriver)
	target, _, err := targets.CreateRenderTarget(RenderTargetDescriptor{Width: 4, Height: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer targets.DestroyRenderTarget(target)
	texture, err := driver.CreateTexture(TextureDescriptor{Width: 1, Height: 1}, []byte{255, 255, 255, 255})
	if err != nil {
		t.Fatal(err)
	}
	defer driver.DestroyTexture(texture)
	instances := make([]Instance2D, 8192)
	instances[0] = Instance2D{Model: [6]float32{2, 0, -1, 0, 2, -1}, TextureModel: [6]float32{1, 0, 0, 0, 1, 0}}
	batch := SpriteBatch{Projection: [16]float32{0: 1, 5: 1, 10: 1, 15: 1},
		Instances: instances, Draws: []SpriteDraw{{Texture: texture, InstanceCount: uint32(len(instances))}}}
	var peak [webGPUFramesInFlight]int
	for frame := range (bufferTrimDelay + 4) * webGPUFramesInFlight {
		if err := driver.BeginFrame(Color{}); err != nil {
			t.Fatal(err)
		}
		if frame >= webGPUFramesInFlight {
			batch.Instances = instances[:1]
			batch.Draws[0].InstanceCount = 1
		}
		instances[0].Color = [4]float32{3: 1}
		instances[0].Color[frame%2] = 1
		if err := targets.BeginRenderTarget(target, RenderTargetClear{ClearColor: true}); err != nil {
			t.Fatal(err)
		}
		driver.SetViewport(Rect{Width: 4, Height: 4})
		if err := driver.DrawSprites(batch); err != nil {
			t.Fatal(err)
		}
		if frame < webGPUFramesInFlight {
			peak[d.frameIndex] = d.frames[d.frameIndex].buffer.capacity
		}
		if err := targets.EndRenderTarget(); err != nil {
			t.Fatal(err)
		}
		if err := driver.Present(); err != nil {
			t.Fatal(err)
		}
		if frame%15 == 0 || frame == (bufferTrimDelay+4)*webGPUFramesInFlight-1 {
			var pixel [4]byte
			if err := driver.(RenderTargetReadbackDriver).ReadRenderTarget(target, RenderTargetRegion{X: 2, Y: 2, Width: 1, Height: 1}, pixel[:]); err != nil {
				t.Fatal(err)
			}
			if pixel[frame%2] != 255 || pixel[1-frame%2] != 0 || pixel[3] != 255 {
				t.Fatalf("frame %d: pixel %v", frame, pixel)
			}
		}
		check()
	}
	for i := range d.frames {
		if peak[i] < 512<<10 || d.frames[i].buffer.capacity > 4096 {
			t.Fatalf("slot %d: peak=%d retained=%d", i, peak[i], d.frames[i].buffer.capacity)
		}
	}
	t.Logf("real WebGPU frame capacities %v -> %d bytes per slot; pixels and validation scopes passed", peak, d.frames[0].buffer.capacity)
}
