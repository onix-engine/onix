# Renderer memory reuse: second implementation round

Recorded 2026-10-30 on Go 1.27.3, Linux amd64, Intel i5-11500. The baseline is the working tree after round one, including the existing unrelated HTML changes. Source snapshots and Go overlays were used to build baseline benchmark binaries without reverting the working tree.

## Changes

Box shadows with sigma ≥4 now blur a single float64 alpha image in place, copying one row or column before overwriting it. The former second full float64 image becomes a line buffer of `max(width, height)` values. Sliding sums retain their floating-point operation order. At 1024×1024 the two working buffers fall from 16 MiB to 8 MiB + 8 KiB. Small shadows keep the existing Gaussian path.

Filter drop-shadow no longer snapshots the full source RGBA image. After constructing the independent shadow mask, compositing reads each original destination pixel once before replacing it. Fractional offsets, Gaussian filtering and alpha composition remain covered by recorded pixel tests. This removes 4 MiB of scratch at 1024×1024, or 256 KiB in the measured 256×256 workload.

Opacity raster images are retained at effect groups and their immediate child subtrees. Deeper ordinary wrappers retain invalidation metadata and paint directly into the nearest retained image. Keeping the immediate child caches lets a group's own paint change reuse static descendants. Every actual opacity/filter group still completes its contents before applying its own effects, preserving [CSS group opacity](https://www.w3.org/TR/2026/CRD-css-color-4-20261008/#transparency). Removing a group releases its image; restoring a former root creates a texture that shares its new raster instead of retaining a second CPU copy. Old textures keep their pixels until their owners release them.

The rounded-clip solid-fill path uses one white texture and per-instance tint. Tint channels preserve the former RGBA8 texture quantization, sprite alpha and rounded mask. Animated colors therefore stop adding entries and device resources to the global color texture cache. The public `CreateTextureByColor` cache still exists for other callers; its ownership policy has not been replaced with eviction.

Compact zero-initialized texture uploads borrow a byte buffer from `sync.Pool`, copy their updated prefix and clear the remaining suffix. Drivers consume creation pixels before returning, so the buffer is returned on success or error. GPU resources and retained texture shadows never enter the pool. Entries discard buffers above 16 MiB; this is a per-entry limit, not a global memory budget. GC eviction, larger payloads and concurrent borrowers can allocate.

## Measurements

Three samples per workload, `-cpu=1`, median values below. HTML and upload samples use 200 ms; changing-color samples use 1000 iterations. B/op measures cumulative Go allocations. The separate retained-image metric sums cached image slice capacities; it excludes metadata, pool entries, driver copies and GPU resources. RSS and native/GPU memory were not measured.

| Workload | Before B/op | After B/op | Before allocs/op | After allocs/op |
| --- | ---: | ---: | ---: | ---: |
| 128² box shadow, fresh scratch | 262144 | 132096 | 2 | 2 |
| 1024² box shadow, fresh scratch | 16777216 | 8396800 | 2 | 2 |
| 256² drop-shadow, fresh scratch | 688288 | 426144 | 4 | 3 |
| Opacity root + 16 ordinary wrappers, fresh cache | 1128552 | 145512 | 57 | 42 |
| Re-upload compact 1024² R8 texture, warm scratch | 1048576 | 0 | 1 | 0 |
| Re-upload compact 1024² RGBA8 texture, warm scratch | 4194304 | 0 | 1 | 0 |
| Rounded clipped rectangle, a new color each iteration | 365 | 0 | 2 | 0 |

The opacity workload retains **17 → 2 RGBA images**, or **1114112 → 131072 bytes** (88.2% less image storage). The shallow one-wrapper control retains two images both before and after, so this saving depends on tree structure. Nodes that genuinely require nested opacity or filters still need independent images. Invalidation metadata remains proportional to the number of nodes.

Warm shadows, drop-shadows, opacity cache hits and repaints remain 0 B/op in these benchmarks. The changing-color workload creates one GPU texture per iteration before this round and zero additional textures after warming the shared white texture. The upload benchmark includes the driver's preallocated copy; pooling removes repeated staging allocation, while retaining a reusable buffer between uploads.

Raw records include timings, but host load and CPU frequency were not isolated enough to infer an application frame-rate improvement. Ordinary wrapper changes can repaint a larger portion of their nearest cached subtree; the immediate child cache boundary preserves reuse when only the effect root changes. Flattening also removes intermediate per-wrapper RGBA8 rounding and coordinate snapping. Fractional positions and translucent overlaps are tested, but arbitrary scenes are not claimed to be byte-identical to the old per-node offscreens.

Raw results: [before](render-memory-round2-before.txt), [after](render-memory-round2-after.txt), [validation](render-memory-round2-validation.txt).

## Validation

Passed targeted package tests for `./render ./graphics ./scene ./ui/html ./ui ./module`, with `-p 1 -parallel=2`; HTML tests were rerun after the final root-lifetime fix. Targeted race tests cover independent pool borrowers, concurrent uploads/painting, error recovery, compact-shadow restoration, every resident device, color tint/masks and changing composite boundaries. `go vet` passed for the modified graphics, scene, HTML and render packages; `git diff --check` passed.

Pixel SHA-256 recordings were captured from the baseline and matched after the changes:

- Box shadows: `8a4ad50ee85b4f15863d6e453cd6bc1d27026b4dc1532f5ef8a18d1a66410c66` (thin images, subimage stride/offset, sigma 0–100).
- Nested opacity/filter composition: `c0e30d63956b38fa5585862e4de6d5ce22aea8c677b7c5b7736de7b404005e00` (descendant mutations).
- Filter drop-shadow: `5400cb3cf6c37e0aea8cf01705e1e97266a275a88000d6784c146491ef3af799` (positive/negative fractional offsets, subimages, zero and large kernels).

Existing Gaussian recordings, text/visibility/filter/group-opacity tests and shared-image resize tests also passed. Additional tests cover memory capacity, collapsing/restoring effect groups, former root texture ownership, unchanged child versions, allocation-free descendant repaint and wrapper-invariant fractional painting. Allocation assertions run separately from race checks, since the race runtime deliberately drops some pool entries.

Android x86_64 targeted tests passed on `10.0.0.174:16416`, using NDK 27.2.12479018, `GOOS=android GOARCH=amd64 CGO_ENABLED=1`, `x86_64-linux-android24-clang` and `-trimpath`. Device tests include the final root restoration fix, pixel recordings, text composites, buffer reuse and upload lifetimes. Font fixtures accompany the tests. These checks use test drivers and do not exercise a native Android surface or Chromium pixel comparison. No editor was launched; visual acceptance remains manual.

```sh
go test -p 1 ./ui/html -run '^$' \
  -bench '^(BenchmarkHTMLShadowWorkingSet|BenchmarkHTMLOpacityWorkingSet|BenchmarkHTMLDropShadowWorkingSet)$' \
  -benchmem -benchtime=200ms -count=3 -cpu=1
go test -p 1 ./graphics -run '^$' -bench '^BenchmarkCompactTextureUpload$' \
  -benchmem -benchtime=200ms -count=3 -cpu=1
go test -p 1 ./scene -run '^$' -bench '^BenchmarkRoundedClipChangingColor$' \
  -benchmem -benchtime=1000x -count=3 -cpu=1
```
