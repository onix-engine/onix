# ECS API implementation record

Recorded 2026-10-05. Eight completed phases implement the [design](ecs-api-design.md),
starting from d1bdabb6. All source callers migrated directly; old APIs have no forwarding
compatibility layer. Measurements and platform results below are historical records.

The 2026-10-29 [generic-method migrations](ecs-generic-methods.md) move component,
resource, command, context, cache and typed-constructor operations onto their owners.
Usage examples below use the
current API; historical names and benchmark labels retain their recorded meaning.

## Component access, queries and scripts

GetRef/GetMut and Slot variants return *T, nil if absent. Get by value retains (T,bool).
Resources keep presence semantics, including a present nil pointer. World.MustMut/EntitySlot.MustMut
report entity/type on missing required components. SceneInstance.Component supports named
low-frequency access; RequireComponent validates at installation without Changed and
returns a generation/World-checked Slot for repeated access. Rebind after scene replacement.

Query and Query2..Query7 return reusable views. Range over .All without parentheses;
Get copies, Ref is read-only by convention, Mut marks only the requested component,
and CommitComponent compares comparable values before marking. Go cannot enforce Ref
immutability. With/Without and SystemContext.Added/Changed work; views capture the change window without
nested-query interference. In borrows a slice, preserves supplied order, validates live
handles/components/filters, and treats nil as empty. Access values/pointers are borrowed
for the iteration; Commands defer structural edits. Completion/break/return/panic release
guards. The closure-returning iter.Seq2 prototype allocated four times per query, so
views replaced it. Archetype-bound columns and split value/change metadata reduce work
without unsafe. Generate query/spawner/patch adapters with go generate ./ecs using
internal/ecsgen/main.go; shared logic remains centralized.

Script Hooks[T] declares Install/Update/Input/Destroy/InputMask via concrete method
expressions. Generated files contain declarations rather than lifecycle bridges.
Registry order determines script-type updates; entity handles sort within each type.
Scene-owned binding lists isolate scenes sharing a World/type, rebuilt only on membership
changes. Context storage reuses by call depth and restores outer contexts on reentry.
Do not retain contexts or component pointers. Install runs after insertion; failure
removes inserted components/input routes. Update errors return immediately and release
guards. Historical 13-app entry points fell 1,843 to 1,015 lines, deleting 102 bridges;
this count excludes the new shared runtime and is not total framework code reduction.

UI SetText/SetPosition/SetSize compare only changed fields, retaining text layout caches,
styles and other transform data. Equal/missing returns false; SetControlBounds maintains
input geometry. Cache validated Slots and presentation state, force first/reinstall apply,
and skip formatting unchanged output without skipping time-dependent animation/audio.
SetIfChanged cannot distinguish missing from unchanged: keep explicit validation where
missing components are errors. html.ControlByID reuses NodeByID/EntityGroups, restricts
surface, uses first document ID and validates generation/control type. Dirty indexes
refresh; clean lookup allocates nothing and does not mark Changed. Before rendering,
after unsynchronized source replacement or for noncontrols it returns false. Live values
still go through SetHTMLControlValue to preserve dirty values/editing state. Parent-indexed
label association replaces O(T*K) scans with O(T+K), preserving last-match behavior where
required; semantic FindPart is appropriate only with verified unique parts.

Phase 1: Go 1.26.0, Linux amd64, i5-11500, 300ms x3 medians, 10,000 entities, identical
business work and a saved baseline binary. All rows are 0 B/op and 0 allocs/op.
Differences around 1–2% are near parity, not guaranteed acceleration or FPS gains.

| Workload | Before us/op | After us/op | Time change |
| --- | ---: | ---: | ---: |
| Read with With filter | 49.41 | 40.31 | -18.4% |
| Read with Changed filter | 72.65 | 71.52 | -1.6% |
| Write two columns | 101.72 | 99.82 | -1.9% |
| Read two values | 61.99 | 51.25 | -17.3% |
| Write one / read one | 85.85 | 75.12 | -12.5% |
| GetMut per entity | 249.20 | 247.50 | -0.7% |

## Phase 2: typed node creation and one migration

NodeSpawner binds the final seven-component UI archetype once and Reserve prepares
capacity. NodeOptions plus typed content replaces repeated Bundle construction/boxing/
sorting. Cached SpawnNode is a convenience path; repeated hot spawning should reuse
a spawner rather than create one per entity. Defaults include visible, TopLeft anchor
and valid transform; reject content duplicating fixed components.

