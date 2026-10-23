//go:build js && wasm && wechat

package render

import (
	"bytes"
	"fmt"
	"syscall/js"
	"testing"
)

// Count distinct typed array views at the GL boundary, separately from their
// backing ArrayBuffers. Reusing a buffer alone does not eliminate view objects.
func newWebGLUploadDriver() (webGL2Driver, js.Value) {
	mock := js.Global().Get("Function").New(`
		const views = new WeakSet();
		const buffers = new WeakSet();
		const stats = { views: 0, buffers: 0 };
		function observe(data) {
			if (!ArrayBuffer.isView(data)) return;
			if (!views.has(data)) { views.add(data); stats.views++; }
			if (!buffers.has(data.buffer)) { buffers.add(data.buffer); stats.buffers++; }
		}
		const gl = {
			createBuffer() { return {}; }, deleteBuffer() {},
			bindBuffer() {}, bindVertexArray() {},
			bufferData(_, data) { observe(data); },
			bufferSubData(_, offset, data) { observe(data); },
			enable() {}, disable() {}, useProgram() {},
			depthMask() {}, activeTexture() {}, bindTexture() {},
			vertexAttribPointer() {},
			getUniformLocation(_, name) { return name; },
			uniform1i() {}, uniform4f() {},
			uniform4fv(_, data) { observe(data); },
			uniformMatrix4fv(_, transpose, data) { observe(data); },
			drawElementsInstanced() {}
		};
		return { gl, stats };
	`).Invoke()
	d := webGL2Driver{gl: mock.Get("gl"), surfaceTarget: &webGLRenderTarget{}}
	d.programs[PipelineShape] = webGLProgram{value: webGLObject(), projectionSet: true}
	d.shapeGeometry[0] = webGLGeometry{vao: webGLObject(), instanceBuffer: webGLObject()}
	return d, mock.Get("stats")
}

func BenchmarkWebGLDynamicMeshUploads(b *testing.B) {
	for _, count := range []int{64, 10000} {
		b.Run(fmt.Sprintf("vertices=%d", count), func(b *testing.B) {
			d, stats := newWebGLUploadDriver()
			d.programs[PipelineMesh] = webGLProgram{value: webGLObject(), uniforms: make(map[string]js.Value)}
			d.meshBuiltins[meshVariantBasic-1] = d.programs[PipelineMesh]
			d.meshGeometry = webGLMeshGeometry{vao: webGLObject(), vertexBuffer: webGLObject(), indexBuffer: webGLObject(), instanceBuffer: webGLObject()}
			texture := d.insertTexture(webGLTexture{value: webGLObject()})
			batch := MeshBatch3D{Vertices: make([]Vertex3D, count), Indices: make([]uint32, count*3), Instances: []Instance3D{{}},
				Draws: []MeshDraw3D{{Texture: texture, IndexCount: uint32(count * 3), InstanceCount: 1}}}
			if err := d.DrawMeshes(batch); err != nil {
				b.Fatal(err)
			}
			stats.Set("views", 0)
			stats.Set("buffers", 0)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if err := d.DrawMeshes(batch); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(stats.Get("views").Int())/float64(b.N), "jsviews/op")
			b.ReportMetric(float64(stats.Get("buffers").Int())/float64(b.N), "jsbuffers/op")
		})
	}
}

func BenchmarkWebGLShapeUploads(b *testing.B) {
	for _, count := range []int{64, 4096} {
		b.Run(fmt.Sprintf("instances=%d", count), func(b *testing.B) {
			d, stats := newWebGLUploadDriver()
			batch := ShapeBatch{Instances: make([]ShapeInstance2D, count)}
			if err := d.DrawShapes(batch); err != nil {
				b.Fatal(err)
			}
			stats.Set("views", 0)
			stats.Set("buffers", 0)
			b.SetBytes(int64(sliceByteSize(batch.Instances)))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if err := d.DrawShapes(batch); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(stats.Get("views").Int())/float64(b.N), "jsviews/op")
			b.ReportMetric(float64(stats.Get("buffers").Int())/float64(b.N), "jsbuffers/op")
		})
	}
}

