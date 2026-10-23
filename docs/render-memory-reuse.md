# Renderer temporary-memory reuse

Recorded 2026-10-17 against `a11936948c69f23b71af3e0047db542008af8951`. Draw, instance, texture and animated deformation buffers already belonged to drivers; GPU resources retain generation pools, retirement queues and frame synchronization. The added pool covers only synchronous static Skin/Morph packing.

`buffer_pool.go` pools pointers to generic wrappers of pointer-free slices, avoiding slice boxing. Packing overwrites valid data and resets lengths without clearing entire arrays. Vulkan copies to native mapped memory, Metal creates independent native buffers and WebGPU copies to JS before returning scratch, including failures; GPU resources never retain pooled Go arrays. Buffers over 4 MiB release their backing arrays, keeping wrappers. This is a per-buffer retention limit, not a global pool budget.

Animated packing reserves known per-deformation capacity while validating/writing, including the batch header on the first item, with no prescan. WebGL reuses the mipmap deduplication map, clears texture references after success/failure, drops maps over 4096 entries and clears storage on disposal. Upload-byte statistics accumulate during uploads and commit only after whole-batch success.

Go 1.26/Linux amd64/i5-11500 and Node 22.22.0 Wasm measurements pin CPU 0 and alternate old/new binaries for six 200ms runs with -cpu=1. Medians are below. Static tests include validation, packing and synchronous consumption, excluding GPU creation/final native copies. Animation uses 64 joints and four morph weights per item. WebGL uses complete UpdateTextures with mock GPU calls. Go allocation metrics exclude JS, native and GPU memory.

Static-load temporary allocations fall; already-warm animation time is essentially unchanged. First use, pool eviction by GC, growth and over-limit requests allocate. Tests cover exact offsets, stale data, error recovery, ownership, GC, concurrency, mipmap deduplication and error statistics. Linux, race, DRM and Node Wasm checks passed; Metal hardware and browser GPU visuals were not tested.

## Examples and commands

```sh
go test ./render ./graphics ./scene/render3d -count=1 -parallel=2
go test -race ./render -run 'Test(BufferPool|StaticDeformation|MeshDeformation)' -count=1
go test -tags onix_drm ./render -count=1 -parallel=2
GOOS=js GOARCH=wasm go test -tags wechat ./render -exec "$(go env GOROOT)/lib/wasm/go_js_wasm_exec" -count=1
GOOS=js GOARCH=wasm go test ./render -exec "$(go env GOROOT)/lib/wasm/go_js_wasm_exec" -count=1
git diff --check
```

```sh
taskset -c 0 go test ./render -run '^$' \
  -bench 'Benchmark(MeshDeformation(Static|Animated)|PackMeshDeformations)$' \
  -benchmem -benchtime=200ms -count=6 -cpu=1

GOOS=js GOARCH=wasm taskset -c 0 go test -tags wechat ./render \
  -exec "$(go env GOROOT)/lib/wasm/go_js_wasm_exec" -run '^$' \
  -bench '^BenchmarkWebGLTextureUpdates$' -benchmem -benchtime=200ms -count=6 -cpu=1
```

## Recorded measurements

| Workload | Time µs/op, before → after | Time change | B/op, before → after | allocs/op, before → after |
| --- | ---: | ---: | ---: | ---: |
| Static deformation, 64 vertices / 4 morph targets | 5.853 → 4.077 | -30.3% | 12288 → 0 | 1 → 0 |
| Static deformation, 1,024 vertices / 4 morph targets | 86.350 → 66.099 | -23.5% | 188416 → 0 | 1 → 0 |
| Static deformation, 10,000 vertices / 4 morph targets | 835.431 → 623.692 | -25.3% | 1761280 → 0 | 1 → 0 |
| Animated deformation, 1 item / cold buffer | 2.574 → 1.301 | -49.5% | 12896 → 4864 | 9 → 1 |
| Animated deformation, 1 item / warm buffer | 0.888 → 0.895 | +0.7% | 0 → 0 | 0 → 0 |
| Animated deformation, 64 items / cold buffer | 150.911 → 138.570 | -8.2% | 1171328 → 1158784 | 16 → 13 |
| Animated deformation, 64 items / warm buffer | 47.943 → 47.532 | -0.9% | 0 → 0 | 0 → 0 |
| Existing PackMeshDeformations | 0.774 → 0.759 | -2.0% | 0 → 0 | 0 → 0 |
| WebGL texture updates, 1 texture / 4 updates | 10.157 → 9.137 | -10.0% | 336 → 0 | 2 → 0 |
| WebGL texture updates, 64 textures / 256 updates | 674.375 → 612.509 | -9.2% | 9368 → 0 | 11 → 0 |

Raw records: [render-memory-reuse-after.txt](render-memory-reuse-after.txt), [render-memory-reuse-before.txt](render-memory-reuse-before.txt).

Performance figures describe the recorded workloads and environments, not complete application frame rates. Warm allocation guarantees exclude first use and capacity growth.
