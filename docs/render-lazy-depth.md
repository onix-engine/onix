# Lazy window depth and 3D pipelines

Recorded 2026-10-19. Metal, Vulkan, GLES, WebGPU and WebGL2/WeChat allocate application-owned window depth and default Mesh/picking pipelines only on first use. Offscreen depth remains controlled by RenderTargetDescriptor.Depth and does not upgrade the window.

| Backend | Pure 2D | First window 3D | Later frames and resize |
| --- | --- | --- | --- |
| Metal | No depth; separate 2D attachment variants | Allocate depth; restart encoder with color Load/depth Clear | Reuse pipelines/depth, resize after 3D use |
| Vulkan | Color-only swapchain framebuffers | Upgrade acquired image only; preserve color and initialize depth layout | Upgrade images on demand; old swapchain depth retires on resize |
| GLES | EGL requests depth 0/no MSAA; default framebuffer | Own RGBA8+Depth24 FBO, copy prior color, clear reversed depth | Reuse FBO and blit presentation; rebuild on resize |
| WebGPU | No depthStencilAttachment | Allocate depth24plus, restart pass preserving color/MSAA | Cache attachment variants; recreate resized depth next frame |
| WebGL2/WeChat | context depth=false; default framebuffer | Own FBO, including MSAA color/depth when requested | Resolve and present fullscreen triangle; rebuild on resize |

GL retains additional color storage after first 3D use, so this optimizes windows that never use 3D. WebGL cannot blit a single-sample FBO directly into an MSAA default framebuffer; texture presentation preserves orientation, alpha, clipping, state and the user's antialiasing setting.

WebGPU uses synchronous createRenderPipeline inside active frames to avoid yielding until the canvas texture expires before submission. Outside frames it retains async compilation/error reporting. First variants compile; cached variants and 2D depth/no-depth combinations reuse resources.

Historical checks passed render/DRM package tests and Vulkan software pixels in Xvfb/lavapipe, covering 2D/resize depth absence, offscreen isolation, 2D→3D preservation, reversed depth, picking/readback/materials and warm allocation/resource counts. Khronos validation layers were unavailable. GLES production frame wrappers passed Mesa pbuffer pixels for depth 0, color preservation, reversed-depth clear, target/readback restoration and presentation beyond scissor.

Android NDK 27.2 built production render code. A device pbuffer probe replaced only native-window surface acquisition, testing resource/draw behavior; it did not validate complete Android windows/rotation. Browser Chrome 153 single/MSAA tests passed on Dawn/lavapipe and ANGLE SwiftShader, not all hardware/browser combinations. Node WebGPU/WeChat tests and Windows/Android cross-compilation passed; Windows GUI and editor/screenshots were not run.

The browser harness is render/tests/sprite_browser.html beside the Wasm binary, Go wasm_exec.js and production editor/templates/web/runtime/webgpu.js. Select WebGL via a wechat build and ?wasm=render-webgl.test.wasm. There are no full-application A/B memory figures for these backends. Depth estimates depend on pixels, format, samples and swapchain images; actual driver/process memory needs measurement.
