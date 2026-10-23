# Renderer resource metadata and upload reuse

Recorded 2026-10-17 after [temporary-memory reuse](render-memory-reuse.md). Vulkan and Metal store Mesh/RenderTarget metadata directly in `GenerationPool` slots, eliminating per-resource Go wrappers. Shared `nativeMesh`/`nativeRenderTarget` values avoid another pool. `getRef` checks ID, occupancy and generation; borrowed pointers last only until that pool's next Insert, Remove or Clear. Callers do not retain them across pool mutations. `Get` remains value-returning and independently inlineable; removal copies the value before clearing and advancing generation.

WebGPU static vertices, indices, Skin/Morph and built-in quad uploads share driver `uploadData` with explicit valid lengths. Static uploads over 4 MiB use temporary JS arrays without growing resident scratch; disposal clears references. The limit applies to static-triggered resident growth, not all frame uploads.

Measurements use Go 1.26/Linux amd64/i5-11500 and Node 22.22.0, CPU 0, six alternating 200ms runs. Native registry benchmarks operate the actual Vulkan tables after warm-up but create no GPU resources or native calls; Metal uses the same representation without local measurement. WebGPU performs CreateMesh/SetMeshDeformation/DestroyMesh with skin, four morph targets and triple-vertex-count indices. Mock writeBuffer and WeakSet count submitted source buffers only, not total JS heap allocation. Results reflect CPU/Go-JS overhead.

Slots retain historical capacity. First table/index growth and first scratch creation allocate; oversized uploads allocate temporary arrays. Tests cover stale handles, reference updates, clear/remove, warm registry allocations, exact upload contents/lengths, static/frame sharing, large-request limits and disposal. Linux, race, DRM and Node checks passed. Metal was not built/run on Apple, and browser GPU visuals were not checked.

## Examples and commands

```sh
go test ./render ./graphics ./scene/render3d -count=1 -parallel=2
go test -race ./render -run 'Test(GenerationPool|VulkanInline|BufferPool|StaticDeformation|MeshDeformation)' -count=1
go test -tags onix_drm ./render -count=1 -parallel=2
GOOS=js GOARCH=wasm go test -tags wechat ./render -exec "$(go env GOROOT)/lib/wasm/go_js_wasm_exec" -count=1
GOOS=js GOARCH=wasm go test ./render -exec "$(go env GOROOT)/lib/wasm/go_js_wasm_exec" -count=1
git diff --check
```

```sh
taskset -c 0 go test ./render -run '^$' \
  -bench 'Benchmark(Native(ResourceRegistry|MeshLookup)|PackMeshDeformations)$' \
  -benchmem -benchtime=200ms -count=6 -cpu=1

GOOS=js GOARCH=wasm taskset -c 0 go test ./render \
  -exec "$(go env GOROOT)/lib/wasm/go_js_wasm_exec" -run '^$' \
  -bench '^BenchmarkWebGPUResourceUploads$' \
  -benchmem -benchtime=200ms -count=6 -cpu=1
```

## Recorded measurements

| Native resource operation | ns/op, before → after | Time change | Go B/op, before → after | Go allocs/op, before → after |
| --- | ---: | ---: | ---: | ---: |
| Mesh register/get/remove, 1 resource | 33.120 → 18.390 | -44.5% | 24 → 0 | 1 → 0 |
| RenderTarget register/get/remove, 1 resource | 33.545 → 9.198 | -72.6% | 16 → 0 | 1 → 0 |
| Mesh register/get/remove, 1,024 resources | 31990.5 → 16130.5 | -49.6% | 24576 → 0 | 1024 → 0 |
| RenderTarget register/get/remove, 1,024 resources | 32051 → 8683 | -72.9% | 16384 → 0 | 1024 → 0 |
| Mesh handle lookup | 1.763 → 1.715 | -2.7% | 0 → 0 | 0 → 0 |
| Animation packing control | 785.15 → 753.00 | -4.1% | 0 → 0 | 0 → 0 |

| WebGPU workload | µs/op, before → after | Time change | Go B/op, before → after | Go allocs/op, before → after | New JS source ArrayBuffer/op, before → after | New JS source bytes/op, before → after |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 64 vertices | 93.835 → 61.703 | -34.2% | 216 → 168 | 16 → 10 | 3 → 0 | 15108 → 0 |
| 10,000 vertices | 4198.945 → 3151.878 | -24.9% | 216 → 168 | 16 → 10 | 3 → 0 | 2360004 → 0 |

Raw records: [render-resource-reuse-after.txt](render-resource-reuse-after.txt), [render-resource-reuse-before.txt](render-resource-reuse-before.txt).

Performance figures describe the recorded workloads and environments, not complete application frame rates. Warm allocation guarantees exclude first use and capacity growth.