func BenchmarkWebGLMeshUploads(b *testing.B) {
	for _, count := range []int{64, 10000} {
		b.Run(fmt.Sprintf("vertices=%d", count), func(b *testing.B) {
			d, stats := newWebGLUploadDriver()
			vertices, indices := make([]Vertex3D, count), make([]uint32, count*3)
			upload := func() {
				handle, err := d.CreateMesh(vertices, indices)
				if err != nil {
					b.Fatal(err)
				}
				if err := d.DestroyMesh(handle); err != nil {
					b.Fatal(err)
				}
			}
			upload()
			stats.Set("views", 0)
			stats.Set("buffers", 0)
			b.SetBytes(int64(sliceByteSize(vertices) + sliceByteSize(indices)))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				upload()
			}
			b.StopTimer()
			b.ReportMetric(float64(stats.Get("views").Int())/float64(b.N), "jsviews/op")
			b.ReportMetric(float64(stats.Get("buffers").Int())/float64(b.N), "jsbuffers/op")
		})
	}
}

func captureWebGLUploads(d *webGL2Driver) js.Value {
	capture := js.Global().Get("Function").New(`
		const calls = [];
		function record(kind, name, data, offset = 0, length = 0) {
			length = length || data.length - offset;
			const bytes = new Uint8Array(data.buffer, data.byteOffset + offset * data.BYTES_PER_ELEMENT, length * data.BYTES_PER_ELEMENT);
			calls.push({ kind, name, source: data, bytes: bytes.slice() });
		}
		return {
			calls,
			bufferData(_, data, usage, offset, length) {
				if (ArrayBuffer.isView(data)) record("buffer", "", data, offset, length);
			},
			bufferSubData(_, offset, data, start, length) { record("buffer", "", data, start, length); },
			uniform4fv(name, data, offset, length) { record("uniform", name, data, offset, length); },
			uniformMatrix4fv(name, transpose, data, offset, length) { record("uniform", name, data, offset, length); },
			compressedTexImage2D(_, level, format, width, height, border, data, offset, length) { record("compressed", String(level), data, offset, length); },
			createTexture() { return {}; }, texParameteri() {},
			readPixels(x, y, width, height, format, type, data) {
				const bytes = new Uint8Array(data.buffer, data.byteOffset, data.byteLength);
				for (let i = 0; i < width * height * 4; i++) bytes[i] = i + 1;
			}
		};
	`).Invoke()
	for _, name := range []string{"bufferData", "bufferSubData", "uniform4fv", "uniformMatrix4fv", "compressedTexImage2D", "createTexture", "texParameteri", "readPixels"} {
		d.gl.Set(name, capture.Get(name))
	}
	return capture.Get("calls")
}

func assertWebGLUploadBytes(t *testing.T, call js.Value, want []byte) {
	t.Helper()
	data := call.Get("bytes")
	got := make([]byte, data.Get("byteLength").Int())
	js.CopyBytesToGo(got, data)
	if !bytes.Equal(got, want) {
		t.Fatalf("%s upload has %d bytes, want %d with matching content", call.Get("name"), len(got), len(want))
	}
}

func TestWebGLBufferUploadsKeepExactPayloadAndReuseArray(t *testing.T) {
	d, _ := newWebGLUploadDriver()
	d.instanceData = js.Global().Get("Uint8Array").New(8192)
	calls := captureWebGLUploads(&d)
	shapes := []ShapeInstance2D{{Kind: 1}, {Kind: 2}}
	for _, instances := range [][]ShapeInstance2D{shapes, shapes[:1]} {
		if err := d.DrawShapes(ShapeBatch{Instances: instances}); err != nil {
			t.Fatal(err)
		}
	}
	vertices := []Vertex3D{{Position: [3]float32{1, 2, 3}}, {UV: [2]float32{.25, .75}}, {Normal: [3]float32{0, 1, 0}}}
	indices := []uint32{0, 1, 2}
	handle, err := d.CreateMesh(vertices, indices)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.DestroyMesh(handle); err != nil {
		t.Fatal(err)
	}
	expected := [][]byte{sliceBytes(shapes), sliceBytes(shapes[:1]), sliceBytes(vertices), sliceBytes(indices)}
	if calls.Length() != len(expected) {
		t.Fatalf("got %d uploads, want %d", calls.Length(), len(expected))
	}
	for index, want := range expected {
		call := calls.Index(index)
		assertWebGLUploadBytes(t, call, want)
		if !call.Get("source").Equal(d.instanceData) {
			t.Fatal("upload allocated a temporary view")
		}
	}
}

