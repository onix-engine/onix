# CPU scratch retention and texture packing

Recorded 2026-10-25, baseline `5e85e3109bc8c73da732e8fcc0b4c62cf6247e7c`, Go 1.27.3/Linux amd64. This change covers Go/JavaScript CPU scratch; it does not change GPU buffer or texture allocation.

Native Sprite/Mesh/picking draw records, deformation words and texture-update descriptors now shrink after 30 consecutive low-demand frame observations. WebGPU also trims deformation/sprite offset arrays. Demand includes every batch between frame starts, including batches flushed or overwritten before the final batch. Large intermittent batches restart observation. Retained capacity is twice recent demand with small per-array floors; oversized idle scratch releases its backing array. Steady loads allocate nothing in this bookkeeping. Shrinking and subsequent regrowth can allocate. Without frame starts, reclamation does not advance.

WebGPU/WebGL upload Uint8Arrays use the same demand policy, with a 64 KiB floor and power-of-two capacity. The original array is reused for stable loads. Copy offsets, payload sizes and ownership are unchanged; the JS runtime decides when unreachable ArrayBuffers are collected.

Metal/GLES accumulate the full pixel length while validating and packing update descriptors, then allocate pixel storage once and copy the batch. Growth of an existing array retains 50% headroom to avoid repeated allocations for slightly larger batches. Single-update calls still consume caller pixels directly. Multi-update calls still use one concatenated CPU copy; this change removes intermediate growth copies, not that final copy. No extra validation/size prescan was added.

## Measurements

| Workload | Before | After |
| --- | ---: | ---: |
| 4096 Sprite + 4096 Mesh records, then 30 one-draw frames | 638952 B retained | 8448 B retained (-98.68%) |
| JS upload array: 1 MiB peak, then 30 four-byte upload frames | 1048576 B retained | 65536 B retained |
| JS upload array: oversized peak followed by 30 idle observations | peak retained | array reference released |
| Cold packing, 4 × 64 KiB updates | 712704 B/op, 4 allocs/op | 262144 B/op, 1 alloc/op |
| Cold packing, 64 × 64 KiB updates | 20176896 B/op, 16 allocs/op | 4194304 B/op, 1 alloc/op (-79.21% bytes) |
| Warm packing, either batch size | 0 B/op, 0 allocs/op | 0 B/op, 0 allocs/op |
| Warm native draw packing, 1 or 1024 draws | 0 B/op, 0 allocs/op | 0 B/op, 0 allocs/op |

Retained figures are array capacities measured in tests, not process RSS. B/op is cumulative Go allocation per operation, not peak live memory. Pixel benchmarks isolate packing and exclude input creation, validation/native descriptor building, cgo and GPU upload. The baseline uses the previous append-per-update loop in an isolated HEAD checkout with the same benchmark harness. Each binary ran three 200 ms samples with one Go CPU. Timing was not isolated from other machine activity; no frame-rate or timing improvement is claimed. Raw output: [before](render-cpu-scratch-before.txt), [after](render-cpu-scratch-after.txt).

## Validation

Tests cover large-then-small batches within a frame, low-load shrink, idle release, regrowth, exact pixel payloads, independent ownership, reuse after smaller uploads, gradual size growth, and warm allocations. Native draw tests exercise real packing and picking paths without requiring GPU resources.

Passed:

- `go test ./render ./graphics ./scene/render3d -count=1 -parallel=2`
- `go test -tags onix_drm ./render -count=1 -parallel=2`
- Targeted `go test -race ./render` for retention, native draw packing and texture packing.
- Xvfb Vulkan window test `TestNativeVulkanFrameDoesNotAllocate`: real frame submission remains allocation-free after warm-up.
- Node/Wasm render tests for WebGPU and WebGL (`-tags wechat`), plus final targeted upload/packing checks.
- Android x86_64 render package tests on `10.0.0.174:16384` using NDK 27/API 28. These do not exercise a native Android rendering surface.

Apple Metal hardware and browser GPU visuals were not exercised. No editor was launched. Resource-slot compaction and adaptive GLES snapshot caching remain separate work.


## Follow-up: WebGPU cache lifetime and resource-pool storage

WebGPU material combinations and deformation-size bindings now share a generic frame-age cache. Entries unused for more than 120 successful frame starts expire from an age-ordered list. Stable hits update list position at most once per frame and do not scan resources. Each cache removes up to the larger of 64 entries or the preceding frame's insertion count, so continuous churn cannot outrun a fixed eviction budget. Active working sets are retained without a hard entry limit.

Per-texture and per-mesh links let resource destruction remove only associated bindings. Expiry unlinks those owners and clears cached JS references; it never destroys the buffers/textures referenced by submitted GPU commands. Existing buffer-generation checks and exact binding sizes remain intact. After the expired backlog drains, 30 low-load observations can rebuild oversized Go maps with a smaller reservation. Map rebuilds preserve entry addresses and cached groups; disposal also drops map storage. The reservation counter is an entry-count hint, not measured Go bucket bytes. Work stops when frames stop, and the JS runtime controls actual object collection.

GenerationPool now embeds its free list in each slot. A live slot links to its own one-based ID; a free slot links to the next free ID. The new link occupies the former occupancy flag's padded space, removing the separate growing free-index slice. Removal clears resource references and increments generations as before; slot reuse, stale-handle checks and LIFO reuse order remain unchanged. Slot-array capacity itself still follows the resource high-water mark. The first release of a prepopulated pool no longer allocates.

| Workload | Before | After |
| --- | ---: | ---: |
| 1024 material combinations and 1024 deformation-size combinations, then one hot entry in each; resources remain alive | 2048 cached entries retained | 2 cached entries retained after expiry/drain |
| Stable WebGPU binding reuse across three frame slots | cached reuse | 0 B/op, 0 allocs/op, 0 new JS bind groups/op |
| Cold resource lifecycle, 1024 uint32-valued slots | 51824 B/op, 23 allocs/op | 38904 B/op, 12 allocs/op |
| Cold resource lifecycle, 65536 uint32-valued slots | 5521272 B/op, 50 allocs/op | 4347896 B/op, 26 allocs/op |
| Warm native mesh/target registry cycles | 0 B/op, 0 allocs/op | 0 B/op, 0 allocs/op |

Pool numbers include creating an empty pool, inserting all resources, and removing them; they are cumulative Go allocation, not retained bytes or native GPU resource creation. The 65536-slot case allocates 1173376 fewer bytes per cold lifecycle. Raw output: [pool before](render-cpu-pool-before.txt), [pool after](render-cpu-pool-after.txt). Same baseline SHA/environment and three 200 ms samples as above; timings are not controlled enough to claim a speedup.

Follow-up tests cover continuous churn faster than 64 new entries/frame, bounded burst draining, age ordering, map shrinking, stable entry addresses, duplicate texture ownership, mesh/texture/driver destruction, eviction/recreation, dynamic binding offsets, and frame-buffer generation changes. Pool tests cover first-release allocations, nonsequential removal, duplicate removal, future/stale handles, cleared references, live iteration, LIFO reuse and generation wraparound.

The native render/graphics/render3d packages, GLES configuration, WebGPU/WebGL Node/Wasm tests, targeted race checks, Android x86_64 package tests, and the allocation-free Vulkan window frame test passed after the follow-up. Final targeted cache tests additionally verified adaptive churn eviction and zero-allocation warm cache reuse. Browser GPU visuals and Apple hardware remain untested.