PrepareRemoval deduplicates component types and caches source-to-target archetypes.
One old-column pass copies retained values, cleans/removes others and commits metadata
once, instead of migrating repeatedly. Preserve retained ticks, swapped rows, generation
checks and cleanup semantics. This is not rollback for external hook side effects.

| Workload | Before ns/op | After ns/op | Time change | B/op / allocs/op (before to after) |
| --- | ---: | ---: | ---: | --- |
| Seven-component Bundle to explicit NodeSpawner | 627.1 | 178.1 | -71.6% | 368 / 5 → 0 / 0 |
| Seven-component Bundle to cached SpawnNode | 627.1 | 205.4 | -67.2% | 368 / 5 → 0 / 0 |
| Unbind six present runtime components | 1900.0 | 378.9 | -80.1% | 0 / 0 → 0 / 0 |
| Remove three of four components | 344.2 | 121.5 | -64.7% | 0 / 0 → 0 / 0 |

## Phase 3: standalone ECS core

Core ecs owns entities/storage/query/commands/schedule/resources/messages. Domain types
move to scene and controls to ui. ChangesSince and Changed filters expose public version
boundaries; domain packages use generic resources/removal hooks rather than private
storage. Message readers borrow native buffers rather than copy transient data. The
following binary/package metrics include standard dependencies and unstripped output:

| Metric | Before phase 3 | After phase 3 |
| --- | ---: | ---: |
| go list -deps ./ecs packages, including standard library and itself | 149 | 63 |
| Minimal unstripped executable | 3,446,392 bytes | 2,502,265 bytes |

| Hot path | Before ns/op | After ns/op | B/op / allocs/op |
| --- | ---: | ---: | --- |
| Read two components, 10,000 entities | 52,299 | 52,170 | 0 / 0 |
| Prepared removal round trip | 117.2 | 115.2 | 0 / 0 |
| World transforms, one of 10,000 changed | 1,491 | 1,440 | 0 / 0 |
| UI transforms, one of 10,000 changed | 2,046 | 1,934 | 0 / 0 |
| Input dragging, 1,000 targets | 221,098 | 226,011 | 0 / 0 |

## Phase 4: independent HTML runtime

ui/html owns retained HTML, DOM/ECS reconciliation and interactions. Shared geometry
stays in scene. ui installs bridges explicitly without putting HTML behind ECS core.
Naming removes duplicated domain prefixes; previous names are removed directly.

| Phase 3 API | Phase 4 API |
| --- | --- |
| `scene.HTML` / `scene.NewHTML` | `html.Surface` / `html.New` |
| `scene.HTMLNodeByID` / `scene.HTMLControlByID` | `html.NodeByID` / `html.ControlByID` |
| `scene.DefineHTML` / `scene.DefineScopedHTML` | `html.Define` / `html.DefineScoped` |
| `scene.HTMLComponent[P,S]` / `scene.HTMLBehavior[P,S]` | `html.Component[P,S]` / `html.Behavior[P,S]` |
| `scene.HTMLRenderContext[P,S]` / `scene.HTMLView` | `html.RenderContext[P,S]` / `html.View` |
| `scene.SyncHTML` / `scene.HTMLAnimating` | `html.Sync` / `html.Animating` |

Shared UI matrix composition combines compatible work without repeatedly resolving
component geometry. HTML keyed order, animation and input keep borrowed state and reuse
scratch. The following includes small regressions rather than claiming uniform gains:

| Hot path | Before ns/op | After ns/op | After B/op / allocs/op |
| --- | ---: | ---: | --- |
| UI authored local matrix | 239.3 | 240.3 | 0 / 0 |
| UI exact composed local matrix | 231.9 | 99.76 | 0 / 0 |
| HTML keyed slot, reverse 32 nodes | 3,781 | 3,919 | 0 / 0 |
| HTML keyed children, reverse 32 nodes | 2,964 | 2,874 | 0 / 0 |
| HTML order, 10 unrelated controls | 55,366 | 54,260 | 0 / 0 |
| HTML order, 1,000 unrelated controls | 222,575 | 210,214 | 0–3 / 0 |
| CSS transform animation, 100 nodes | 88,433 | 87,491 | 0 / 0 |
| CSS color animation, 100 nodes | 281,242 | 274,576 | 0 / 0 |

