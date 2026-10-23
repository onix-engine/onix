# Shared renderer logic

Production code decreases by 132 lines relative to the optimization-start snapshot, excluding tests/docs and earlier memory work. Metal/Vulkan share C draw/resource definitions and parameter packing while retaining separate normal/picking entries. Batches are borrowed by pointer; statistics and handle resolution share the existing draw traversal/buffer. Cold non-inline error formatting avoids success-path stack/instruction expansion.

Metal lighting writes directly to the final 3584-byte uniform. Vulkan/WebGPU/GLES share scene-light filling. WebGL normal/picking mesh paths share validation/upload; WebGL/WebGPU share generic slice views and TypedArray scratch. Generic resource destruction preserves handles when native destruction fails.

Go 1.26/Linux amd64/i5-11500/Node22 medians use four CPU-0 alternating runs. Old/new native packing share one test binary; WebGL functions share a Wasm module/upload helper to isolate extraction overhead. All listed cases are 0 B/op/0 allocs/op before and after. Full-snapshot Wasm checks also retain no new JS upload buffers/views for warm dynamic meshes/shapes. Static resource creation still allocates twice on WebGL and ten times on WebGPU; process timing variation cannot imply GPU frame rate.

Checks cover uniform offsets/native sizes, buffer reuse, stale handles, failure ownership, picking without material lookup, Vulkan steady-state and GLES/Metal mock upload pools. Native/DRM/CGO-disabled and Node Wasm tests passed. Browser pixel tests skip in Node; Apple GPU and editor screenshots were not verified.

## Recorded measurements

| Workload | Before | After | Go allocations |
| --- | ---: | ---: | --- |
| Native mesh packing, 1 draw | 38.89 ns/op | 37.04 ns/op | Both 0 B/op / 0 allocs/op |
| Native mesh packing, 1,024 draws | 42.26 µs/op | 40.18 µs/op | Both 0 B/op / 0 allocs/op |
| Native lighting uniform, 16 lights | 869.3 ns/op | 439.1 ns/op | Both 0 B/op / 0 allocs/op |
| WebGL dynamic mesh, 64 vertices | 40.76 µs/op | 39.28 µs/op | Both 0 B/op / 0 allocs/op |
| WebGL dynamic mesh, 10,000 vertices | 73.79 µs/op | 69.72 µs/op | Both 0 B/op / 0 allocs/op |

Performance figures describe the recorded workloads and environments, not complete application frame rates. Warm allocation guarantees exclude first use and capacity growth.
