//go:build js && wasm && !wechat

package render

import (
	"bytes"
	"fmt"
	"syscall/js"
	"testing"
)

func TestWebGPUStaticUploadsReuseSourceAndPreserveExactPayloads(t *testing.T) {
	mock := js.Global().Get("Function").New(`
		const calls = [];
		return {
			calls,
			device: { createBuffer(d) { return { size: d.size, destroy() {} }; } },
			queue: { writeBuffer(buffer, offset, data, dataOffset = 0, size = data.byteLength - dataOffset) {
				if (offset + size > buffer.size) throw new Error("upload exceeds destination");
				calls.push({ source: data.buffer, offset, bytes: data.slice(dataOffset, dataOffset + size) });
			} }
		};
	`).Invoke()
	d := webGPUDriver{device: mock.Get("device"), queue: mock.Get("queue")}
	vertices := []Vertex3D{{Position: [3]float32{1, 2, 3}}, {UV: [2]float32{.25, .75}}, {Normal: [3]float32{0, 1, 0}}}
	indices := []uint32{0, 1, 2}
	descriptor := MeshDeformationDescriptor{VertexCount: 3, Skin: make([]MeshSkinVertex, 3)}
	words, _, err := packMeshDeformationStatic(nil, descriptor)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := d.CreateMesh(vertices, indices)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.SetMeshDeformation(handle, descriptor); err != nil {
		t.Fatal(err)
	}
	mesh, _ := d.meshes.Get(handle)
	if mesh.deformation.vertexCount != 3 {
		t.Fatal("static deformation metadata was lost")
	}
	d.frames[0].buffer.value = webGPUObject("size", 16)
	d.writeBuffer(8, []byte{9, 8, 7, 6})
	calls := mock.Get("calls")
	if calls.Length() != 4 {
		t.Fatalf("got %d uploads, want 4", calls.Length())
	}
	for i, expected := range [][]byte{sliceBytes(vertices), sliceBytes(indices), sliceBytes(words), {9, 8, 7, 6}} {
		call := calls.Index(i)
		if !call.Get("source").Equal(calls.Index(0).Get("source")) {
			t.Fatalf("upload %d allocated another JS source buffer", i)
		}
		actual := make([]byte, call.Get("bytes").Get("byteLength").Int())
		js.CopyBytesToGo(actual, call.Get("bytes"))
		if !bytes.Equal(actual, expected) {
			t.Fatalf("upload %d changed its payload", i)
		}
	}
	if calls.Index(3).Get("offset").Int() != 8 {
		t.Fatal("frame upload offset changed after a static upload")
	}
}

func TestWebGPUOversizedStaticUploadsKeepWarmScratchBounded(t *testing.T) {
	mock := js.Global().Get("Function").New(`return {
		calls: [], writeBuffer(_, offset, data, start = 0, size = data.byteLength) {
			this.calls.push({ buffer: data.buffer, size });
		}
	}`).Invoke()
	d := webGPUDriver{queue: mock}
	d.writeStaticBuffer(webGPUObject(), []byte{1, 2, 3, 4})
	warm := d.uploadData
	large := make([]byte, webGPUStaticUploadMaxBytes+4)
	for range 2 {
		d.writeStaticBuffer(webGPUObject(), large)
		if !d.uploadData.Equal(warm) {
			t.Fatal("oversized static upload replaced persistent frame scratch")
		}
	}
	calls := mock.Get("calls")
	if calls.Index(1).Get("buffer").Equal(calls.Index(2).Get("buffer")) || calls.Index(1).Get("size").Int() != len(large) {
		t.Fatal("oversized upload retained its source or changed its size")
	}
}

func TestWebGPUDestroyReleasesUploadScratch(t *testing.T) {
	d := webGPUDriver{
		uploadData:  js.Global().Get("Uint8Array").New(1024),
		textureData: js.Global().Get("Uint8Array").New(1024),
	}
	d.Destroy()
	if d.uploadData.Truthy() || d.textureData.Truthy() {
		t.Fatal("destroyed driver retains JS upload storage")
	}
}

