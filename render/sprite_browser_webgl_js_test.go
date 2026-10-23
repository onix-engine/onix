//go:build js && wasm && wechat

package render

import (
	"syscall/js"
	"testing"
)

func newSpriteBrowserDriver(t *testing.T, canvas js.Value) (Driver, [2]ShaderDescriptor, func()) {
	context := canvas.Call("getContext", "webgl2", webGLObject("depth", false, "antialias", canvas.Get("__onixTestAntialias").Truthy()))
	driver, err := NewWebGL2Driver(context)
	if err != nil {
		t.Fatal(err)
	}
	return driver, [2]ShaderDescriptor{
			{WebGL2VertexSource: webGLSpriteVertex},
			{WebGL2FragmentSource: "#version 300 es\nprecision mediump float;\nflat in vec4 _vs2fs_location2;\nout vec4 color;\nvoid main() { color = _vs2fs_location2; }"},
		}, func() {
			t.Helper()
			if code := context.Call("getError").Int(); code != 0 {
				t.Fatalf("WebGL error: 0x%x", code)
			}
		}
}

func meshBrowserFragment() ShaderDescriptor {
	return ShaderDescriptor{WebGL2FragmentSource: "#version 300 es\nprecision highp float;\nout vec4 color;\nvoid main() { color=vec4(0.0,0.0,1.0,1.0); }"}
}

func prepareLazyBrowserCapture(Driver) {}
func assertLazyBrowserResources(t *testing.T, driver Driver, depth, mesh bool) {
	t.Helper()
	d := driver.(*webGL2Driver)
	if (d.surfaceTarget != nil) != depth {
		t.Fatal("unexpected surface depth allocation")
	}
	if d.pickingProgram.value.Truthy() {
		t.Fatal("ordinary rendering compiled picking program")
	}
	hasMesh := d.programs[PipelineMesh].value.Truthy()
	for _, program := range d.meshBuiltins {
		hasMesh = hasMesh || program.value.Truthy()
	}
	if hasMesh != mesh {
		t.Fatal("unexpected builtin mesh program allocation")
	}
}
func captureLazyBrowserSurface(t *testing.T, driver Driver) func() []byte {
	return func() []byte {
		t.Helper()
		d := driver.(*webGL2Driver)
		pixels := make([]byte, d.width*d.height*4)
		data := js.Global().Get("Uint8Array").New(len(pixels))
		d.gl.Call("bindFramebuffer", webGLFramebuffer, js.Null())
		d.gl.Call("readPixels", 0, 0, d.width, d.height, webGLRGBA, webGLUnsignedByte, data)
		d.gl.Call("bindFramebuffer", webGLFramebuffer, d.surfaceFramebuffer())
		js.CopyBytesToGo(pixels, data)
		row := d.width * 4
		for y := 0; y < d.height/2; y++ {
			a, b := y*row, (d.height-1-y)*row
			for x := 0; x < row; x++ {
				pixels[a+x], pixels[b+x] = pixels[b+x], pixels[a+x]
			}
		}
		return pixels
	}
}

func lazyBrowserPickingPipeline(driver Driver) js.Value {
	return driver.(*webGL2Driver).pickingProgram.value
}
