# Rendering hot paths and allocation reuse

Zero allocation applies to engine-controlled warm paths with unchanged resources and
loads within retained capacity: no Go/C heap allocations or new upload/readback/
deformation descriptor resources. Initialization, resource creation, swapchain rebuilds,
target resizing and growth still allocate. Vendor driver internals are outside this guarantee.

## Vulkan uploads and readback

Single updates pass original pixels. Batch uploads validate/reserve native mapped
staging first, then Go fills it and submits: one copy from source pixels, without a
Go concatenation buffer. RGBA8/R8 offsets align to four bytes; byte counters count
actual pixels. Native queues retain handles/offsets, never Go pointers. Between reserve
and fill/submit, do not call another driver method. Out-of-frame threshold flushing
occurs during submission.

Each in-flight frame retains staging/instance capacity and waits for its fence before
reuse; BeginFrame does not overwrite prequeued data. Exceeding 4 MiB no longer destroys
staging every frame: peak capacity stays until device disposal. External textures use
a fixed pending-index queue, removing destroyed entries to prevent reused-slot mistakes.
Deformation descriptor sets persist per frame and reuse after fences; the existing
256-mesh-submission pool limit returns an explicit error when exceeded.

Synchronous readback reuses mapped buffer, command buffer and fence, waiting on the
submission fence rather than QueueWaitIdle. It still returns this operation's pixels
synchronously, preserving picking timing.

## GLES, cgo and events

Sprite attributes/draw and twelve mesh instance attributes each use consolidated C
calls with static/stack data. Buffer-orphaning internals are not covered by engine C
allocation counts. Metal/GLES single updates avoid concatenation; batches reuse a
contiguous buffer and one cgo call, with byte counting merged into metadata traversal.
Audited synchronous Vulkan/Metal/GLES calls use noescape/nocallback only when data is
consumed/copied before return and native resources own queued work. macOS AppKit can
invoke WebKit callbacks and retains callback support; close-state output lives in Context.

Linux/macOS dispatch pointers into reused event arrays instead of copying a 4096-byte
text payload. Retained text is copied to Go strings; repeated text commands preserve
the command before callbacks. Linux batching has no Go callbacks. On Linux amd64,
Go 1.26.0, i5-11500, three dispatch-only mouse-event microbenchmarks measured:

| Implementation | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| Before, by value | 544.9–560.0 | 4864 | 1 |
| After, reused pointer | 2.117–2.314 | 0 | 0 |

These are dispatch costs without business callbacks, not FPS. The first phase passed
Linux render/window tests, DRM GLES/Android arm64 builds and targeted cgocheck2 checks;
Android connection failed and Apple execution was unverified.

## Ordered four-texture sprites

Builtin sprites use four fixed slots across Vulkan/Metal/GLES/WebGL2/WebGPU. Preserve
submission order; allocate a slot on first texture use, reuse existing slots, flush
on the fifth distinct texture. Atlas children share parent slots. Sizes/mips/RGBA8/R8
may differ. Explicit gradients computed before branching preserve mip selection.
Custom shaders, blend/clip/projection/view changes, shape/mesh draws and explicit
flushes break batches. Custom materials retain single-texture draws and all Custom fields.

TextureSlot adds one final float: Instance2D grows 128 to 132 bytes (+3.125%). Fixed
arrays avoid temporary maps/combination objects. Vulkan reuses texture descriptors;
WebGPU reuses per-texture/per-frame bind groups; other backends bind four textures.
R8 means white RGB/coverage alpha; GL converts or swizzles as appropriate. GLES/WebGL2
use unpack alignment one for arbitrary widths.

Under unchanged state, 1000 alternating A/B/C/D sprites require one draw; A/B/C/D/E
requires 250. A single atlas already needs one. Draw reductions are not measured FPS.
Warm Canvas batching has zero Go allocations; JavaScript/browser API objects are excluded.

## Tilemaps

Inverse-transform the viewport to map space for conservative candidate rows/columns,
then apply precise culling. Translation, rotation, scale, mirror/shear work; singular
or non-affine transforms traverse all cells conservatively. Add one boundary cell for
roundoff and edge contact. Snapshots own their storage:

- Revision==0 permits direct edits and copies each extraction into reused storage.
- Nonzero Revision promises a version bump for every Tiles edit. Matching source,
  length and revision reuse without a full scan/copy. Replacing any of those refreshes.
- Texture/size/opacity metadata updates every extraction independently.
- Removing maps clears snapshots and source references.

Static maps may use Revision:1; increment after edits. Both modes can allocate nothing
at stable capacity, but stale explicit revisions hide edits.

## Materials and screen effects

