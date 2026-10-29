# ECS generic methods

The Go 1.27 API migrations, dated 2026-10-29, move operations onto the types that
own their state. The first round covers component access, resources, commands,
bundles and entity slots; the second covers system contexts and caches; the final
round covers typed query, spawner and patch constructors. Source callers, tests,
examples and generator templates migrate directly; the old package functions are removed.

| Owner | Methods |
| --- | --- |
| `*World` | `Insert`, `Insert2`–`Insert8`, `Remove`, `Has`, `Get`, `GetRef`, `GetMut`, `MustMut`, `SetIfChanged` |
| `*World` | `InsertResource`, `GetResource`, `GetResourceMut`, `RemoveResource`, `ResourceChangeTick` |
| `*World` | `ComponentRevision`, `ComponentValueRevision`, `ComponentVersion`, `ComponentStructureVersion`, `OnComponentRemove` |
| `*World` | `WriteMessage`, `AdvanceMessages`, `PrepareRemoval` |
| `*World` | `FrameValue`, `FrameScratch`, `FrameScratchMap`, `ScopeValue`, `MessageScratch` |
| `*World` | `Query`, `Query2`–`Query7` |
| `*World` | `NewSpawner1`–`NewSpawner8`, `NewPatch1`–`NewPatch8` |
| `*SystemContext` | `Local`, `Added`, `Changed`, `ResourceAdded`, `ResourceChanged` |
| `*Commands` | `Insert`, `Remove`, `RemovePrepared`, `Spawn`, `SpawnPrepared`, `Despawn` |
| `Bundle` | `Value`; pointer receivers provide `Put`, `PutEntity`, `Extend` |
| `EntitySlot` | `Has`, `Get`, `GetRef`, `GetMut`, `MustMut` |

```go
world := ecs.NewWorld()
bundle := ecs.NewBundle(2)
bundle.Put(scene.TransformFromXYZ(0, 8, 0))
bundle.Put(scene.Visibility{Visible: true})
entity := world.Spawn(bundle)

world.SetIfChanged(entity, scene.Visibility{Visible: false})
world.InsertResource(scene.ViewportSize{Width: 1280, Height: 720, ScaleFactor: 1})

slot := world.Slot(entity)
transform := slot.MustMut[scene.Transform]()
// Use the component pointer before the next structural mutation.
_ = transform

for entity := range world.Query[scene.Visibility]().All {
    world.Commands().Remove[scene.Visibility](entity)
}
err := world.FlushCommands()
```

`EntitySlot` already stores its world and entity generation. Its methods use that
world without a second world argument. Empty, stale and missing-component lookups
return false or nil; `MustMut` panics with the entity and component type. Slots
with equal entity handles in different worlds remain independent. Explicit
`World.EntityBySlot` validation still rejects a slot bound to another world.

Read access preserves change ticks. `GetMut` and `MustMut` mark writes;
`SetIfChanged` compares comparable values before writing and preserves Added.
Resource lookups retain presence booleans, including when the stored value is nil.
Command streams retain their typed storage, capacity reuse, queue ordering and
failure cleanup. Bundle methods retain dynamic component boxing.

Query views, typed spawners and patches keep their existing implementations and
arity families. World methods construct their world-bound plans and typed views.
The remaining package functions are `NewWorld`, `NewBundle`, `NewSchedule`,
`With`, `Without`, `Unscoped`, `Type` and `CommitComponent`. The constructors and
stateless descriptors have no owning instance. `CommitComponent` remains a function
because its comparable constraint is narrower than `ComponentAccess[T any]`.

The generator in `internal/ecsgen` emits typed World query, spawner, patch and
`Insert2`–`Insert8` methods.
Regenerate with `go generate ./ecs`. Method expressions preserve callback forms,
for example `(*ecs.World).ComponentVersion[scene.Transform]` has type
`func(*ecs.World) uint64`.

## Context and cache methods

System callbacks use `context.Local[Tag, T]()`, `context.Added[T]()` and
`context.Changed[T]()`. Filters retain the context until query construction,
where the query captures its change window and rejects a nil or foreign context.
Resource conditions return false for a nil context, a context with no world or
a missing resource. A method expression such as
`(*ecs.SystemContext).ResourceChanged[Settings]` can be used as an `ecs.Condition`.

```go
window := world.ChangesSince(lastTick)
for entity, transform := range world.Query[scene.Transform](
    window.Changed[scene.Transform]()).All {
    updateTransform(entity, transform.Ref())
}
lastTick = window.ThisRun()
```

Local values belong to the system context and survive schedule recompilation.
FrameValue persists per world; FrameScratch truncates its slice on each call and
FrameScratchMap clears its reused map. ScopeValue follows the selected QueryScope
and restores that scope's cache when switching back. Without a scope it shares
the corresponding FrameValue. MessageScratch retains borrowed data through two
calls to AdvanceMessages. Initial storage creation and capacity growth may allocate.

