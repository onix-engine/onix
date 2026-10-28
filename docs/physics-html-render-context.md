# Physics, incremental HTML state, and frame ownership

## 2D collision searches and sleeping

`physics.World` retains separate static and dynamic spatial hash grids. A proxy
snapshot includes shape, pose, body/collider generation and collision policy.
Each contact rebuild checks these inputs, recomputes bounds only for changed
geometry, and removes/reinserts grid entries only when their coverage changes.
A body type change transfers its proxy between grids. Dead proxies release
entries to a free list. Cell-size changes reindex live proxies.

Candidate pairs with unchanged endpoints survive subsequent rebuilds. Only
changed proxies query the indices, followed by deterministic sort/deduplication.
Static sensors retain static/static overlap events. Large shapes and extreme
coordinates retain the conservative fallback. Synchronizing proxy snapshots is
still O(n); unchanged objects avoid bounds calculation and spatial searches.
The solver continues rebuilding contacts after position corrections, allowing
new wall/stack contacts to enter the current step.

```go
simulation := physics.NewWorld()
simulation.Sleep = physics.SleepConfig{
    Enabled: true,
    LinearThreshold: .05,   // world units / second
    AngularThreshold: .05, // radians / second
    TimeThreshold: .5,     // seconds
}
```

Sleeping defaults to disabled. Positive finite thresholds are optional; omitted
or invalid thresholds use the defaults above. Dynamic bodies joined by solid
contacts sleep together after the whole island becomes quiet. Sensors and shared
static supports do not merge islands. Sleeping skips integration and constraint
solving, while collision Begin/Stay/End handling remains available when Step runs.
Forces, impulses, pose/settings changes, changed/removed supports, new solid
contacts with changed supports, awake impacts and moving kinematics wake affected
islands. `Body.Wake` wakes explicitly; `Body.IsSleeping` reports actual state.
The final solid-contact graph is retained between steps. Wake requests traverse
only the affected island and share visited flags until the next step, avoiding
whole-world scans for each setter. Generation checks prevent reused body slots
from inheriting old contacts; queue storage is reused without warm allocations.

`TaskCount2D` allows sleeping islands to stop the on-demand loop. It observes
unconsumed ECS changes and simulation input snapshots, so queued forces and
collider/support changes require a physics tick before idle. Use tracked ECS
writes, or request a frame after changing state outside ECS, as for other idle
inputs. Both `TaskCount2D` and `TaskCount3D` retain pending ECS synchronization
until a fixed step consumes it, including initial static bodies, switching the
last body to static, removal, deactivation and execution-scope changes. Sleeping
applies to the 2D solver; non-static 3D bodies remain continuous.

`World.Statistics()` exposes bounds updates, index updates, pair queries, contact
builds and sleeping body counts without allocating. Targeted validation includes
brute-force pair comparisons after mutations, extreme coordinates, dense stack
correction, eight-body island sleep/wake, independent islands on a shared floor,
new/removed supports, ECS forces after idle and warm allocation checks.

Measured on an i5-11500, linux/amd64, short benchmark runs:

| Workload | Before | After | Warm allocations |
| --- | ---: | ---: | ---: |
| 256 unchanged separated dynamic bodies, Step | 276 µs | 11.0 µs | 0 |
| 1024 unchanged separated dynamic bodies, Step | 2525 µs | 60.4 µs | 0 |
| unchanged 256-sensor broadphase | 237 µs | 6.8 µs | 0 |
| one moving proxy among 4096 static bodies, broadphase | — | 70 µs | 0 |
| 64 sleeping bodies in an 8×8 stack, Step | — | 139 µs | 0 |
| 4096 bodies without colliders, batch SetVelocity with sleeping enabled | 41.7 ms | 63 µs | 0 |

The first three rows measure reuse of unchanged candidates. They do not estimate
moving dense-stack speedups or GPU work. The sleeping-stack benchmark also
reports the number asleep (64); results vary with host load.

## HTML stage state

`htmlRenderState` embeds six internal owners without extra heap objects:

| Owner | Inputs and retained outputs |
| --- | --- |
| Document | DOM, source, ID index, document/event revisions, mutation roots |
| Style | stylesheet/cascade caches, computed styles, animations, style changes |
| Layout | layout cache/tree, dirty layout/order nodes, topology and used size |
| Paint | retained boxes/entities, reconciliation queues, textures, extents/culling |
| Input | pseudo-state, control snapshots, focus, event and scroll/clip routes |
| Update | render key, visibility, pending work, sync counters and diagnostics |