## Phase 5: 3D and generic camera integration

scene/render3d owns extraction, material registration, deformations and drawing.
Public generic camera registration separates shared scene camera orchestration from
optional 3D resources. Install onix.Render3D or explicitly call render3d.Register;
3D-free applications do not need the package. No compatibility aliases remain.

| Phase 4 API | Phase 5 API |
| --- | --- |
| `scene.Mesh3D` | `render3d.Mesh` |
| `scene.MeshMaterial3D[T]` | `render3d.Material[T]` |
| `scene.MeshDeformation3D` | `render3d.Deformation` |
| `scene.Camera3D` | `render3d.Camera` |
| `scene.RenderWorld3D` / `NewRenderWorld3D` | `render3d.World` / `NewWorld` |
| `scene.RegisterCameraRenderer3D` | `render3d.Register` |
| `scene.RegisterMaterial3D[T]` | `render3d.World.RegisterMaterial[T]` |
| `scene.ExtractRender3DSystem` / `PrepareRenderResources3DSystem` | `render3d.ExtractSystem` / `PrepareSystem` |
| `scene.SortRenderPhases3DSystem` / `Render3DSystem` | `render3d.SortSystem` / `DrawSystem` |
| `scene.UpdateMeshDeformations3DSystem` | `render3d.UpdateDeformationsSystem` |
| `scene.Environment3D` / `ViewLighting3D` / `GroundPlane3D` | `render3d.Environment` / `ViewLighting` / `GroundPlane` |
| `scene.RenderQuality3D` / `QualityHigh3D` | `render3d.Quality` / `QualityHigh` |

| Hot path | Before ns/op | After ns/op | B/op / allocs/op |
| --- | ---: | ---: | --- |
| Prepare frame, two cameras / 100 meshes | 100,815 | 98,969 | 0 / 0 |
| Prepare frame, two cameras / 1,000 meshes | 1,005,695 | 1,046,380 | 0 / 0 |
| 1,000 deformations without joint/node transforms | 5,827 | 4,117 | 0 / 0 |
| 1,000 deformations with eight joints each | 775,549 | 782,142 | 0 / 0 |

## Phase 6: native message range and constant-time discard

MessageReader.All(world) borrows pointers. Advance the cursor before invoking the body:
break/return/panic consume only entered messages, leaving the rest. Mutations such as
Handled are visible to other readers. Sequences resolve World/cursor when executed,
not when cached; a later Discard still takes effect. Stale readers begin at the earliest
retained frame; previous-frame messages precede current-frame messages. Appends during
iteration, including growth, remain visible without loss/duplication. A pointer is valid
only for its current body, and advancing message frames during iteration is forbidden.

Discard moves only this reader to the current count in O(1), without clearing buffers
or affecting other readers; later writes remain readable. Neither reading nor skipping
changes World mutationVersion. Use Read for copies or ReadInto for reused storage.
Registration/growth can allocate; warm direct range, stack readers and dispatch do not.

Go 1.26.0, Linux amd64, GOMAXPROCS=6, 300ms x3 medians, identical binaries run sequentially;
setup outside timing and final cursors observed. Ordinary-function aggregation improved
roughly 54–57%, with consumers inlined; project dispatch differences around 2% do not
prove stable speedup. B.Loop integer cases regressed about 13%, InputEvent about 19%:
Go 1.26 keepalive/inlining behavior retains indirect yield/state checks there. A b.N
control did not show the same regression. Range does not guarantee faster consumers.
Discard stays about 15.5 ns across counts. Copied Read intentionally allocates.

| Workload | Before ns/op | After ns/op | B/op / allocs/op |
| --- | ---: | ---: | --- |
| Consume one integer in ordinary function | 22.02 | 16.11 | 0 / 0 |
| Consume 100 integers in ordinary function | 256.0 | 118.7 | 0 / 0 |
| Consume 10,000 integers in ordinary function | 23,353 | 10,587 | 0 / 0 |
| Consume 100 InputEvent in ordinary function | 255.0 | 110.9 | 0 / 0 |
| Discard one message | 19.35 | 15.50 | 0 / 0 |
| Discard 100 messages | 174.6 | 15.54 | 0 / 0 |
| Discard 10,000 messages | 15,880 | 15.44 | 0 / 0 |
| Consume 100 integers in B.Loop | 245.5 | 277.3 | 0 / 0 |
| Consume 10,000 integers in B.Loop | 23,149 | 26,214 | 0 / 0 |
| Consume 100 InputEvent in B.Loop | 245.9 | 293.6 | 0 / 0 |
| Read copies 100 InputEvent | 3,437 | 3,422 | 16,384 / 1 |
| Project dispatch, 10,000 filtered pointer subscribers | 100.5 | 98.38 | 0 / 0 |
| Project dispatch, 10,000 keyboard subscribers | 45,208 | 44,330 | 0 / 0 |

