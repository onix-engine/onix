# Generic architecture optimizations

Implemented on 2026-10-29 against the existing uncommitted generic-method migration.
The baseline is a source snapshot taken before these six optimizations, rather
than Git HEAD. No benchmark below compares against the pre-migration engine.

## Implemented changes

1. **Scope queries use archetype membership.** Membership bitsets are compiled
   after scope resets or structural mutations. A sorted list of occupied blocks
   skips empty regions; only occupied blocks need clearing on rebuild. Fully
   selected tables retain the ordinary contiguous iteration path. Changed/Added
   filters keep their group and block rejection. Generations are checked when
   compiling membership, including after migrations and swap removal.
2. **Transform records are dense.** World/UI transform workspaces use
   `internal/entitycache.Store[T]`: a four-byte entity index and dense active
   records, with generation checks and swap deletion. Reset clears live records
   and their indices, including reference-bearing values. It retains capacity
   for reuse. Hierarchy storage is unchanged.
3. **Bindings share entity lookup and preserve change tracking.**
   `World.BindComponent[T]` resolves a type once; `World.Row` validates and locates
   an entity once. UI transform refresh reads its inputs through these bindings.
   Global transforms commit only changed matrices. Animation bindings are fetched
   once per system run, and float, rotation, texture and visibility samples mark
   their target components only when values change. Animation filtering reads the
   Animator before acquiring write access.
4. **Common creation uses typed spawners.** Module UI/world roots, children and
   owned children have lazily initialized typed factories. Dynamic content writes
   into the transform Bundle directly. Anchors are authored before spawning;
   alternate transforms are inserted together. Deferred command spawning retains
   the existing command ordering and Bundle representation.
5. **Content capabilities compose.** `WithContent[P, F]` adds animation, layout,
   trails, HTML, particles, tilemaps, Spine, 2D physics or spatial content. Duplicate
   capabilities are installed once. Writers append into one Bundle. Upload and
   lifecycle callbacks include selected features only. Generator system and
   content selection share one descriptor table; no feature implicitly selects
   FullContent. All six generated examples use the new profiles. Obsolete fixed
   animation-profile wrappers were removed.
6. **Glyph eviction is incremental.** Plain, phased and outlined records share a
   bounded 4096-entry CLOCK ring. Hits set reference bits; overflow evicts one cold
   record instead of clearing every map. Upload flushing visits queued page indices
   and inspects at most eight additional atlas slots for reclamation. Queues hold
   no strong page pointers; returned glyphs and pending draws still preserve atlas
   lifetimes.

## Measurements

Linux amd64, Go 1.27.3, Intel i5-11500. Three runs, one benchmark CPU,
200–300 ms per run; values are medians. These are microbenchmarks, not measured
application-wide CPU or frame-time reductions. Raw samples are in
[generic-six-optimizations.txt](benchmarks/generic-six-optimizations.txt).

| Workload | Before | After |
| --- | ---: | ---: |
| Scope: 10k entities, 10k matching, read | 68.05 µs | 36.95 µs |
| Scope: 10k entities, 1k matching, read | 38.16 µs | 4.02 µs |
| Scope: 10k matching, one Changed row | 48.08 µs | 0.725 µs |
| Scope: 1k matching, one Changed row | 33.45 µs | 0.426 µs |
| Four component reads: ordinary vs shared Row | 91.72 ns | 26.84 ns |
| Module: create world and 1000 world roots | 716.45 µs / 3270 allocs | 389.29 µs / 272 allocs |
| Module: create world and 1000 UI roots | 785.02 µs / 4317 allocs | 422.02 µs / 319 allocs |
| 10k world transforms, one update | 1.089 µs | 1.003 µs |
| 10k UI transforms, one update | 1.925 µs | 1.564 µs |
| 256 playing animators | 52.48 µs | 50.31 µs |
| 256 paused animators | 27.75 µs | 25.10 µs |

Scope, component reads, transform updates and animator benchmarks allocate zero
bytes after warmup. Selecting only the first and last of 100k entities costs
approximately 135 ns and zero allocations. Warm typed spawn/despawn with reused
storage also passes a zero-allocation regression; the table's creation measurements
include growing a new world and its columns.

With 100k unrelated entities and one world/UI transform each, retained transform
record storage changes from **18.31 / 40.44 MiB** to **401608 / 401840 bytes**
(approximately 0.383 MiB each). This counts backing capacities of record/index
arrays, excluding workspace headers, active lists, ECS columns and the separate
4.58 MiB hierarchy cache. The reduction is specific to sparse participation;
when every entity participates, dense records add index/entry bookkeeping.

## API contracts and remaining costs

- Ordinary scoped queries use table order and set membership: duplicate scope
  entities are visited once. `Query.In` preserves explicit order and duplicates.
  Code that needs a particular order must use an explicit list or sorting.
- Scope resets and structural changes rebuild membership; that work is linear
  in scope members plus occupied blocks, with block sorting. Warm queries reuse
  it. An unchanged scope cannot avoid rebuilding after a world structural change.
- EntityRow and binding accesses expire after structural mutations; bindings
  themselves survive migrations. Foreign-world or expired rows panic. Do not
  retain accesses or column pointers across spawning, removal or migration.
- Dense-store pointers expire on Ensure/Delete/Reset. Its small index still
  scales with the largest entity slot observed; capacity is retained for reuse.
- SpatialContent groups models, cameras, lights and 3D physics because cooked
  colliders consume model data. It is coarser than the other capabilities.
- CLOCK eviction is amortized: a miss may clear up to 4096 reference bits before
  finding a victim. Atlas allocation still searches existing pages; bounded
  reclamation applies to flushing, not every cold allocation. Live glyphs may
  legitimately retain pages beyond the reusable atlas byte budget.
- Existing release-size measurements predate this work. Link exclusion tests
  pass; no new application-size reduction is claimed here.

## Validation

Successful package and device output is saved in
[generic-six-validation.txt](benchmarks/generic-six-validation.txt).

Targeted full package tests passed for ecs, entitycache, scene, module, project,
text, the root app, scene/render3d, ui/html, ui/markdown and the 2048, Flappy Bird,
Pelican and Spine examples. The animation example compiles. Editor module build,
packaging, generated binding and forbidden optional symbol tests passed. Targeted
vet passed. ECS generation was checked for deterministic output.

Race tests passed for ecs, entitycache, module, text and the changed scene
transform/animation tests. The complete scene race suite has a preexisting
`TestTextWindowRelayoutSizesCurrentDocument/unwrapped` allocation assertion
(3 versus at most 2); the identical failure reproduces with the baseline overlay.

Android x86_64/API 32: the full ECS suite passed on 10.0.0.174:16384, along with
selected transform-cache, repeated-animation, bound-animation, quaternion-rotation,
CLOCK eviction, record-budget and atlas-reclamation tests. The scene test font
paths were remapped to pushed fixtures for headless device execution.

Existing failures reproduced against the pre-optimization source snapshot:

- ui: `TestBindControlExpandsGenericTextControlMode`,
  `TestHTMLInputEditsAndEmitsEvents`, `TestHTMLTextVisualRolesSurviveResize`.
- physics: `TestDynamicPoseRetainsPrecisionBetweenECSWrites`.
- examples/webview: `TestWebViewModuleOwnsPageBoundsBridgeAndLifecycle`.

No editor launch, screenshot testing, broad `go test ./...`, commits or visual
acceptance were performed.