func TestWebGLDynamicMeshUniformsShareOneArrayWithCorrectRanges(t *testing.T) {
	d, _ := newWebGLUploadDriver()
	calls := captureWebGLUploads(&d)
	d.programs[PipelineMesh] = webGLProgram{value: webGLObject(), uniforms: make(map[string]js.Value), projectionSet: true}
	d.meshBuiltins[meshVariantBasic-1] = d.programs[PipelineMesh]
	d.meshGeometry = webGLMeshGeometry{vao: webGLObject(), vertexBuffer: webGLObject(), indexBuffer: webGLObject(), instanceBuffer: webGLObject()}
	texture := d.insertTexture(webGLTexture{value: webGLObject()})
	batch := MeshBatch3D{
		Vertices: []Vertex3D{{Position: [3]float32{3, 2, 1}}, {}, {}}, Indices: []uint32{0, 1, 2}, Instances: []Instance3D{{}},
		Draws: []MeshDraw3D{{Texture: texture, IndexCount: 3, InstanceCount: 1}},
		Lights: []Light3D{{PositionDirection: [4]float32{1, 2, 3, 4}, Direction: [4]float32{5, 6, 7},
			Color: [4]float32{.1, .2, .3, .4}, Params: [4]float32{8, 9, 10, 11}, Right: [4]float32{12, 13, 14}, Up: [4]float32{15, 16, 17}}},
	}
	for index := range batch.Lighting.Shadows {
		shadow := &batch.Lighting.Shadows[index]
		for component := range shadow.Matrix {
			shadow.Matrix[component] = float32(index*16 + component + 1)
		}
		shadow.Rect, shadow.Params = [4]float32{.1, .2, .3, .4}, [4]float32{1, 2, 3, 4}
		batch.Lighting.LightShadows[index] = [4]float32{float32(index + 1), 2, 3}
	}
	for repeat := range 2 {
		batch.Lighting.OcclusionInverseProjection[0] = float32(repeat + 7)
		batch.Lights[0].Color[0] = float32(repeat + 1)
		calls.Set("length", 0)
		if err := d.DrawMeshes(batch); err != nil {
			t.Fatal(err)
		}
		uniforms := make([]float32, meshLightUniformFloatCount)
		writeMeshLightUniforms(uniforms, batch.Lights)
		writeMeshLighting(uniforms, batch.Lighting)
		expected := map[string][]float32{
			"shadowMatrices": uniforms[meshShadowMatrixOffset:meshShadowRectOffset], "shadowRects": uniforms[meshShadowRectOffset:meshShadowParamsOffset],
			"shadowParams": uniforms[meshShadowParamsOffset:meshLightShadowOffset], "lightShadows": uniforms[meshLightShadowOffset:meshOcclusionInverseOffset],
			"occlusionInverseProjection": uniforms[meshOcclusionInverseOffset:],
			"lightPosition":              uniforms[32:96], "lightDirections": uniforms[96:160], "lightColors": uniforms[160:224], "lightParams": uniforms[224:288],
			"lightRight": uniforms[meshLightRightOffset:meshLightUpOffset], "lightUp": uniforms[meshLightUpOffset:meshShadowMatrixOffset],
		}
		if calls.Length() != 3+len(expected) {
			t.Fatalf("got %d uploads, want 3 geometry uploads and %d uniform uploads", calls.Length(), len(expected))
		}
		for index, want := range [][]byte{sliceBytes(batch.Vertices), sliceBytes(batch.Indices), sliceBytes(batch.Instances)} {
			assertWebGLUploadBytes(t, calls.Index(index), want)
			if !calls.Index(index).Get("source").Equal(d.instanceData) {
				t.Fatal("dynamic geometry upload created a view")
			}
		}
		for index := 3; index < calls.Length(); index++ {
			call := calls.Index(index)
			name := call.Get("name").String()
			want, ok := expected[name]
			if !ok {
				t.Fatalf("unexpected or duplicate uniform %q", name)
			}
			assertWebGLUploadBytes(t, call, sliceBytes(want))
			if !call.Get("source").Equal(d.meshUniformData.floats) {
				t.Fatal("lighting uniform upload did not use the shared array")
			}
			delete(expected, name)
		}
	}
	d.activeTarget = d.targets.Insert(&webGLRenderTarget{descriptor: RenderTargetDescriptor{ColorFormat: RenderTargetUint32}})
	d.pickingProgram = webGLProgram{value: webGLObject(), projectionSet: true}
	// Picking consumes geometry only, even when material bindings are invalid.
	batch.Draws[0].Texture = TextureHandle{}
	batch.Draws[0].Shader = NewHandle[MeshShaderResource](99, 1)
	batch.Draws[0].AlphaMode = 99
	batch.Draws[0].BlendMode = MeshBlendMode(99)
	calls.Set("length", 0)
	if err := d.DrawMeshPicking(batch); err != nil {
		t.Fatal(err)
	}
	for index, want := range [][]byte{sliceBytes(batch.Vertices), sliceBytes(batch.Indices), sliceBytes(batch.Instances)} {
		assertWebGLUploadBytes(t, calls.Index(index), want)
	}
}