func TestWebGPUFrameRetirementClearsReferencesAndKeepsStorage(t *testing.T) {
	mock := js.Global().Get("Function").New(`
		const stats = { destroyed: 0 };
		return {
			stats, resource: { destroy() { stats.destroyed++; } },
			frame: { begin() {}, setViewport() {}, setScissorRect() {} }
		};
	`).Invoke()
	retired := []js.Value{mock.Get("resource"), js.Undefined(), mock.Get("resource")}
	d := webGPUDriver{
		width: 64, height: 64, sampleCount: 1, frameIndex: webGPUFramesInFlight - 1,
		frame: mock.Get("frame"), clearValue: webGPUObject(),
		depth: webGPUDepthBuffer{value: webGPUObject(), view: webGPUObject(), width: 64, height: 64},
	}
	d.frames[0].retired = retired
	if err := d.BeginFrame(Color{}); err != nil {
		t.Fatal(err)
	}
	if mock.Get("stats").Get("destroyed").Int() != 2 || len(d.frames[0].retired) != 0 || cap(d.frames[0].retired) != len(retired) {
		t.Fatal("retirement changed destruction counts or discarded reusable storage")
	}
	for _, value := range retired {
		if !value.IsUndefined() {
			t.Fatal("empty retirement queue retains a JS resource reference")
		}
	}
	retired[0] = mock.Get("resource")
	d.frames[0].retired = retired[:1]
	d.frames[0].destroyRetired()
	if mock.Get("stats").Get("destroyed").Int() != 3 || !retired[0].IsUndefined() {
		t.Fatal("reused retirement storage was not cleared")
	}
}

func TestWebGPUDestroyReleasesRetirementStorage(t *testing.T) {
	mock := js.Global().Get("Function").New(`
		const stats = { destroyed: 0 };
		return { stats, resource: { destroy() { stats.destroyed++; } } };
	`).Invoke()
	retired := []js.Value{mock.Get("resource")}
	d := webGPUDriver{}
	d.frames[1].retired = retired
	d.Destroy()
	if mock.Get("stats").Get("destroyed").Int() != 1 || d.frames[1].retired != nil || !retired[0].IsUndefined() {
		t.Fatal("destroyed driver retained retirement storage or resource references")
	}
}

func BenchmarkWebGPUResourceUploads(b *testing.B) {
	for _, vertexCount := range []int{64, 10000} {
		b.Run(fmt.Sprintf("vertices=%d", vertexCount), func(b *testing.B) {
			// Observe new source ArrayBuffers, including their capacity, at the
			// upload boundary. The WeakSet does not retain retired source arrays.
			mock := js.Global().Get("Function").New(`
				const seen = new WeakSet();
				const stats = { buffers: 0, bytes: 0 };
				return {
					stats,
					device: { createBuffer(d) { return { size: d.size, destroy() {} }; } },
					queue: { writeBuffer(_, offset, data) {
						if (!seen.has(data.buffer)) {
							seen.add(data.buffer);
							stats.buffers++;
							stats.bytes += data.buffer.byteLength;
						}
					} }
				};
			`).Invoke()
			d := webGPUDriver{device: mock.Get("device"), queue: mock.Get("queue")}
			vertices := make([]Vertex3D, vertexCount)
			indices := make([]uint32, vertexCount*3)
			descriptor := MeshDeformationDescriptor{VertexCount: uint32(vertexCount), TargetCount: 4,
				Skin: make([]MeshSkinVertex, vertexCount), MorphTargets: make([]MeshMorphVertex, vertexCount*4)}
			for i := range descriptor.Skin {
				descriptor.Skin[i].Weights[0] = 1
			}
			upload := func() {
				handle, err := d.CreateMesh(vertices, indices)
				if err != nil {
					b.Fatal(err)
				}
				if err := d.SetMeshDeformation(handle, descriptor); err != nil {
					b.Fatal(err)
				}
				if err := d.DestroyMesh(handle); err != nil {
					b.Fatal(err)
				}
			}
			upload()
			stats := mock.Get("stats")
			stats.Set("buffers", 0)
			stats.Set("bytes", 0)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				upload()
			}
			b.StopTimer()
			b.ReportMetric(float64(stats.Get("buffers").Int())/float64(b.N), "jsbuffers/op")
			b.ReportMetric(float64(stats.Get("bytes").Int())/float64(b.N), "jsbytes/op")
		})
	}
}