Material extraction, extension textures and instance packing reuse caches. Texture
flags are computed during preparation, without rescanning during packing. Sheen tables
are shader constants; disabled layers skip sampling/basis work. Vertices include 16-byte
tangents; instances remain 192 bytes, mesh uniforms 3584. Warm material packing and CPU
tangent morph/skin tests assert zero Go allocation; native Vulkan checks C/resource counts.
IOR uses existing Material.y, EmissiveStrength multiplies packed RGB, adding no fields.
GLES/Vulkan pixels cover direct/IBL/SSR, high IOR, partial metals and HDR capture.

SSR retains per-camera color/depth targets and reuses prepared opaque snapshots without
re-extraction/upload. Depth and RGBE capture precede reflection composition; custom opaque
and transparent objects follow. With SSAO, double color width for AO capture without
extra samplers. Estimated SSR targets cost 16 bytes/viewport pixel, or 24 with AO,
plus the original AO depth target. Resize lazily rebuilds; disable/removal releases;
picking retains caches. Trace at most 32/64/128 steps, six binary refinements, one ray
per enabled base/coat/sheen layer. Skip zero weights/roughness cutoffs; only hits fetch
four decoded RGBE texels. No frame-by-frame environment convolution occurs.

Light candidates retain peak capacity. Each camera uses fixed 16-element index/score
arrays and one scan instead of sorting or allocating per-camera slices. Shadow slots
follow selection; presets limit complete shadow groups/resolution/cascades/AO/SSR.
Disabled effects release targets. See [lighting](render-lighting.md) for budgets.

FXAA retains viewport RGBA8+depth and fixed one-draw/instance arrays: one color write,
one fullscreen sample pass, UI afterward. Cold enable/resize still allocate. Warm
FXAA+AO+SSR offscreen/window frames measure zero Go allocation with a noncopying mock
driver. CPU allocation results do not measure GPU bandwidth or frame time.

Fog/ACES add arithmetic inside builtin fragments without new passes/targets/samplers.
Reserved lightShadows[0..7].w stores camera values; all 16 lights/tiles remain and
uniforms stay 3584 bytes. Disabled fog skips distance/exponential work. Extraction,
packing and complete warm fog/ACES+FXAA+AO+SSR frames measure zero Go allocation.
Logical and isolated GLES/Vulkan pixel regressions cover validation, multiple cameras,
serialization, preview settings, alpha, orthographic fog, extreme HDR, sky and capture
isolation. Shader generation validates WGSL, compiles GLSL/SPIR-V and generates MSL.

## Seven optimizations recorded 2026-10-13

1. Static deformation streams upload once per GPU mesh via SetMeshDeformation.
   Per-frame MeshDeformationData carries only mesh handle, joints and morph weights.
   Failed creation/static upload releases the new mesh and permits retry.
2. WebGPU separates material group 0 from animation/static group 1. Dynamic offsets
   avoid invalidating material bindings. Cache by mesh/range/frame generation; growth
   rebuilds affected bindings. Encoded-but-unsubmitted buffers retire with the frame.
3. Vulkan retires only queued texture/mesh/shader/target slots through embedded index
   chains after fences, without scanning all resources or allocating list nodes.
   Windows external D3D11 texture synchronization also uses fixed indices.
4. WebGL2 reuses an 80-byte command/TypedArray and one JS helper per mesh draw for
   changed textures, twelve attributes and draw. Reset texture caches per batch;
   initialize program sampler units once. Vertex source is shared with GLES.
5. WebGPU sprite uniforms deduplicate four-bit RGBA8/R8 coverage masks into at most
   sixteen 256-byte-aligned slots per batch: one uniform upload plus instance upload.
6. Metal retains a geometrically grown Shared readback buffer until driver destruction.
   Readback stays synchronous; command buffers/blit encoders still allocate API objects.
7. Vulkan/Metal picking skips Surface texture validation/packing unused by picking shaders.

MeshGPUDeformationDriver requires SetMeshDeformation and DrawMeshesDeformed. Direct
callers CreateMesh, install static streams once, then submit matching Mesh handles.
Reject duplicate static upload, wrong vertex counts, stale handles and insufficient joints.
graphics.Mesh automates uploads. Set Vertices/Indices/Skin/MorphTargets before first
successful upload; changes require a new mesh. Later source edits do not change resident
GPU streams. CPU originals stay for other devices and CPU deformation/picking. This
trades resident GPU memory for fewer per-frame transfers. Vulkan static data uses
set 3/binding 14 within four descriptor sets; Metal vertex buffer 5; WebGPU group 1/
binding 1. Vertex/instance/eight-word header layouts remain unchanged.

Linux amd64, Go 1.26.0, i5-11500, 10,000 vertices, four morph targets and 64 joints,
three 2-second runs, static upload outside timing:

| Packing | ns/op | Per-frame deformation bytes | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| Before, includes static streams | 539,898–554,954 | 1,764,144 | 0 | 0 |
| After, animation only | 754.5–769.5 | 4,144 | 0 | 0 |