## Phase 7: mixed generic patches and system fonts

Patch1..Patch8 combine inserts/replacements/removals into one source-to-target migration.
Plans bind a World; generic values are not boxed. Reserve prepares populated or
preallocated source archetypes and migration target capacities; new structures,
unused source shapes, growth and first command streams can
allocate. Insert2..Insert8 handles multiwrites without removals.

Retained components preserve values/Added/Changed. Replacements preserve Added and
advance Changed; new values share one write tick. Writes win over removal of the same
type, replacement skips removal hooks, missing removals are ignored and duplicate writes
panic at construction. All actual removal hooks run before writes/migration with the
complete old row readable. Hook panic leaves storage unchanged but does not undo external
side effects. Structural changes update entity/swapped-row/StructureVersion once;
replacement-only patches do not migrate. Stale entities error. Apply during active
queries/cleanup panics; Queue defers and rejects Commands from other Worlds. Queue
copies payload values under normal Go semantics, so pointer/slice backing may be shared.
Execution/discard/checkpoint rollback clear released references. Existing command slots
and typed streams are reused; appendWithTicks writes values/ticks in one operation.

Android font discovery parses font_fallback.xml/fonts.xml for normal 400 sans-serif,
then available weights, respecting configured collection indices and system/product/
vendor/data paths, not hardcoded filenames. XML axis elements are skipped. TTF/OTF/
TTC/OTC work. Apple uses CoreText UI-font URL/PostScript name to choose the proper
collection face. text.DefaultFont shares one parsed font/glyph cache for UI/HTML/editor,
lives for the process lifetime and must not be closed. Unavailable fonts return nil;
explicit fonts remain supported. Cold parsing allocates, cache hits do not. Temporary
CFF probes close their backing. This adds no per-glyph fallback, variable-axis instancing
or complex shaping. See [system fonts](system-fonts.md).

Mixed-patch benchmark: Go 1.26.0, Linux amd64, GOMAXPROCS=6, 300ms x3 medians. Each
round trip removes two/adds two in each direction while retaining four columns;
identical values/final structure, all capacities warm. Sequential calls make eight
operations; patches two migrations. Control paths retain their actual measurements:

| Mixed structural round trip | ns/op | B/op / allocs/op |
| --- | ---: | --- |
| Sequential Insert / Remove | 1,197 | 0 / 0 |
| Two Patch.Apply | 268.1 | 0 / 0 |
| Two Patch.Queue + Flush | 442.5 | 0 / 0 |

| Control path | Before ns/op | After ns/op | B/op / allocs/op |
| --- | ---: | ---: | --- |
| Reused CommandInsert round trip | 123.1 | 122.8 | 0 / 0 |
| Prepared CommandSpawn round trip | 99.29 | 100.5 | 0 / 0 |
| PreparedRemoval round trip | 111.6 | 117.7 | 0 / 0 |
| Sequential Remove round trip | 339.0 | 325.7 | 0 / 0 |
| New World spawns 1,000 Spawner2 entities | 68,103 | 70,498 | about 180,954 / 140 |

## Phase 8: UI layout and final boundaries

Layout/Widget constraints, caches/default JSON and tests move to ui. Seventeen duplicate
constant names are removed; keep LayoutNone/LayoutResizeChildren/WidgetAlignAlways.
SiblingOrder is shared hierarchy order, while UITransform/Size2D/anchors/clips/text metrics
stay in scene for drawing, input, animation and layout. Anchor.TopLeft/Sprite2D.Size
share existing geometry, including one-axis explicit sizes and trimmed texture dimensions.

