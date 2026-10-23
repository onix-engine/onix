# ECS repeated-traversal optimizations

Recorded 2026-09-14. Eight changes combine compatible traversals while preserving dependency stages. Cursor/selection ValuePart lookup builds an owner→Entity scratch index only when needed, replacing O(S×T) scans with O(T+S). Command trimming scans the retained queue once to record each typed stream and spawn prefix, then truncates streams: O(K×Q) becomes O(Q+K) without a temporary map.

Auto-grid measures cell maxima once per layout, recalculating row/column counts after container changes without remeasurement. Horizontal/vertical resizing retains the necessary total-size pass but combines child resizing and positioning. One read-only TextField query collects missing password/composition caches; insertion waits until the query ends. Menu/ContextMenu prepare owner sets then share one text-width query, measuring shared labels once.

Generic typedColumn.moveTo combines component copying and swap removal before final entity-row updates. Archetype creation sets mask bits while creating columns. Fixed repeated linear passes are still O(n); these changes reduce constants, while relationship/command changes remove nested scans.

Indexes preserve the first query-order match even when expected display components are absent, store entities rather than pointers across migration and refresh per synchronization stage. Rollback keeps prefix payloads/spawns, cancels dropped reservations and clears retained-length counters. Migration preserves Added/Changed ticks and swapped rows. Layout reads effective sizes after writes; menu measurements do not mark Text2D Changed.

Go 1.26/Linux amd64/i5-11500 medians use three runs, with all six warm paths 0 B/op/0 allocs/op. Command trimming models partial rollback, not every successful Flush. Fontless menu/cursor cases isolate lookup/surface costs rather than full typography. Grid positioning and archetype creation had correctness/traversal checks without separate timing. Widget phases, transform parent dependencies and bounds-center/radius stages remain separate.

Regression coverage includes rollback/reuse, migration/ticks, directions/scaling/zero space, grid, duplicate owners, cache recovery, real-font measurement and unchanged-state behavior. Targeted ECS/UI/physics/root/project tests and vet passed without editor, screenshots, GPU or mobile measurements.

## Examples and commands

```sh
go test ./ecs ./ui -run '^$' -bench '^BenchmarkTraversal' -benchmem -count=3
```

```sh
go test ./ecs/... ./physics
go test . ./project/...
go vet ./ecs/...
git diff --check
```

## Recorded measurements

| Workload | Before ns/op | After ns/op | Time reduction | After B/op / allocs/op |
| --- | ---: | ---: | ---: | ---: |
| Command trim / 8 types / 1,024 inserts | 18,559.0 | 3,395.0 | 81.7% | 0 / 0 |
| Migration / 8 retained components / insert-remove round trip | 444.7 | 352.2 | 20.8% | 0 / 0 |
| Auto-grid / 1,000 children / adaptive container | 158,792.0 | 89,831.0 | 43.4% | 0 / 0 |
| Text caches / 1,000 TextFields | 35,017.0 | 20,271.0 | 42.1% | 0 / 0 |
| Menu metrics / 1,000 shared labels | 155,144.0 | 97,342.0 | 37.3% | 0 / 0 |
| Cursor surfaces / 300 cursors | 401,640.0 | 154,415.0 | 61.6% | 0 / 0 |

Performance figures describe the recorded workloads and environments, not complete application frame rates. Warm allocation guarantees exclude first use and capacity growth.
