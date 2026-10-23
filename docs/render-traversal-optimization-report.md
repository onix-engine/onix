# Renderer repeated-traversal optimizations

Recorded 2026-09-14. Six changes combine compatible passes. Main-view 3D resource preparation immediately queues opaque/transparent items; failures clear all partial main-view queues. Skin/Morph/joint/weight validation occurs during packing into reusable buffers using float32 exponent tests, and uploads happen only after full success. Draw statistics accumulate during existing backend draw loops and commit after success.

Sorted 2D items stream batch boundaries through generic pointers without copying full items or storing a runtime batches slice. Spine still submits complete contiguous ranges. HTML child recursion immediately combines overflow ranges, preserving postorder dependencies. A single DFS collects CSS and strips metadata in place, including CSS from removed head subtrees in source order.

Sorting, bounds-center-before-radius, GLES whole-batch prevalidation and deformation draw-index checks remain required. Unified multi-camera sorting was not implemented. `QueueMeshes3DSystem`/queue_meshes_3d are removed; preparation now classifies, and custom pipelines must depend sorting directly on prepare_render_resources_3d without compatibility wrappers.

Linux amd64/i5-11500 five-run medians are below; both versions remain warm 0 B/op/0 allocs/op. The deformation/HTML baseline comes from `91271ed`; old 2D batch construction remains test-only. Cases isolate CPU sorting/consumption, validation/packing and overflow computation, excluding real GPU work. Preparation queueing, statistics and CSS collection have no separate timing claim.

Tests cover classification/picking/shadows/error retry, order/material/Spine boundaries, packed layouts/finite values/errors, statistics and CSS/parent/range correctness. Targeted native/DRM/Node checks passed; Node skips browser GPU tests. Historical external application tests had display-service and layout failures reproduced with baseline overlays. Those application records are outside this engine repository. Metal received source review without Apple hardware; browser visuals/editor screenshots were not run.

## Examples and commands

```sh
go test ./render -run '^$' -bench BenchmarkPackMeshDeformations -benchmem -count=5
go test ./ecs -run '^$' -bench 'BenchmarkRenderPhase2DTraversal|BenchmarkHTMLOverflowExtents' -benchmem -count=5
```

## Recorded measurements

| CPU microbenchmark | Before | After | Time reduction |
| --- | ---: | ---: | ---: |
| Deformation validation/packing: 10,000 vertices, 4 morphs, 64 joints | 1,290.019 μs/op | 506.445 μs/op | 60.7% |
| 2D sort/consume: 10,000 items, shared material | 1,052.510 μs/op | 824.686 μs/op | 21.6% |
| 2D sort/consume: 10,000 items, alternating materials | 1,440.557 μs/op | 842.703 μs/op | 41.5% |
| HTML overflow: 10,101 boxes | 315.028 μs/op | 289.278 μs/op | 8.2% |

Performance figures describe the recorded workloads and environments, not complete application frame rates. Warm allocation guarantees exclude first use and capacity growth.
