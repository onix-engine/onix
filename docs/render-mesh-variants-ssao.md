# Mesh shader variants and SSAO

Recorded 2026-10-19. Four fixed fragment variants derive from mesh.frag.wgsl through shadergen for Vulkan, Metal, WebGPU and GLES/WebGL2: full PBR, basic PBR, Unlit and depth. Basic retains normal/MR/AO/emissive/IOR/specular, lights/shadows/IBL/fog/tone mapping while removing clearcoat, sheen, anisotropy and SSR resolve. Advanced/mixed instances and SSR resolve select full PBR. Depth passes retain mask, mirrored/double-sided behavior and RGB24 encoding with three varying inputs; opaque depth avoids base-color sampling.

Each draw scans only its own instance range once, returning early when full PBR is required. Fixed variant selection allocates nothing and does not generate feature-combination pipelines. Backends lazily cache actual program/attachment/blend variants. Metal libraries compile individually; pure 2D no longer compiles full PBR. Custom vertex-only shaders pair with full built-in fragments. Static SPIR-V sizes below exclude vertex stages and are not GPU-memory or FPS measurements; using all variants can cost more pipeline memory than one.

SSAO retains one center world→NDC solve and derives disk radii from inverse-projection homogeneous quotients rather than three solves. A fixed 32-entry spiral table removes per-sample sin/cos/sqrt. Existing 8/16/32 taps, world radius/bias, texel-center reconstruction, exclusion/transparent semantics and depth resolution remain, with no added frame allocation/upload.

Generate with python3 render/internal/shadergen/generate.py using naga and glslangValidator/glslc; spirv-val validated all four outputs. Historical Metal tests ran on real hardware with MTL_DEBUG_LAYER=1, including 18 PBR comparisons, depth/Unlit/selection/lazy compilation and SSAO pixels 170 unoccluded/142 occluded. Android x86_64 emulation reported Adreno 640/GLES3.1 and passed render/render3d and 108 focused variant pixel cases, plus production GLES pbuffer probes. It does not establish ARM phone performance. WebGPU/WebGL2 received Wasm compile checks only in this round.

The complete Android shadow suite was intermittently failing: old shaders failed once/passed twice; new shaders passed once/failed twice. Root cause is unresolved and baseline reproduction does not exclude this change's influence. Focused variant/SSAO tests passed; the full suite is not stably green. No Linux tests, editor or screenshot tests were run in this recorded round.

## Examples and commands

```sh
"$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/linux-x86_64/bin/x86_64-linux-android35-clang" \
  -std=c11 -O2 render/tests/gles_mesh_variants_test.c \
  -o /tmp/onix-shader-gles-variants-android -lEGL -lGLESv3 -lm
adb shell mkdir -p /data/local/tmp/onix-shader-opt
adb push /tmp/onix-shader-gles-variants-android \
  render/shaders/gles_mesh.frag render/shaders/gles_mesh_basic.frag \
  render/shaders/gles_mesh_unlit.frag render/shaders/gles_mesh_depth.frag \
  /data/local/tmp/onix-shader-opt/
adb shell 'cd /data/local/tmp/onix-shader-opt && chmod +x onix-shader-gles-variants-android && ./onix-shader-gles-variants-android gles_mesh.frag gles_mesh_basic.frag gles_mesh_unlit.frag gles_mesh_depth.frag'
```

```sh
cd render
xcrun clang -fno-objc-arc -O2 tests/metal_mesh_variants_test.m \
  -framework Foundation -framework Metal -framework QuartzCore -framework CoreVideo \
  -o /tmp/onix-metal-variants
MTL_DEBUG_LAYER=1 /tmp/onix-metal-variants
```

## Recorded measurements

| Fragment program | Input locations | SPIR-V bytes |
| --- | ---: | ---: |
| Full PBR | 11 | 202,420 |
| Basic PBR | 9 | 94,112 |
| Unlit | 8 | 44,984 |
| Depth | 3 | 5,164 |

Performance figures describe the recorded workloads and environments, not complete application frame rates. Warm allocation guarantees exclude first use and capacity growth.
