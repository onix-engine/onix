# Renderer memory reuse: first implementation round

Recorded 2026-10-30 on Go 1.27.3, Linux amd64, Intel i5-11500. The baseline is the working tree at the start of this round, including its existing uncommitted HTML changes. A saved source snapshot was applied through Go's overlay support for the before measurements.

## Changes

HTML opacity composition and box shadows borrow pooled raster scratch instead of retaining filter buffers on every node. Children finish rasterizing before their parent borrows scratch, allowing a serial subtree traversal to reuse the same storage. The pool owns only temporary paint images, filter rows, kernels, masks and source copies. Returned raster images remain independently owned. A scratch entry drops its buffers when their combined capacity exceeds 32 MiB; this is a per-entry limit, not a global pool budget.

Generated background, shadow and opacity textures share their retained raster pixels through `graphics.CreateTextureBySharedImage`. This removes the second CPU image while preserving the payload for additional devices and device recreation. `UpdateRegions` still updates every resident device and advances the texture revision; it skips the CPU copy when the source already addresses the same complete rows. Resizing detaches the new raster before painting, preserving the old texture's pixels until its owner releases it. Existing image constructors continue to copy caller pixels.

Font rasterization pools contiguous scaled points, contour views, LCD sample masks and large outline-expansion scratch. LCD output and cached grayscale masks keep separate storage. TrueType decoding writes coordinates directly into the returned points; small flags and contour expansion use stack arrays. Quadratic flattening appends directly into the result instead of allocating arrays at every recursive split. Malformed flag repeats beyond the declared point count now return an error. Each font scratch entry drops buffers above 1 MiB.

Canvas and 3D view buffers shrink after 30 observations below one quarter of their capacity, retaining twice the recent maximum and small capacity floors. Canvas records every batch flushed during the frame. 3D views aggregate multiple draw passes and reclaim extracted snapshots only while their phase aliases are reset. Live values survive shrinking. Idle oversized buffers release their arrays. Shrinking and later growth can allocate; steady use does not allocate in this bookkeeping. Reclamation advances with frame/extraction calls, not wall-clock time. Retained 2D node blocks and map capacity are outside this round.

## Measurements

Three 200 ms samples, `-cpu=1`; table values are medians. B/op measures cumulative Go allocation, not retained heap, RSS, native memory or GPU memory. Timings were not isolated enough to claim a frame-rate improvement.

| Workload | Before B/op | After B/op | Before allocs/op | After allocs/op |
| --- | ---: | ---: | ---: | ---: |
| Create one 128×128 shadow raster and its deferred texture | 395472 | 67568 | 16 | 13 |
| Create 16 independent 128×128 shadow rasters and textures | 6313344 | 1066892 | 155 | 107 |
| 18px-range `A`, new grayscale size | 704 | 288 | 6 | 3 |
| 18px-range `渲`, new grayscale size | 2768 | 480 | 14 | 3 |
| 18px `A`, LCD rasterization | 2144 | 960 | 7 | 2 |
| 18px `渲`, LCD rasterization | 5360 | 1600 | 15 | 2 |
| Decode TrueType `A` | 324 | 228 | 6 | 4 |
| Decode TrueType `渲` | 920 | 552 | 7 | 4 |

Font benchmarks retain the source outline, isolating rasterization from outline regeneration after GC. They include newly returned bitmap allocations. The shadow population benchmark creates fresh node caches and disposes their textures each iteration, while pooled intermediates are reused. For a 1024×1024 retained RGBA raster, shared texture ownership removes one 4 MiB CPU copy; this is payload accounting, not measured RSS.

Capacity tests exercise Canvas bursts of 8192 reserved instances followed by one-instance frames: instance capacity falls to 256, and unused oversized draw/shape buffers release their storage. 3D tests start with capacity 4096 in each array; after sustained small demand, extracted/instance/draw capacities become 64, vertex capacity 256, index capacity 384, and idle deformation capacity zero. Tests verify values, earlier batches and subsequent allocation-free reuse.

Warm Sprite submission, 3D preparation, HTML text updates, hover and stylesheet reuse remain 0 B/op and 0 allocs/op in the recorded workloads. The existing 10,000-object scroll benchmark warms only eight frames, so its measurement includes a one-time delayed shrink: 272 B/op averaged over 100 iterations, with allocs/op rounded to zero. Color animation similarly reports 38 B/op averaged over its run. These figures are not an absolute zero-allocation claim for every animation or capacity transition.

Raw results: [before](render-memory-round1-before.txt), [after](render-memory-round1-after.txt), [warm paths](render-memory-round1-warm.txt).

## Validation

Passed:

- Package tests: `./render ./graphics ./font ./text ./scene ./scene/render3d ./ui/html ./ui ./module ./internal/reuse`, with `-p 1 -parallel=2`.
- Targeted race checks for scratch ownership, concurrent HTML painting, shared texture updates, malformed glyph data and buffer reclamation.
- `go vet` on graphics, font, HTML rendering, 3D rendering and reuse packages; `git diff --check`.
- Font pixel/geometry SHA-256 recordings from the original implementation, covering five characters, four sizes, four subpixel phases, composite glyphs, wide LCD masks and GC between characters. Existing HTML filter, opacity, shadow and animation pixel tests passed.
- Android x86_64 targeted tests on `10.0.0.174:16416`, using NDK 27.2.12479018, `x86_64-linux-android24-clang`, `GOOS=android GOARCH=amd64 CGO_ENABLED=1` and `-trimpath`. Regular/bold Noto Sans SC files accompanied the binaries. Tests cover pixels, pool ownership, shared texture updates/recreation and retention; they do not exercise an Android native rendering surface.

No editor was launched. Visual acceptance remains manual.

```sh
go test -p 1 ./font ./ui/html -run '^$' \
  -bench '^(BenchmarkGlyphRasterization|BenchmarkHTMLRasterPopulation)$' \
  -benchmem -benchtime=200ms -count=3 -cpu=1
```
