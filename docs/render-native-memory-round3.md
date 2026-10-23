# Native renderer memory: third round

Recorded 2026-10-18. `native_buffer.h` centralizes array shrinking: after thirty low-use observations, retain twice the window peak rounded to a power of two and per-array floors. Normal frames update counters only. Failed shrinking preserves pointers/capacity for a later retry.

Vulkan upload queues record peak demand between frame starts, including completed/cancelled entries, so an empty frame-start queue cannot cause repeated shrink/regrow. Shrinking preserves pending order, cancelled holes and per-texture indexes without rescanning textures; minimum 32 entries. After GPU retirement, Vulkan instance/staging/retired-attachment tables, Metal upload blocks and GLES upload tables release excess CPU capacity with minimum counts 4/2/4, 4 and 8 respectively. Existing frame completion protects retained GPU handles and mappings.

Vulkan Mesh deformation/lighting descriptors use 16-set pools instead of reserving 256 initially, with up to sixteen pools preserving the original 256-set cap. After fences, low demand retires excess tail pools while keeping the first. Failed first allocation destroys the new pool without advancing ownership/cursors. NativeScratchBytes now counts upload queues and management tables, so values can increase through more complete accounting; driver descriptor storage is excluded.

Table tests simulate peak CPU table capacities with real retained small buffers rather than unnecessarily creating 1,024 GPU buffers; GLES uses actual 256 uploads. Values are metadata capacity/counts, not process RSS/VRAM. GPU retirement may require one window and CPU table shrinking another. Slot observations are not displayed frames; no automatic recovery happens without frames.

Tests cover cancelled/pending queues, epochs, failed resize/retry, intermittent peaks, data/handle retention, descriptor limits/regrowth and real two-slot pixel readback above the 16-set boundary. Existing pixel/upload/readback/lifecycle/allocation regressions passed. Vulkan AddressSanitizer ran with driver leak detection disabled, so it is not a process-wide leak proof. Linux software Vulkan/GLES, Metal mocks, Android x86_64 package tests and Android arm64/Windows syntax checks passed; Apple GPU and Android native surface remain unverified.

## Examples and commands

```sh
go test ./render ./graphics ./scene/render3d -count=1 -parallel=2
go test -tags onix_drm_vulkan,onix_drm ./render -count=1 -parallel=2
cc -std=c11 -Wall -Wextra -Werror -fsyntax-only -Irender render/vulkan_native.c
```

## Recorded measurements

| Workload | Retained before | Retained after |
| --- | ---: | ---: |
| Vulkan upload queue after 4,096-entry peak | 294912 B (288 KiB) | 2304 B (32 entries) |
| Vulkan instance/staging/retirement tables: 1,024 entries each | 114688 B (112 KiB) | 368 B total (4/2/4 entries) |
| Metal upload table: 1,024 entries | 24576 B (24 KiB) | 96 B (4 entries) |
| GLES 256 small uploads then one/frame | 8192 B (256 entries) | 256 B (8 entries) |
| Initial Vulkan Mesh descriptor reservation | 256 sets | 16 sets |
| Vulkan Mesh peak 256 sets then one/frame | 256 sets | 16 sets; 15 pools released |

Performance figures describe the recorded workloads and environments, not complete application frame rates. Warm allocation guarantees exclude first use and capacity growth.
