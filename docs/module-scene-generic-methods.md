# Module and scene generic methods

Module and scene operations now live on the objects that own their state. All
callers, examples, editor generator templates and embedded project test sources
use the method API directly. The old package functions have been removed.

| Owner | Methods |
| --- | --- |
| `*module.Context` | `Resource`, `Get`, `Set`, `Remove`, `Each` |
| `*module.Context` | `BindEmitter`, `BindRequester`, `Send`, `Own`, `Disown`, `InstallMaterial3D` |
| `*module.Content` | `Get`, `Ref`, `Mut`, `MustMut`, `Has`, `Insert`, `SetIfChanged` |
| `*module.Content` | `Resource`, `Store`, `ResourceMut`, `RemoveResource`, `Query`, `Query2` |
| `*module.Content` | `FindPart`, `SpawnNode`, `SpawnNode2`, `BindControl`, `Define`, `Mount`, `UpdateState` |
| `*module.Registry` | `RegisterMod`, `RegisterScene`, `RegisterValue` |
| `module.Message` | `Value`, `RequestValue`, `ResultValue` |
| `module.Slot` | `Read` |
| `*module.Scene` | `Emitter` |
| `*scene.RenderWorld` | `RegisterMaterial2D`, `RegisterCameraRenderer` |
| `*render3d.World` | `RegisterMaterial` |
| `scene.AnimationTrack[T]` | `Sample` |

```go
registry := module.Registry{}
err := registry.RegisterMod("counter", func() Counter { return Counter{} },
    module.Lifecycle[Counter]{Install: installCounter})

emitter, err := context.BindEmitter[int64]("changed")
requester, err := context.BindRequester[string, int64]("length")
value, ok := message.Value[int64]()
request, ok := message.RequestValue[string, int64]()
result, ok := message.ResultValue[int64]()
label, err := slot.Read[string]()

content := context.Content()
transform, ok := content.Get[scene.UITransform](entity)
for entity, access := range content.Query[scene.Visibility]() {
    _ = entity
    _ = access.Get()
}

scene.RenderWorldFor(world).RegisterMaterial2D[CustomMaterial2D]()
render3d.WorldFor(world).RegisterMaterial[CustomMaterial3D]()
sampled, ok := track.Sample(time, interpolate)
```

`RenderWorldFor` and `render3d.WorldFor` retrieve or initialize the render state
stored in an ECS world. Registrations modify that state directly and retain
their existing idempotence and cache invalidation. Private generic helpers for
message storage, queueing, animation binding, hierarchy refresh, render snapshots,
input collection, sprite trails and light extraction also use owning methods.
Animation track and keyframe helpers use their receiver's existing type parameter.

Ownership checks, message borrow lifetimes, typed payload buffers, deferred
structural writes and query boundaries retain their existing behavior. This
migration adds no storage, boxing or traversal. The existing zero-allocation
benchmarks cover message dispatch, request/reply, private content queries,
component rendering callbacks and animation updates.

Constructors and stateless helpers remain functions. This includes `module.New`,
`scene.NewAnimationTrack`, slice/map utilities, type predicates, render-input
descriptors and generic sort/draw algorithms. Helpers whose only owner is the
foreign `*ecs.World` or `*ecs.SystemContext`, including `scene.ActiveTaskCount`,
remain functions because Go methods must be declared in the receiver's package.

Validation passed for module, module/audio, scene, render3d, project,
project/assets, the root package, ui/html, ui/markdown and four game examples.
The editor and all six examples compile. Editor module-build tests pass with
`GOMODCACHE=/home/zhuoh/go/pkg/mod`, including generation determinism, independent
project request execution and packaged release loading. Targeted vet passed.
Module and render3d race checks passed. Scene race checks passed with
`TestTextWindowRelayoutSizesCurrentDocument` excluded: its unwrapped case measures
3 allocations under race instrumentation against a limit of 2; its normal test
passes. No repository-wide recursive test command was run.

Full module, scene and render3d tests also passed on the connected Android API 32
x86_64 device. Test fixtures were copied to the device; a build overlay remapped
the test font loader's host source path to that fixture directory. The overlay
changes only the test harness and is not part of the engine source.

The remaining App, graphics, physics, project, HTML and internal helper changes
are recorded in [generic-method-migration.md](generic-method-migration.md).