Cache keys now use distinct named generic types for persistent values, slices
and maps. Previously, FrameValue[Tag, T] and FrameScratch[Tag, *T] used identical
anonymous struct keys in the shared frame map, causing a type-assertion panic.
The regression test exercises both initialization orders and verifies zero
allocations after warming both caches.

## World-bound constructors

All 23 typed constructors are World methods. Query options remain variadic,
including With/Without and context-owned Added/Changed filters. Views retain
their query lifetime guard, scope lookup and per-archetype typed-column binding.
Spawners retain their bound columns and capacity reservation; patches retain
their transition cache, change ticks, cleanup hooks and typed command payloads.

```go
spawn := world.NewSpawner2[scene.Transform, scene.Visibility]()
spawn.Reserve(128)
entity := spawn.Spawn(scene.TransformFromXYZ(0, 8, 0), scene.Visibility{Visible: true})

for entity, row := range world.Query2[scene.Transform, scene.Visibility]().All {
    draw(entity, row.A.Ref(), row.B.Get().Visible)
}

patch := world.NewPatch1[scene.Visibility]()
err := patch.Apply(entity, scene.Visibility{Visible: false})
```

New standalone project scaffolds declare `go 1.27.0`. Independent-project test
sources also use the World query methods. No legacy free-function wrappers remain.

## First-round validation

Go 1.27.3 on Linux amd64 passed ECS tests, ECS race checks and targeted vet checks
for the migrated packages and examples. All ECS tests also passed on the connected
Android API 32 x86_64 device. Repeated ECS generation produced identical files.
Component access, query traversal and recycled command benchmarks retained zero
allocations; the spawner benchmark that includes initialization retained its
existing allocation count. Short timing samples do not establish a speedup.

Targeted package tests passed for scene, render3d, module, project, project/assets,
ui/html, ui/markdown, the root package and the 2048, flappybird, pelican and spine
examples. The animations example and HTML compatibility runner compiled.

Broader checks found existing failures, each reproduced using an overlay of the
workspace source saved immediately before this migration:

| Package | Existing failures |
| --- | --- |
| physics | Dynamic pose precision between ECS writes |
| ui | Generic text-control caret, HTML edit position and text visual resize |
| editor | Progress animation, selection highlight, scene inspector and grid geometry |
| editor | Two generated-project builds cannot resolve dependencies with `GOPROXY=off` |
| examples/webview | Suspended browser visibility |

These failures remain outside this API migration.

## Second-round validation

ECS tests, race checks and the connected Android API 32 x86_64 test run passed.
Regeneration remained stable, targeted vet checks passed, and the editor compiled.
Tests passed for scene, render3d, ui/html, ui/markdown, module, project and the root
package. The same three UI test functions and one physics precision test failed
under both the migrated code and the saved pre-round source overlay; remaining
UI and physics tests passed with those baseline failures explicitly excluded.

The cache-key regression reproduced the panic before the fix and passed after
it. New tests also cover resource-condition method expressions, Added/Changed
windows and rejection of nil or foreign filter contexts. Cache, filtered-query,
message-system and recycled-command benchmarks retained zero allocations.

Three alternating 300ms runs pinned to one CPU measured the combined three-cache
lookup benchmark at 46.62 ns/op before and 48.76 ns/op after (medians, about +4.6%).
Both remained at 0 B/op and 0 allocs/op. These short samples do not establish a
general performance gain; this round primarily improves API ownership and fixes
the cache-key collision.

## Final-round validation

The final round moved 23 query, spawner and patch constructors onto World and
migrated 759 call sites across 176 Go files. Embedded independent-project test
sources use the same methods, and new project scaffolds declare Go 1.27.0.
The public API audit leaves only the eight package functions documented above;
source and documentation scans found no remaining calls to the removed APIs.

ECS tests and race checks passed on Go 1.27.3 Linux amd64. All ECS tests also
passed on the connected Android API 32 x86_64 device. Targeted vet checks passed
for ECS, the generator, affected engine packages, editor and all six examples.
Repeated generation produced identical files, and formatting and diff checks
passed. Warm query traversal and mixed-patch/command benchmarks retained
0 B/op and 0 allocs/op. Timing samples were noisy and support no speed claim.

Explicit package tests passed for scene, render3d, ui/html, ui/markdown, module,
project and the root package. UI, physics, editor and webview checks passed with
the previously reproduced baseline failures explicitly excluded. The 2048,
flappybird, pelican and spine examples passed; animations and the HTML
compatibility runner compiled. No repository-wide recursive test command was run.

Both independent-project tests passed after completing the local dependency
cache: typed registry generation and request execution, plus packaged release
loading, compilation and link inspection. The rerun used
`GOMODCACHE=/home/zhuoh/go/pkg/mod`; subprocesses retained `GOPROXY=off`.
This resolves their validation blocker without changing project dependency
versions or persistent Go environment settings. The other baseline failures
listed above remain outside this migration.

The remaining App, graphics, physics, project, HTML and internal helper changes
are recorded in [generic-method-migration.md](generic-method-migration.md).