| Previous API | Final API |
| --- | --- |
| `scene.Layout` / `NewLayout` | `ui.Layout` / `NewLayout` |
| `scene.Widget` / `NewWidget` | `ui.Widget` / `NewWidget` |
| `scene.SyncUILayoutSystem` | `ui.SyncLayoutSystem` |
| `scene.UpdateLayout` | `ui.UpdateLayout` |
| `scene.UILayoutPending` | `ui.LayoutPending` |
| `scene.UILayoutOrder` | `scene.SiblingOrder` |

Widget stretching computes final width/height before updating display/input bounds once,
preserving Receiver/PassThrough and avoiding double Changed writes. Size updates follow
Size2D then Sprite2D then ClipRect2D. Queries use already-bound Ref; generic shape calls
remove boxing. Redundant pointer-presence booleans were removed only where downstream
reads were unaffected; value/resource presence booleans remain. Production code reduced
84 net lines, excluding tests/docs. JSON fields remain, but moved types change typedbin
fingerprints: rebuild resources. Scheduling name remains sync_ui_layout.

Phase 8 compares phase-7/final binaries sequentially at 300ms x3 medians with setup
outside timing. Differences -4.2%..2.3% are control-path noise, not FPS claims. Every
row retains warm zero allocations; mutation-count regressions directly verify single writes.

| Workload | Before ns/op | After ns/op | B/op / allocs/op |
| --- | ---: | ---: | --- |
| Automatic grid layout | 96,367 | 98,579 | 0 / 0 |
| Stable hierarchy cache | 54.50 | 52.21 | 0 / 0 |
| Hierarchy cache rebuild | 34,784 | 34,034 | 0 / 0 |
| Collect 10,000 2D render items | 3,176,433 | 3,247,003 | 0 / 0 |
| Same-position input, 1,000 targets | 438,472 | 432,909 | 0 / 0 |
| Dragging input, 1,000 targets | 225,418 | 220,227 | 0 / 0 |

## Recorded verification and limits

Targeted ECS/scene/render3d/font/text/UI/HTML/markdown/project/physics/root/editor/window/
WebView tests, vet, generator repeatability and CGO-disabled core checks passed across
phases. Tests cover early/panic exits, filters/ticks, nested queries, structural cleanup,
Slots, scene-owned scripts, index invalidation, message cursor semantics, patch errors,
font discovery and widget mutation counts. Narrow release linkage checks retained 2D
animation cleanup without optional UI/HTML/3D synchronization. No whole-repository test
command or editor screenshot testing was used.

Historical external application checks excluded stale dist exports, which needed rebuilding.
Five pre-existing layout failures were independently reproduced and explicitly excluded;
some full asset regressions were killed for resource limits, so only selected relevant
logic/allocation tests passed. Other historical entry-loading failures lacked default
resources; window-dependent tests needed Xvfb. These were not counted as full-suite success.

Windows amd64/MinGW and Android arm64/NDK API24 CGO builds passed; render3d/HTML/WebView
wasm builds passed. Actual Android 12/API32 x86_64 CPU ECS/scene/UI/HTML tests passed,
later including font/text. Earlier font-dependent fixture failures also reproduced on
baseline; explicit Noto fixtures fixed them without relaxed assertions. Later default
font discovery found /system/fonts/Roboto-Regular.ttf, face 0. Device data was temporary
and cleaned. These results exclude real GPU pixels/FPS; arm64 was compile-only.
Apple font code/source selection exists, but missing Apple SDK/devices prevented full
iOS/Metal compilation/runtime verification. Compile checks do not substitute for devices.

Raw logs: [phase 4](ecs-fourth-benchmarks.txt), [phase 5](ecs-fifth-benchmarks.txt),
[phase 6](ecs-sixth-benchmarks.txt), [phase 6 Android](ecs-sixth-android-tests.txt),
[phase 7](ecs-seventh-benchmarks.txt), [phase 7 Android](ecs-seventh-android-tests.txt),
[phase 8](ecs-eighth-benchmarks.txt), [phase 8 Android](ecs-eighth-android-tests.txt).

## Usage examples

```go
if animator := context.World.GetMut[scene.Animator](context.Entity); animator != nil {
    animator.Speed = script.Speed
    animator.Play()
}
```

```go
for entity, access := range world.Query[scene.Visibility]().All {
    if access.Get().Visible {
        visit(entity)
    }
}

for _, row := range world.Query2[themeColor[engineui.Label], engineui.Label]().All {
    binding := row.A.Get()
    label := row.B.Get()
    label.Color = themeColorFor(mode, binding.Role)
    ecs.CommitComponent(row.B, label)
}
```

