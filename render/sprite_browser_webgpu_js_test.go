//go:build js && wasm && !wechat

package render

import (
	"syscall/js"
	"testing"
)

func newSpriteBrowserDriver(t *testing.T, canvas js.Value) (Driver, [2]ShaderDescriptor, func()) {
	antialias := canvas.Get("__onixTestAntialias").Truthy()
	driver, err := NewWebGPUDriver(canvas, false, antialias)
	if err != nil {
		t.Fatal(err)
	}
	device := driver.(*webGPUDriver).device
	device.Call("pushErrorScope", "validation")
	return driver, [2]ShaderDescriptor{
			{WebGPUVertexSource: webGPUSpriteVertex},
			{WebGPUFragmentSource: "@fragment fn sprite_fragment(@location(2) @interpolate(flat) custom: vec4<f32>) -> @location(0) vec4<f32> { return custom; }"},
		}, func() {
			t.Helper()
			value, err := awaitWebGPU(device.Call("popErrorScope"))
			if err != nil {
				t.Fatal(err)
			}
			if value.Truthy() {
				t.Fatal(value.Get("message").String())
			}
			device.Call("pushErrorScope", "validation")
		}
}

func meshBrowserFragment() ShaderDescriptor {
	return ShaderDescriptor{WebGPUFragmentSource: "@fragment fn mesh_fragment() -> @location(0) vec4<f32> { return vec4<f32>(0.0,0.0,1.0,1.0); }"}
}

func prepareLazyBrowserCapture(driver Driver) {
	d := driver.(*webGPUDriver)
	d.context.Call("configure", webGPUObject("device", d.device, "format", d.format, "alphaMode", "opaque", "usage", webGPUTextureRender|webGPUTextureCopySrc))
}

func assertLazyBrowserResources(t *testing.T, driver Driver, depth, mesh bool) {
	t.Helper()
	d := driver.(*webGPUDriver)
	if d.depth.value.Truthy() != depth || d.surfaceDepthEnabled != depth {
		t.Fatal("unexpected surface depth allocation")
	}
	if d.pickingPipeline.Truthy() {
		t.Fatal("ordinary rendering compiled picking pipeline")
	}
	found := false
	for _, variants := range d.programs[PipelineMesh].meshPipelines {
		for _, pipeline := range variants {
			found = found || pipeline.Truthy()
		}
	}
	if found != mesh {
		t.Fatal("unexpected builtin mesh pipeline allocation")
	}
}

func captureLazyBrowserSurface(t *testing.T, driver Driver) func() []byte {
	t.Helper()
	d := driver.(*webGPUDriver)
	buffer := d.device.Call("createBuffer", webGPUObject("size", 256*d.height, "usage", webGPUBufferCopyDst|webGPUBufferMapRead))
	d.frame.Get("pass").Call("end")
	d.frame.Get("encoder").Call("copyTextureToBuffer", webGPUObject("texture", d.context.Call("getCurrentTexture")), webGPUObject("buffer", buffer, "bytesPerRow", 256), webGPUArray(d.width, d.height, 1))
	descriptor := d.frame.Get("descriptor")
	d.frame.Get("colorAttachment").Set("loadOp", "load")
	d.frame.Get("depthAttachment").Set("depthLoadOp", "load")
	d.frame.Set("pass", d.frame.Get("encoder").Call("beginRenderPass", descriptor))
	return func() []byte {
		t.Helper()
		defer buffer.Call("destroy")
		if _, err := awaitWebGPU(buffer.Call("mapAsync", 1)); err != nil {
			t.Fatal(err)
		}
		data := make([]byte, 256*d.height)
		js.CopyBytesToGo(data, js.Global().Get("Uint8Array").New(buffer.Call("getMappedRange")))
		buffer.Call("unmap")
		pixels := make([]byte, d.width*d.height*4)
		for y := 0; y < d.height; y++ {
			copy(pixels[y*d.width*4:], data[y*256:y*256+d.width*4])
		}
		if d.format == "bgra8unorm" {
			for i := 0; i < len(pixels); i += 4 {
				pixels[i], pixels[i+2] = pixels[i+2], pixels[i]
			}
		}
		return pixels
	}
}

func lazyBrowserPickingPipeline(driver Driver) js.Value {
	return driver.(*webGPUDriver).pickingPipeline
}
