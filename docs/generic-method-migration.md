# Remaining generic-method migration

The Go 1.27 migration moves 77 additional free functions onto their existing
owners. It updates 963 direct calls, along with method expressions, examples,
embedded project source, generator templates and documentation. There are no compatibility
wrappers. Existing ECS/module/scene migrations are described in
[ecs-generic-methods.md](ecs-generic-methods.md) and
[module-scene-generic-methods.md](module-scene-generic-methods.md).

## Public API

| Owner | Methods |
| --- | --- |
| `*onix.App` | `InstallSystems[T]`, `LoadModules[P]` |
| `*graphics.Canvas2D` | `ScratchValue[Tag, T]`, `RetainUntilFlush[T]` |
| `*physics.World`, `*physics.World3D` | `AddCollider[S]`, `RemoveCollider[S]` |
| `*project.SceneInstance` | `Component[T]`, `RequireComponent[T]`, `BindControl[T]`, `BindControlEntity[T]` |
| `project.ResourceLoadContext` | `RegisterNamedResource[T]` |
| `*project.Resources` | `LoadNamedResource[T]` |
| `project.ScriptComponentSpec` | `DecodeScriptValues[T]` |
| `project.AnimationKeyframeSpec[T]` | `EffectiveAnimationTween` |
| `*html.Definition[P, S]` | `DefineScopedChild[CP, CS]`, `ComponentChild`, `MountComponent`, `HydrateComponent`, `MountComponentInto`, `HydrateComponentInto` |
| `*html.View` | `AppendComponentChild[P, S]` |

Both physics world types use `AddCollider` and `RemoveCollider`; the receiver
identifies the simulation dimension. HTML definitions and animation keyframes
reuse their receiver's existing type parameters.

```go
if err := app.InstallSystems[onix.Physics3D](); err != nil {
    return err
}
entry, err := app.LoadModules[OnixContentProfile](plan, registry, options)

world := physics.NewWorld3D()
body := world.CreateBody(physics.NewBodyDef3D(physics.BodyDynamic))
collider := world.AddCollider(body, physics.NewColliderDef(physics.Sphere{Radius: 1}))
world.RemoveCollider(collider)

transform, err := instance.Component[scene.Transform]("player")
slot, err := instance.RequireComponent[scene.Transform]("player")
```

Method expressions retain callback signatures, for example
`(*onix.App).InstallSystems[onix.Physics3D]` is a `func(*onix.App) error`.
Nil receivers retain the original checks; callers passing a literal nil must
supply its receiver type, for example `(*html.View)(nil)`.

## Internal ownership

ECS component registration, archetype columns, component rows, resource slots,
message buffers, change filters, patch writes and typed command streams now use
methods on World, SystemContext, archetype, patchRow and Commands. The ECS
generator emits the same methods and regenerates deterministically.

Physics runtime synchronization and collider state conversions use their runtime
and state receivers. Scene animation target resolution and collider settings use
project-owned plans, instances and specifications. Editor animation selection,
key editing, rendering, viewport tracks and temporary resource-overlay cleanup
use existing editor state. HTML reconciliation writes, definition scope and
mounting use writer/definition receivers. UI composition and tree helpers reuse
existing composition and node types.

Render buffer retention owns its generic trimming, reset and JS copy helpers.
GLES/WebGL/WebGPU drivers own upload and shader operations that already received
the driver. GenerationPool owns native resource destruction and reuses its value
type parameter for the destroy callback. FileSystem owns typed path operations.
No additional state, boxing, interfaces or traversals are introduced.

## Functions retained after the audit

Constructors and stateless algorithms remain functions, including project
loaders, HTML Define/DefineScoped, UI spawn/bind helpers, sort/slice/map helpers,
codecs and descriptor builders. Helpers operating on a foreign `*ecs.World`,
`*ecs.SystemContext`, `*render.Device`, or C/COM types cannot declare methods on
those types in their current package. No new wrapper types are introduced just
to provide a receiver.

`ecs.CommitComponent[T comparable]` retains its narrower comparison constraint;
`ComponentAccess[T any]` cannot acquire that constraint through a method.
Specialized helpers such as the string-keyed `loadNamedResource` and shader
constructors using concrete GenerationPool value types remain functions: those
specialized instantiations cannot be used as method receiver declarations without
redesigning their existing type constraints or operations.

## Validation

Go 1.27.3 Linux amd64 passed targeted normal tests for ECS, fs, graphics, project,
project/assets, module, module/audio, scene, render3d, ui/html, ui/markdown and the
root package. The 2048, flappybird, pelican and spine examples passed. The editor,
all six examples, HTML compatibility runner and ECS generator compile. Targeted
vet passed, and render/graphics/fs cross-compile for js/wasm. Repeated ECS
generation produces identical files.

UI, physics, editor and webview checks pass after excluding the existing failing
tests. Every excluded failure was reproduced with a Go overlay restoring the
source saved before this migration. The render Vulkan steady-state subprocess
also segfaults with both the migrated and pre-migration source; other render tests
pass. Independent-project request execution and packaged release tests pass with
`GOMODCACHE=/home/zhuoh/go/pkg/mod`, while their child processes retain
`GOPROXY=off`.

ECS, fs, graphics, module and project race checks pass. HTML race checks pass
after excluding 13 allocation-limit tests that also fail with the saved
pre-migration source; normal HTML tests pass without exclusions. These checks
report no data races. Component reads, query
traversal, mixed patches, recycled commands, message dispatch, request/reply and
reused HTML child slots retain 0 B/op and 0 allocs/op. HTML child snapshots retain
one allocation and component state synchronization retains two allocations,
matching pre-migration measurements. Timing samples do not establish a speedup.

The connected Android API 32 x86_64 device passes the full ECS suite and the
physics suite with the existing dynamic-pose precision failure excluded.
No repository-wide recursive test command or editor screenshot test was run.
