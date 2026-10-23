# Native render attachments and buffer retention

Recorded 2026-10-18. Vulkan honors `RenderTargetDescriptor.Depth`: depthless targets create no depth image/view/memory and use color-only framebuffers. Color-format-specific depthless passes and Sprite/LCD/Shape/corner-mask/Mesh pipelines are created only when drawn. Custom shaders own native modules and lazily cache depth/blend variants instead of creating six Mesh pipelines at registration. Modules do not borrow caller SPIR-V; pipelines/modules retire with frames or driver disposal. First use can compile and fail.

Vulkan/Metal readback buffers observe maximum per-frame requests. Above 64 KiB, thirty low-demand observations at no more than a quarter capacity retire storage for right-sized recreation; Vulkan protects incomplete readbacks. Small/intermittently busy buffers stay warm. No submitted frames means no timed reclamation.

Vulkan first instance blocks shrink from 1 MiB to 64 KiB, staging from 4 MiB to 256 KiB; Metal starts at 64 KiB instead of 1 MiB. Later blocks grow toward normal bulk granularity; large requests round rather than double, e.g. 9 MiB staging reserves 12 MiB. Existing fences/caches/shrink behavior remains. Resident Vulkan mesh arenas still start at 1 MiB. Disposal also fixes an omitted LCD pipeline release.

The table reports requested buffer capacities and theoretical depth bytes, not process RSS or measured VRAM. Driver alignment can differ; BufferMemoryStatistics excludes textures, resident mesh and driver internals. Tests cover real depthless Vulkan pixels, custom pipelines/blends, load/clear, upload interruption, readback peak/shrink/protection, native lifecycle and stable zero allocations/resource creation. Linux uses software Vulkan and Objective-C Metal mocks. Android/Windows syntax and Android builds passed; x86_64 Android render tests passed without native GPU surface rendering. Apple GPU testing was not performed.

## Examples and commands

```sh
go test ./render ./graphics ./scene/render3d -count=1 -parallel=2
go test -tags onix_drm_vulkan,onix_drm ./render -count=1 -parallel=2
```

## Recorded measurements

| Workload | Before | After |
| --- | ---: | ---: |
| Vulkan: two slots with small instance/staging blocks | 10 MiB | 640 KiB |
| Metal: three slots with small upload blocks | 3 MiB | 192 KiB |
| Vulkan: low use after 32 MiB readback | Retains 32 MiB | 0 after shrink; next 4-byte readback retains 4 KiB |
| D32 bytes for depthless 1920×1080 offscreen target | Approximately 7.91 MiB | 0 |

Performance figures describe the recorded workloads and environments, not complete application frame rates. Warm allocation guarantees exclude first use and capacity growth.
