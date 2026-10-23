# WebGL upload views and WebGPU retirement references

Recorded 2026-10-17 after [resource reuse](render-resource-reuse.md). WebGL `copySliceToJS` returns the driver's retained Uint8Array rather than allocating a subarray wrapper. Sprite, Shape, static/dynamic Mesh and picking uploads pass explicit offsets and lengths; quad initialization shares existing scratch.

Lighting copies the full block into a retained TypedArray once, then uses offset/length sections for eleven matrix/vector uniforms, replacing eleven Go-to-JS copies. ASTC mip uploads and readbacks likewise use ranges without temporary views. Uint8Array offsets are bytes; Float32Array offsets are elements, following WebGL2 overload semantics.

WebGPU retirement clears each old js.Value during its existing destroy traversal, keeping capacity until driver disposal. No separate cleanup pass is added; released objects await ordinary GC, with no claimed heap-byte/time benefit for this change alone.

Go 1.26/Linux amd64/i5-11500/Node 22.22.0 measurements alternate six 200ms CPU-0 runs. Dynamic DrawMeshes includes geometry/lighting; shapes use DrawShapes; static cases create/destroy meshes. Scratch, uniform lookups and resource slots are warmed. Mock GL and WeakSet count first submitted TypedArrays and backing ArrayBuffers, excluding unsubmitted wrappers and other JS heap allocations. Go metrics exclude native/JS/GPU memory and cannot imply GPU frame rates.

Tests check precise ranges and source-object reuse for long/short shapes, mesh/picking, all lighting sections, four ASTC mips, readback orientation/bounds, retirement count, cleared references and queue reuse/disposal. Warm dynamic paths are allocation-free; initialization, growth, first uniform lookup and static resource JS calls still allocate. Linux and Node Wasm checks passed without browser GPU or WeChat device visual validation.

## Examples and commands

```sh
go test ./render ./graphics ./scene/render3d -count=1 -parallel=2
GOOS=js GOARCH=wasm go test -tags wechat ./render \
  -exec "$(go env GOROOT)/lib/wasm/go_js_wasm_exec" -count=1
GOOS=js GOARCH=wasm go test ./render \
  -exec "$(go env GOROOT)/lib/wasm/go_js_wasm_exec" -count=1
git diff --check
```

```sh
GOOS=js GOARCH=wasm taskset -c 0 go test -tags wechat ./render \
  -exec "$(go env GOROOT)/lib/wasm/go_js_wasm_exec" -run '^$' \
  -bench '^BenchmarkWebGL(Shape|Mesh|DynamicMesh)Uploads$' \
  -benchmem -benchtime=200ms -count=6 -cpu=1
```

## Recorded measurements

| Workload | µs/op, before → after | Time change | Go B/op, before → after | Go allocs/op, before → after | New JS source ArrayBuffer/op, before → after | New submitted TypedArray views/op, before → after |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Dynamic mesh, 64 vertices | 181.229 → 57.811 | -68.1% | 104 → 0 | 13 → 0 | 2 → 0 | 5 → 0 |
| Dynamic mesh, 10,000 vertices | 192.906 → 80.970 | -58.0% | 104 → 0 | 13 → 0 | 2 → 0 | 5 → 0 |
| Shapes, 64 instances | 12.073 → 4.832 | -60.0% | 8 → 0 | 1 → 0 | 0 → 0 | 1 → 0 |
| Shapes, 4,096 instances | 31.651 → 25.664 | -18.9% | 8 → 0 | 1 → 0 | 0 → 0 | 1 → 0 |
| Static mesh create/destroy, 64 vertices | 31.977 → 24.756 | -22.6% | 32 → 16 | 4 → 2 | 0 → 0 | 2 → 0 |
| Static mesh create/destroy, 10,000 vertices | 58.911 → 51.510 | -12.6% | 32 → 16 | 4 → 2 | 0 → 0 | 2 → 0 |

Raw records: [render-webgl-upload-reuse-after.txt](render-webgl-upload-reuse-after.txt), [render-webgl-upload-reuse-before.txt](render-webgl-upload-reuse-before.txt).

Performance figures describe the recorded workloads and environments, not complete application frame rates. Warm allocation guarantees exclude first use and capacity growth.
