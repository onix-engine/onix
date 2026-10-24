# ECS warm paths and caches

Generic With/Without/Added/Changed options avoid per-query closures. World scratch builds masks, but query plans own independent copies so nested/later queries cannot overwrite them. Change-filter tick intervals snapshot when constructing a view; reconstruct the view for a new SystemContext interval.

After type/plan/archetype warm-up, ordinary queries and up to eight Added/Changed filters run without allocation. With/Without do not consume change-filter slots. Above eight change filters, a new view snapshot can allocate; iteration of a saved view reuses pooled overflow bindings once warm. Each iterator independently binds tick slices on archetype changes, avoiding per-row column lookups and supporting nesting.

Change filters skip unchanged 4096-row groups before testing 64-row blocks and individual ticks. Group summaries share the existing bounds arrays and rebuild lazily after writes. Component writes still update row ticks and public revisions immediately. Command descriptors use pointer-free stream IDs (24 bytes on amd64); typed payload streams retain and clear references independently. Patch/removal Reserve warms populated or preallocated source tables, skips replacement-only targets and does not grow world entity slots. Reserve source spawners first when preparing an empty scene. See [balanced optimization](ecs-balanced-optimization.md) for measurements and validation.

Use `Query[T](world).All` and multi-column views. Read with GetRef/Ref and acquire Mut only after deciding a write is needed. Mutable access marks change. FlushCommands recycles queue/spawn storage and clears references on success, error and panic, cancelling unexecuted reservations. PreparedBundle supports allocation-free warmed spawn/flush/despawn within capacity, excluding user callbacks and growth.

Text phases collect selected labels and measure owner icons/labels in one query; post-layout refresh prevents stale widths. Generic scratch indexes clear old keys and preserve first-match/empty-label semantics without marking association components Changed. FindPart[P,T] caches entity IDs, rebuilding after relationship/part versions and migrations, including matches lacking Text2D/UITransform. Before/after-layout measurements remain separate.

SetIfChanged updates existing unequal comparable components only; equal/missing/stale returns false without insertion. CommitComponent commits with the query tick. Equal values do not trigger Changed. HTML lives in ui/html; pure UI forwarding APIs are removed, but form-state operations such as SetHTMLControlValue remain. Keyed slots/children share matching and cleanup; slots maintain static-child indexes on commit.

UI hierarchy caches retain parent links, ordering inputs, depth and sorted results. Every pass still inspects topology inputs; only actual relationship/order/membership changes recompute depth/sorting. This detects direct mutable Parent writes, removals, generation reuse and ancestors without Layout. Unrelated archetype traversal reordering may conservatively rebuild. The cache does not skip layout itself, visibility or Widget constraints.

[Traversal optimization](ecs-traversal-optimization-report.md) combines cursor/selection indexing, cache checks, menu metrics, rollback, migration and layout passes. [Memory reuse](ecs-memory-reuse.md) bounds pooled bundle/query/scheduler scratch and clears references with independent nested borrows. [Hot-path work](ecs-hotpath-optimization-report.md) accumulates text advances, reuses windowed line layouts, propagates dirty transforms and skips unchanged controls/blockers/Tab work. Keep normal-build allocation assertions separate from race instrumentation and avoid concurrent high-load profiling while benchmarking.

## Examples and commands

```sh
go test ./ecs ./scene ./ui/html ./ui ./ui/markdown ./physics -p=1
go vet -p=1 ./ecs ./scene ./ui/html ./ui ./ui/markdown
go test ./ecs ./scene ./ui -p=1 -run '^$' -bench 'Benchmark(FilteredQueryTenThousand|CommandSpawnPreparedRecycled|UILayoutHierarchy|SelectorLabelMetrics)$' -benchmem
```