Transfer payload falls 99.76%; a separate 1,760,004-byte static stream uploads once.
This measures CPU packing, excluding initialization/GPU/full-frame time.

## Resource management complexity

Typed native_slots.h pools embed free-list nodes: slot allocation/recycling O(1),
N creations amortized O(N) instead of O(N^2). Reserve storage before resource creation;
failures consume no index. Vulkan frees retired slots only after fences. Growth allocates;
warm reuse does not. Tests cover failure, the 4096-slot limit and 2048 warm reuse cycles.

WebGPU bindings embed fixed texture reverse-index nodes, including duplicate slots.
Deleting T textures/G bindings costs O(T+G), removing all affected reverse links. Mesh
binding caches are per mesh/range, avoiding device-wide scans. First-time creation allocates.
Vulkan textures link upload indices; cancellation marks relevant entries and submission
skips them in order. Epochs invalidate submitted heads without table scans; queue growth
preserves indices. Cancellation becomes O(T+U), with O(1) external pending removal.
Staging byte totals update incrementally, including alignment gaps. Tests cover growth,
cancel order, burst/in-frame submission, epochs and forbidden pre-fence reuse.

MipChain computes dimensions/encoded sizes/offsets sequentially, reducing WebGL2/WebGPU
metadata work O(L^2) to O(L) with zero allocations. DeviceValues.Set combines device
and reusable-slot lookup. A 16384x8192 BC7, 15-level, 300ms Linux microbenchmark measured
1331 ns/op random level lookup versus 128.0 sequential, both 0 B/op, 0 allocs/op. This
is metadata work, not whole upload throughput/FPS.

## Recorded verification limits

Linux package, native Vulkan pixels/steady C-resource allocation, GLES, DRM, release,
cgocheck2, Windows cgo compile and wasm checks passed at the documented stages. Node
and Chromium software WebGPU/WebGL2 tests covered batching, shaders, deformation,
buffer retirement and invalidation. Browser entry: render/tests/sprite_browser.html,
TestBrowser.*Pixels; the required WebGPU flags are retained below. Ordinary headless
ANGLE failed at render-pass creation in that environment.

Android arm64/x86_64 cross-compilation passed. The actual device reports x86_64;
23 render tests and subsequent CPU logic/packing suites passed there. Earlier arm64
execution raised SIGILL. Some later effect phases encountered Connection refused,
so those mobile pixels/GPU costs remain unverified. Cross-compilation is separate
from execution. No Apple SDK/device was available: Metal source generation is not
Apple compilation or runtime validation. Browser API allocations and vendor internals
are excluded from engine-issued allocation counters. No editor screenshots were run.

## Commands

```sh
go test ./graphics ./render ./ecs
go test -tags onix_drm ./render ./graphics ./ecs
go test ./render -run TestVulkanNativeSteadyStateAllocations -count=1 -v
go test ./window -run '^TestNative(MouseEventDoesNotAllocate|EventTextSurvivesBufferReuse|RepeatedCommandSurvivesBufferReuse)$' -count=1
xvfb-run -a env LP_NUM_THREADS=2 WAYLAND_DISPLAY= go test ./window -run '^TestNativeVulkanFrameDoesNotAllocate$' -count=1
go test ./window -run '^$' -bench '^BenchmarkNativeMouseEvent$' -benchmem
go test ./ecs -run '^$' -bench BenchmarkTilemap -benchmem
go test ./graphics -run TestSprite -bench BenchmarkCanvasSpriteMultiTexture1000 -benchmem
```

```sh
GOOS=js GOARCH=wasm go test -c ./render -o /tmp/onix-render-webgpu.test.wasm
GOOS=js GOARCH=wasm go test -tags wechat -c ./render -o /tmp/onix-render-webgl.test.wasm
```

```sh
go test ./render ./graphics ./scene/render3d ./scene
go test ./render -run '^TestVulkanNativeSteadyStateAllocations$' -count=1 -v
go test ./render -run '^$' -bench '^BenchmarkPackMeshDeformations$' -benchmem -count=3 -benchtime=2s
GOOS=js GOARCH=wasm go test ./render -exec "$(go env GOROOT)/lib/wasm/go_js_wasm_exec" -run '^(TestWebGPU.*|TestCopy.*|TestMeshDeformation.*)$' -count=1
GOOS=js GOARCH=wasm go test -tags wechat ./render -exec "$(go env GOROOT)/lib/wasm/go_js_wasm_exec" -run '^(TestWebGL.*|TestCopy.*)$' -count=1
```

```sh
go test ./render -count=1
go test ./render -run '^$' -bench '^BenchmarkTextureMipChain$' -benchmem -benchtime=300ms
GOOS=js GOARCH=wasm go test ./render -exec "$(go env GOROOT)/lib/wasm/go_js_wasm_exec" -run 'TestWebGPU|TestWebGL|TestTextureMipChain' -count=1
```
