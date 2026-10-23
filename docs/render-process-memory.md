# Linux software Vulkan process RSS

Recorded 2026-10-18. Five alternating independent-process pairs measured Linux/glibc CPU Vulkan reclamation. Stage medians below are MiB. Low-load RSS falls 7.285 MiB (6.91%) and Private_Dirty 7.383 MiB; peak RSS does not decrease. Baseline low-load range is 104.973–105.602 MiB, optimized 97.770–98.426 MiB. The [CSV](render-process-memory.csv) retains all fifty records.

Both versions report 395920 B of rendering buffers; median live libc allocations are 32017024/32022208 B. The change returns free heap pages to the OS rather than reducing live allocations. libc free arenas can still include nonresident pages.

`vulkan_native.c` accumulates retired large buffers, mesh arenas, readback and texture blocks; 4 MiB schedules reclamation but does not measure exact reclaimable libc pages. Existing draw/upload loops count commands without another traversal. After fences, thirty low-demand slot reuses lower historical peaks to twice recent demand, minimum 1024 commands. Retiring a command peak also requests collection.

A request waits thirty quiet frame starts. More than 1024 commands, a buffer allocation of at least 1 MiB or further retirement delays it. One `malloc_trim(0)` consumes the request; idle frames do not repeatedly trim. Hardware Vulkan and non-glibc platforms do not enable this policy. It allocates no arrays or GPU resources. All five optimized processes trimmed once; median call time 2.582ms, range 1.429–3.412ms, synchronously on the render thread, potentially adding frame latency. No submission means no timer-driven reclamation.

Measurements use Mesa llvmpipe LLVM 20.1.2, LP_NUM_THREADS=2, disabled shader disk cache, Xvfb 1280×720 and a 960×540 window. A native C probe excludes Go, warms eight frames, issues 32768 draws with one sprite in each of two slots, then two 130-frame single-draw stages. It models command bursts, not distinct objects or texture-heavy editor workloads. After idle waits, the parent samples smaps_rollup five times at 10ms intervals. No forced GC or extra trim occurs.

Baseline SHA-256: da081d9521d8744da449e032076bb8cd1abd14b742e77e8c8f47486a0820831e. Optimized: a8f773ca0e1f03b40c8578a244dd0108a93c44296b29d7d89f65f138768f1a7a. The baseline includes the previous three rounds, not repository HEAD. The script needs cc/Xvfb/X11/software Vulkan; default ICD is /usr/share/vulkan/icd.d/lvp_icd.json. CSV memory-page columns use KiB, buffers/libc use bytes, trim_ns cumulative nanoseconds. No environment-sensitive RSS pass threshold is imposed.

Native policy, pixel, lifecycle and stable-allocation checks, DRM, strict C and cross-syntax tests passed; Android package tests do not exercise glibc trimming or native surface rendering. No hardware GPU/other OS/full-application RSS benefit was measured. `malloc_trim` affects free libc pages process-wide, not active objects or the Go heap.

## Examples and commands

```sh
python3 render/tests/measure_vulkan_rss.py \
  --baseline-render /path/to/before/render --repeats 5 > /tmp/comparison.csv
```

## Recorded measurements

| Stage | Before RSS | After RSS | Before Private_Dirty | After Private_Dirty |
| --- | ---: | ---: | ---: | ---: |
| After small-load warm-up | 100.973 | 101.043 | 33.910 | 33.934 |
| After command peak | 132.309 | 132.414 | 65.246 | 65.332 |
| After 130 low-load frames | 105.438 | 98.152 | 38.414 | 31.031 |
| After another 130 low-load frames | 105.438 | 98.152 | 38.414 | 31.031 |

Performance figures describe the recorded workloads and environments, not complete application frame rates. Warm allocation guarantees exclude first use and capacity growth.
