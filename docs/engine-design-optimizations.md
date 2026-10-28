# Engine Design Optimizations

## Scope

The seven changes apply to the engine's scheduling, ECS, HTML renderer and
render device ownership. Existing simulation remains continuous by default.

| Area | Implementation |
| --- | --- |
| Transform synchronization | World and UI transforms are synchronized in the render stage before camera extraction. Physics2D and Physics3D share one fixed-post-update propagation system. |
| HTML textures | Each surface owns its generated gradients and opacity textures. Removal, reconciliation and shutdown release unused textures. Opacity buffers and same-size textures are reused. |
| Change tracking | Added/Changed queries use conservative summaries for blocks of 64 rows. Physics accesses mark changes only when values actually differ. Component and world versions expose invalidation without scanning rows. |
| Incremental transforms and layout | Static inputs return immediately. World transforms update dirty descendants; UI matrices reuse parent revisions. Layout containers compare cached inputs before computing constraints. |
| 2D ordering | Extraction sorts render items once. Camera filtering and drawing preserve the extracted order. |
| Idle scheduling | Opt-in on-demand apps suspend schedules between events or deadlines. Desktop and DRM loops wait for native events; browser loops stop requesting animation frames while idle. |
| Device ownership | Windows own their Device and Canvas. Textures with CPU payloads, meshes, shaders and targets retain separate device residencies. Disposal uses the owning drivers, and backend statistics remain per device. |

## Idle API

```go
app.SetRenderMode(onix.RenderOnDemand)
app.SetIdleEnabled(true)

// Request a frame from a worker without writing to ECS from that worker.
app.RequestRender()

// Schedule an update at the earliest requested deadline.
app.WakeAt(time.Now().Add(250 * time.Millisecond))
```

The first frame, pending input, explicit render requests, tracked ECS mutations
and due deadlines run the schedules. RequestRender, RenderRequest.Request and
WakeAt can be called concurrently. ECS mutations, window changes and rendering
remain on the application loop thread. Pointer data changed outside tracked
mutable access requires an explicit render request.

Installed capabilities no longer keep the loop running by themselves. Scoped,
active animations, Spine playback, live particles, fading/emitting trails and
awake non-static physics bodies keep updating. Sleeping 2D contact islands permit idle. Finished effects, paused ancestors and
suspended scenes permit idle. Task counts reuse semantic component revisions;
unmanaged physics bodies are also considered. WebViews keep their callbacks
serviced while a browser component exists. Nested layout settling and active
HTML transform transitions also keep updating. Module Update callbacks require
ticks; FrameUpdate callbacks run once per requested frame and permit idle.
Custom systems that depend on time must schedule their next WakeAt or leave
idle disabled. The frame clock includes elapsed wall time when waking.

Android/iOS hosts still own their callback cadence. App.Step skips idle engine
work, but this change does not suspend the native mobile host's frame callbacks.

## Resource preparation

Texture and shader construction now prepares CPU data without calling the active
render driver. Scene preloaders share background CPU preparation and an explicit
device upload queue with time, byte and resource budgets. CPU readiness and GPU
readiness are separate. See [Task scheduling and resource preparation](task-resource-preparation.md)
for the APIs, ownership rules, budget limits and focused validation.

## Rendering Contract

Use window-owned Device/Canvas for application rendering, or create an explicit
Device for standalone rendering. ActivateRender selects the window's immediate
context; activation does not destroy another device. Install/Uninstall own only
the standalone default device. The obsolete global Generation API is removed.

Resource RenderHandleFor(device), target ColorTextureFor(device)/BeginFor(device)
and Canvas submissions use explicit device residency. GPU-only and external
textures cannot be reconstructed on another device without CPU pixels; their
foreign-device handles are invalid. External refresh always uses its importer.
Rendering still shares one thread; this is not concurrent GPU command submission.

Changed filters retain their original tick and row semantics, including wrap,
archetype migration and swap removal. Conservative summaries can scan additional
rows; they must never drop matching rows. Topology changes rebuild transform
indices. Relevant UI input changes still scan UI entities to compare inputs,
while unchanged matrix calculations are skipped.

HTML opacity flattening keeps the existing supported paint subset. This work
does not extend flattening to text, replaced elements, controls, shadows or
overflow. Opacity bounds and raster invalidation are collected in the same tree
walk; child compositing follows after child buffers are available.

## Measurements

Linux/amd64, Go 1.26.0, Intel i5-11500, 100 ms benchmark runs. These are small
CPU benchmarks, not whole-frame FPS or GPU measurements. Earlier baseline runs
and two updated samples are shown; short runs have normal timing variation.

| Benchmark | Earlier baseline | Updated samples | Go allocations |
| --- | ---: | ---: | ---: |
| With filter, 10,000 rows | 45.6 us | 47.6-47.9 us | 0 |
| Changed filter, all 10,000 rows match | 64.6 us | 69.2-70.6 us | 0 |
| Changed filter, one matching row in 10,000 | Not measured | 0.99-1.38 us | 0 |
| Stable layout hierarchy, 1,000 nodes | 32.1 us | 0.052-0.057 us | 0 |
| Layout hierarchy rebuild, 1,000 nodes | 38.1 us | 34.6-34.8 us | 0 |

Block summaries benefit sparse changes but add overhead when every row matches.
The stable hierarchy result measures cache validation, not the complete layout
system. Regression tests also assert zero warmed allocations for static world/UI
transforms, opacity cache hits and same-size repaints, idle Step, device switches,
target residency and native wake signals. Creation, first residency, buffer growth
and texture/target size changes may allocate. Driver-internal allocations are
outside these Go assertions.

## Verification

Focused package tests:

```sh
go test -p 2 . ./ecs ./ui ./graphics ./render ./physics ./window -count=1
go test -race -p 2 . ./graphics ./render ./window \
  -run 'Test(Idle|NativeWake|ResourcesKeep|CanvasUses|GPUOnlyAndExternal|TargetsKeep|StatisticsRemain|DefaultDevice)' -count=1
go test -p 2 -tags onix_drm ./window \
  -run 'Test(IdleDeadline|NativeWake|FramePacer)' -count=1
go test ./ecs -run '^$' \
  -bench 'Benchmark(FilteredQueryTenThousand|UILayoutHierarchy|ChangedQuerySparse)$' \
  -benchmem -benchtime=100ms -count=2
```

Windows/amd64, browser WebAssembly and Android/arm64 builds were checked with
their platform toolchains. Android device connection to 10.0.0.174:16384 returned
Connection refused, so device runtime validation remains outstanding. Apple
platform compilation and native UI behavior need verification on Apple hardware.
No editor or screenshot tests were started.


## Physics, HTML state and explicit frame context

2D physics now retains static/dynamic indices and unchanged candidate pairs,
with optional contact-island sleeping. HTML separates stage state and reports
full-update fallback reasons. ECS rendering and scene preparation select their
device through the world frame, including editor previews. See
[Physics, HTML and rendering context](physics-html-render-context.md) for APIs,
benchmarks, ownership rules and validation limits.
