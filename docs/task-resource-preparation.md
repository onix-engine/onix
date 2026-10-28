# Task scheduling and resource preparation

## Idle scheduling

On-demand applications with idle enabled inspect actual scoped work, rather than
the set of installed features. Playing animations, incomplete Spine playback,
emitting/live particles, fading or continuously emitting trails, and non-static
physics bodies require updates. Activity inherited from ancestors and module
execution scopes exclude paused and suspended content. Non-static physics bodies
are conservatively active unless 2D sleeping is enabled. Pending physics inputs
keep the loop awake until a fixed step consumes them, even when the last body
becomes static, is removed or leaves the active scope. Initial static bodies
also receive a synchronization tick before idle.

Task counts cache semantic component revisions, hierarchy inputs, entity lifetime
and query-scope membership. Unrelated archetype migrations preserve the cache.
Stable checks and idle Step calls allocate nothing after warmup. Manually created
physics bodies are included even without ECS components.

Module Update callbacks, pending events, requests and switches require ticks.
FrameUpdate callbacks permit idle and run once per requested frame. Layout,
HTML transitions, input, explicit render requests and wake deadlines retain their
existing scheduling rules. WebViews keep servicing native callbacks while a
scoped browser component exists; their backends do not yet provide dedicated
event wakeups. Custom systems still schedule WakeAt or disable idle. Simulation
component changes must use tracked mutable ECS access. Resource pointer changes
outside that access require an explicit render request.

## CPU preparation and GPU upload

Texture, decoded image, compressed texture and shader constructors do not invoke
the active render driver. Texture Valid/ValidFor checks never upload; target
views only refresh CPU bounds and check the target's lifetime.
RenderHandleFor(device) performs residency acquisition on the render thread;
upload errors are reported there rather than from CPU-only constructors. External
texture imports remain native operations on that thread. Render-target texture
views are CPU-only; their attachments are realized by explicit handle acquisition
on the selected device.
CPU texture patches before the first upload update only the shadow payload.

resource.Preparation[T] owns one background CPU result and exposes preparation
states, nonblocking Take, Finish, Cancel and an explicit synchronous WaitCPU.
Load functions must not access ECS or render devices. Wake callbacks must be
safe to call from the worker. Cancellation does not wait for blocking I/O; the
worker discards unpublished results when it returns. After Take, cleanup belongs
to the receiving thread. Compressed container decoding owns its payload until
upload, so the source blob may be released immediately after decoding.

graphics.UploadQueue deduplicates textures (including atlas views), meshes and
shaders. AddMaterial3D collects material textures and shaders. Advance accepts an
explicit device and UploadBudget:

```go
ready, err := queue.Advance(device, graphics.UploadBudget{
    Time:      2 * time.Millisecond,
    Bytes:     4 << 20,
    Resources: 8,
})
```

Budgets are checked between resources. A driver call cannot be interrupted, and
the first oversized resource is allowed to avoid starvation. Nonpositive Time
performs one unit; nonpositive Bytes/Resources add no limit. Byte counts estimate
input payloads, including mesh deformation streams, rather than total GPU memory
or generated mip levels. A failed upload retains its position and CPU payload
for retry. A different device restarts the queue using that device's residencies.
Missing/destroyed devices cannot complete nonempty queues. GPU-only textures
release their CPU payload after successful upload and cannot subsequently move
to another device. Reset releases resource references and retains queue storage.

## Scene loading

Both project.BeginPreloadScene and module.SceneManager.BeginPreload return before
resource I/O and decoding finish. Call Advance or AdvanceFor on the owning loop
thread to commit CPU results and advance GPU uploads. CPUReady reports completed
CPU construction; GPUReadyFor reports residency on a specific live device.
Ready/ReadyFor permits headless completion after CPU preparation. With a live
device, activation requires completion on the current device.

```go
loader, err := runtime.Manager().BeginPreload(module.ScenePreloadRequest{
    Scene: "game.json",
})
// Handle err, then call from the application's frame callback:
ready, err := loader.AdvanceFor(device, graphics.UploadBudget{
    Time: 2 * time.Millisecond, Bytes: 4 << 20, Resources: 8,
})
if err == nil && ready {
    err = runtime.Manager().Activate(loader.Scene(), module.SceneActivationOptions{})
}
```

CPU completion requests a frame; committed work that still needs another Advance
requests another frame. Waiting on the worker does not continuously request
frames. NewApp installs the render request before its first frame, so loaders
can capture it during startup. Standalone project worlds install a RenderRequest;
standalone module runtimes supply Options.Wake for this behavior.
Discard cancels pending work and releases hidden scene ownership. Failed CPU
preparation or GPU upload preserves the foreground. Discard after activation
does not destroy the committed module scene. Only one module preload may be
pending; cancel it before requesting another managed transition.

The project loader commits nodes, resolves references, prepares text glyphs and
collects uploads incrementally. The module loader decodes new private resource
stores on the worker; existing foreground stores remain on the loop thread.
Module installation, startup events, custom Content hooks, layout and glyph
preparation remain one main-thread commit unit. This unit can exceed the time
budget; the loader then defers uploads to the next call. Enter/Resume hooks run
during activation and any resources they create are prepared synchronously before
the visibility commit. Applications should prepare expensive assets before these
hooks. Synchronous Preload convenience APIs use the same upload queue.

Resource discovery includes authored sprites/layers, animation texture keys,
particle appearances, tilemaps, Spine textures, prepared glyph atlases, meshes,
environment textures and registered generic materials/submesh slots. Register
custom material types through the normal renderer APIs before preparation.
Resources created later, changed text windows or changed raster scales may need
additional uploads during rendering.

## Validation

Focused tests cover blocked worker decoding, wakeup, transfer/cancellation/error
ownership, hidden-scene cleanup, unchanged foreground, activation readiness,
device changes, payload ownership, material discovery, upload budgets and retry.
Warm task-count checks, idle Step, repeated resource discovery and ready-queue
checks assert zero Go allocations.

```sh
go test -p 2 . ./ecs ./resource ./graphics ./graphics/codec/ontex \
  ./project ./module ./scene ./scene/render3d -count=1
go test -race -p 2 . ./resource ./graphics ./graphics/codec/ontex \
  ./project ./module ./scene ./physics \
  -run 'Test(Preparation|UploadQueue|CPUPreparation|DecodeOwns|AsyncScenePreload|ScenePreloader|QueueEntityUploads|AnimationTask|EffectTask|TaskCount|Idle|AppModuleFrame)' -count=1
```

Expanded regression checks still encounter the previously reproduced physics
precision and UI input/caret failures. The independent native Vulkan C allocation
test also terminated with a segmentation fault on this host. Those failures are
outside these task/resource changes. No editor or screenshot validation was run.
ADB connected successfully to 10.0.0.174:16384; Android runtime validation was
not performed.