`beginSync` captures the full-update cause. Cached/event fast paths require no
pending layout or paint work. A successful `commitSync` consumes document
mutations, style changes and layout invalidations through their respective
owners. Paint completion consumes deferred input paint; style/event updates
preserve it. Errors retain pending work for retry. After adding owner geometry
components, the renderer reacquires the render-state reference because ECS
archetype migration can invalidate the previous pointer.

```go
diagnostic := html.Diagnostics(world, surfaceEntity)
// Path: cached, events, style, partial, or full.
// Fallback: initial-document, document-replaced, render-environment,
// image-provider, layout-requested, visual-requested, retained-paint-invalid,
// visibility-changed, control-or-event-contract, structure-ownership,
// box-ownership, context-ownership, or stacking-ownership.
```

Diagnostics describe the last sync and allocate nothing. Local reconciliation
records its preflight rejection reason. Existing style-only, partial layout,
structural reconciliation, culling and event paths remain in use. This is an
internal ownership change; CSS and DOM rendering semantics are unchanged.

## Explicit rendering destination

```go
canvas := graphics.NewCanvas2D(device, width, height)
frame := graphics.BindFrame(world, canvas)
frame.Uploads.AddTexture(texture)
ready, err := frame.AdvanceUploads(graphics.UploadBudget{Resources: 2})
```

Each ECS world/window has a `*graphics.FrameContext` containing its canvas and
upload queue. Its device is derived from the canvas, so a separate device field
cannot disagree with the drawing destination. `FrameFor`, `CanvasFor` and
`DeviceFor` read it without a global fallback. An unbound world is headless.
BindFrame reuses the existing resource; rebinding preserves queued CPU resources
and lets the queue check residency on the new device.

App window creation and frame entry bind this resource before startup systems.
2D/3D extraction, submission, debug statistics, shadow/postprocessing resources,
HTML density/viewport and scene preload defaults use the owning frame. Editor
preview worlds receive the parent window's canvas explicitly. Extracted cameras
capture output dimensions so later window/context changes do not change their
placement matrix.

`ScenePreloader.Ready/Advance` use the owning world's frame device; `ReadyFor` and
`AdvanceFor` select an explicit device. `Bundle.PreloadScene` prepares CPU data;
`Bundle.PreloadSceneFor(device, id)` also uploads. Texture region updates patch
existing device residencies and CPU shadows without implicitly uploading to the
current global device. Explicit resource preparation acquires new residency.
Render-target texture views are also constructed without driver calls;
`RenderHandleFor(device)` realizes the attachment on the selected device, so
editor targets cannot accidentally upload to another window's global device.

Public immediate-mode `ActiveDevice`, `CurrentContext`, and no-device resource
handle helpers remain available. The ECS rendering and preparation paths use
explicit destinations. Integrations running schedules directly must BindFrame
before rendering. Binding/uploading and ECS access remain render-thread work.
Tests alternate two worlds/devices while deliberately selecting the other global
context, and verify upload/submission ownership and camera snapshot isolation.

## Validation limits

No editor was launched and visual acceptance remains manual. Android ADB connected
to 10.0.0.174:16384; these CPU changes were not validated through device rendering.
The existing `TestDynamicPoseRetainsPrecisionBetweenECSWrites` still fails and is
excluded from the otherwise passing physics package run. Five editor UI assertions
also reproduce on the original HEAD; two independent-project packaging tests are
blocked by missing dependencies in their isolated cache with GOPROXY=off.


Focused commands used (never `go test ./...`):

```sh
go test -p 2 . ./graphics ./scene ./scene/render3d ./module ./ui/html -count=1
go test ./physics -skip '^TestDynamicPoseRetainsPrecisionBetweenECSWrites$' -count=1
go test -p 2 ./project ./ui/html ./ui -skip 'TestBindControlExpandsGenericTextControlMode|TestHTMLInputEditsAndEmitsEvents|TestHTMLTextVisualRolesSurviveResize' -count=1
go test -race -p 2 ./physics ./graphics ./scene ./scene/render3d ./ui/html ./module -run 'Sleep|Broadphase|WorldFrames|FrameContext|SyncState|FallbackDiagnostics|AsyncScenePreload|TextureUpdate|TestPrepareRenderResourcesQueuesMainViewsAndDiscardsFailure' -count=1
go test ./physics -run '^$' -bench 'BenchmarkWorld2D(Step|Broadphase|StaticIndex|SleepingStack)$' -benchmem -benchtime=200ms -count=1
```

A subsequent race run also covered GPU-only texture patches before first upload
and preservation of deferred input paint during HTML style synchronization.
The editor package passes with the five reproduced UI failures and the two
isolated-cache packaging failures excluded. No new native Vulkan rendering test
was run; the host's previously reproduced native allocation-test crash remains
outside this change's validation.