```go
query := world.Query[scene.Visibility](ecs.With[scene.UITransform]())
for entity, access := range query.In(entities).All {
    ecs.CommitComponent(access, scene.Visibility{Visible: shouldShow(entity)})
}
```

```go
for event := range reader.All(world) {
    if event.Handled {
        continue
    }
    if err := handle(event); err != nil {
        return err
    }
}
```

```go
return project.NewScriptRegistry(
    project.Script[scripts.CameraReadout]("CameraReadout",
        project.Hooks[scripts.CameraReadout]{
            Update: (*scripts.CameraReadout).Update,
        }),
)
```

```go
if entity, ok := html.ControlByID(world, surface, "editor"); ok {
    engineui.SetHTMLControlValue(world, entity, value)
}
```

```go
nodes := ui.NewNodeSpawner[scene.Sprite2D](world)
nodes.Reserve(128)
entity := nodes.Spawn(ui.NodeOptions{
    Parent: parent, X: 10, Y: 20, StackIndex: 3,
}, sprite)

// Convenience calls reuse the World factory without rebinding each time.
label := ui.SpawnNode2(world, ui.NodeOptions{Parent: entity},
    scene.NewText2D("Onix"), scene.ClipRect2D{Width: 120, Height: 24})
```

```go
removal := world.PrepareRemoval(
    ecs.Type[ui.Control](),
    ecs.Type[scene.InputTarget2D](),
    ecs.Type[scene.InputState2D](),
    ecs.Type[ui.Button](),
)
removal.Reserve(128) // Prepare populated/preallocated sources and target capacity.
changed := removal.Apply(entity)

// Defer through Commands during iteration.
for entity := range world.Query[ui.Control]().All {
    world.Commands().RemovePrepared(entity, removal)
}
err := world.FlushCommands()
```

```go
import (
    "github.com/diiyw/onix/ecs"
    "github.com/diiyw/onix/scene"
    "github.com/diiyw/onix/ui"
)

world := ecs.NewWorld()
label := ui.SpawnNode2(world, ui.NodeOptions{Parent: group},
    scene.NewText2D("Group"), scene.ClipRect2D{Width: 120, Height: 24})
ui.SetText(world, label, "Renamed")

for entity, text := range world.Query[scene.Text2D]().All {
    draw(entity, text.Ref())
}
```

```go
window := world.ChangesSince(lastTick)
for entity, transform := range world.Query[scene.Transform](
    window.Changed[scene.Transform]()).All {
    updateTransform(entity, transform.Ref())
}
lastTick = window.ThisRun()
```

```go
import (
    engineui "github.com/diiyw/onix/ui"
    "github.com/diiyw/onix/ui/html"
)

if entity, ok := html.ControlByID(world, surface, "quick-open-query"); ok {
    engineui.FocusControl(world, entity)
}
```

```go
import "github.com/diiyw/onix/scene/render3d"

render3d.WorldFor(context.World).RegisterMaterial[effectMaterial]()
material := effectMaterial{Shader: shader}
return context.World.Insert(context.Entity,
    render3d.Material[effectMaterial]{Material: &material})
```

```go
app.scriptInputEvents.Discard(context.World)
```

```go
// Prepare once: remove Sprite2D and write two components.
patch := world.NewPatch2[scene.SpriteLayers2D, scene.Size2D](ecs.Type[scene.Sprite2D]())
patch.Reserve(128)

// Apply immediately with one migration.
if err := patch.Apply(entity, layers, size); err != nil {
    return err
}

// During queries, queue values for Commands to apply afterward.
patch.Queue(context.Commands, entity, layers, size)
```

```go
return world.Insert2(content,
    scene.Parent{Entity: area},
    ui.ScrollContent{Area: area, X: localX, Y: localY},
)
```

```go
return cache.patch.Apply(entity, layers, scene.Size2D{Width: 1, Height: 1})
```

```go
if layout := world.GetMut[ui.Layout](sceneInstance.MustEntity("signalsGrid")); layout != nil {
    layout.Type = ui.LayoutGrid
    layout.Constraint, layout.ConstraintNum = ui.LayoutConstraintFixedColumn, 3
}
```
