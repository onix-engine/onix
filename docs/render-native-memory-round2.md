# Native renderer memory: second round

Recorded 2026-10-18. Shared Vulkan/Metal/GLES upload-deduplication tables shrink after thirty frame-slot reuses with demand at most a quarter capacity, retaining twice the window peak rounded to a power of two, minimum 16 entries. Intermittent large requests reset observation. Vulkan material entries/hash buckets use the same policy with a 64-entry floor after frame fence completion and release excess descriptor pools while preserving retained handles. Failed CPU resizing preserves all original storage/pools.

Texture descriptor pools change from 4096 startup sets to lazy 64-set pools. Texture ownership tracks pools, free slots are reused, empty pools retire after thirty frame starts, and the first pool remains warm. Referenced pools remain protected by fences. Creation/first-allocation failure does not commit ownership. A fixed 64-record pool table needs no management heap allocation. Shared texture/render-target view creation preserves sampler and R8 swizzle behavior.

GLES distinguishes CPU snapshot demand from GPU upload demand, freeing unused deduplication snapshots during existing buffer cleanup while retaining live GPU buffers. Snapshots up to 1 KiB stay warm; the next cached upload recreates actual capacity. Vulkan Scratch statistics now include material entries and buckets.

The table is requested capacity/counts, not RSS or descriptor VRAM. Slot observations differ from displayed frames and stop when submission stops. Tests cover shrink failures, collisions, regrowth, handles, pool limits/failure cleanup, active tails, snapshots, exact GPU contents and stable allocation/resource counts. Linux uses software drivers and Metal mocks; Android x86_64 render tests and arm64/Windows C checks passed without Android surface or Apple GPU validation. Shrinking and rewarming may allocate.

## Examples and commands

```sh
go test ./render ./graphics ./scene/render3d -count=1 -parallel=2
go test -tags onix_drm_vulkan,onix_drm ./render -count=1 -parallel=2
cc -std=c11 -Wall -Wextra -Werror -fsyntax-only -Irender render/vulkan_native.c
```

## Recorded measurements

| Workload | Retained before | Retained after |
| --- | ---: | ---: |
| Upload metadata: 4,096-entry peak then 8/frame | 196608 B (192 KiB) | 768 B (16 entries) |
| Vulkan material metadata: 4,096 combinations then 8/frame | 360448 B (352 KiB) | 5632 B (64 entries) |
| Material descriptor pools per slot | 64 | 1 |
| First Vulkan texture descriptor reservation | 4096 sets | 64 sets; no pool without textures |
| GLES 128 KiB cached upload then uncached uploads | 128 KiB CPU snapshot | 0 CPU snapshot; original 128 KiB GPU buffer reused |

Performance figures describe the recorded workloads and environments, not complete application frame rates. Warm allocation guarantees exclude first use and capacity growth.