func TestWebGLCompressedTextureRangesAndReadbackReuse(t *testing.T) {
	d, _ := newWebGLUploadDriver()
	calls := captureWebGLUploads(&d)
	d.astcExtension = webGLObject()
	descriptor := TextureDescriptor{Width: 8, Height: 8, Levels: 4, Format: TextureFormatASTC4x4}
	pixels := make([]byte, descriptor.Format.ChainSize(8, 8, 4))
	for index := range pixels {
		pixels[index] = byte(index)
	}
	if _, err := d.CreateTexture(descriptor, pixels); err != nil {
		t.Fatal(err)
	}
	if calls.Length() != 4 {
		t.Fatalf("got %d compressed uploads, want 4", calls.Length())
	}
	for level, mip := range descriptor.MipChain {
		call := calls.Index(level)
		assertWebGLUploadBytes(t, call, pixels[mip.Offset:mip.Offset+mip.Bytes])
		if !call.Get("source").Equal(d.textureData) {
			t.Fatal("compressed upload created a view")
		}
	}
	handle := d.targets.Insert(&webGLRenderTarget{framebuffer: webGLObject(), descriptor: RenderTargetDescriptor{Width: 2, Height: 2}})
	d.gl.Set("bindFramebuffer", js.Global().Get("Function").New(""))
	destination := bytes.Repeat([]byte{0xee}, 20)
	if err := d.ReadRenderTarget(handle, RenderTargetRegion{Width: 2, Height: 2}, destination[:16]); err != nil {
		t.Fatal(err)
	}
	if want := []byte{9, 10, 11, 12, 13, 14, 15, 16, 1, 2, 3, 4, 5, 6, 7, 8, 0xee, 0xee, 0xee, 0xee}; !bytes.Equal(destination, want) {
		t.Fatalf("readback changed row order or wrote beyond destination: %v", destination)
	}
	warm := d.readbackBytes
	if err := d.ReadRenderTarget(handle, RenderTargetRegion{Width: 1, Height: 1}, destination[:4]); err != nil {
		t.Fatal(err)
	}
	if !d.readbackBytes.Equal(warm) || !bytes.Equal(destination[:4], []byte{1, 2, 3, 4}) || destination[4] != 13 {
		t.Fatal("smaller readback replaced storage or copied stale capacity")
	}
}
