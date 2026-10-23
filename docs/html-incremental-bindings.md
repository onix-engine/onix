# Local external HTML offset and scroll bindings: phase 6

Stable `SetHTMLNodeOffset` and `SetHTMLNodeScroll` bindings no longer force full reconciliation for ordinary DOM style, geometry or text changes. Bindings replay on updated primitives and clip entities; unrelated coordinates, scroll baselines and control revisions remain unchanged.

Each sync resolves binding IDs to cached DOM nodes and applies effective offsets and scroll bindings in one entity traversal. Full rendering visits all entities; local rendering visits queued entities. `BindingEntities` measures scope. `htmlChild` stores the external viewport and origin. Viewport/origin replacement restores unscrolled coordinates from `HTMLScrollContent` before converting to the new viewport, avoiding repeated scroll displacement. Offset changes update both transforms and scroll baselines; identical repeated bindings do not accumulate offsets.

Control and CSS clip descendants inherit scrolling through their parent entities. Replacing an existing CSS clip with an external binding requires one full refresh before local updates resume. Source replacement clears bindings and cached nodes. ID changes trigger full reconciliation and resolve targets again. All writes through old component pointers must precede `Insert`, which may migrate entities.

Overlapping bindings use registration order: the last matching offset wins; the first matching scroll binding owns top-level primitives. Updating a binding preserves order. Immediate setters still scan all primitives of their HTML owner; this phase limits replay during DOM synchronization. The API operates on local primitive coordinates and does not implement CSS transforms. Viewport lifetime remains the caller's responsibility.

Tests cover local/full equivalence, wrapping, controls, nested clipping, repeated and overlapping bindings, viewport changes, IDs, source replacement and headless scrolling. Warm nonempty input resizing asserts zero allocations and bounded entity visits. Empty editable fragments, first bindings and structural growth may allocate. Benchmarks compare phase 5 `9ae6977`, three 200ms runs on Linux amd64/i5-11500/Go 1.26. One large phase-6 sample reported 1 B/op, and a baseline sample 246 B/op, despite rounded 0 allocs/op.

Topology, attributes, clipping topology, z-index, shared ownership and environment retain earlier fallbacks. See [phase 7](html-incremental-structure-styles.md).

## Commands

```sh
go test ./ecs/... ./internal/layout ./internal/style ./internal/dom ./internal/css ./project
go vet ./ecs/... ./internal/layout ./internal/style ./internal/dom ./internal/css
git diff --check
```

## Recorded results

| Unrelated inputs | Phase 5 | Phase 6 | Speedup | Phase 6AllocationsMedian |
| --- | ---: | ---: | ---: | ---: |
| 10 | 80.21 µs | 15.01 µs | 5.3× | 0 B / 0 allocs |
| 1,000 | 8.33 ms | 87.99 µs | 94.7× | 0 B / 0 allocs |

Raw commands and measurements: [html-incremental-bindings-benchmarks.txt](html-incremental-bindings-benchmarks.txt).

These are historical measurements for the described paths, not editor frame times. Initialization and warm-up are excluded unless stated otherwise. Allocation guarantees apply only to the explicitly tested warm paths. Editor and screenshot checks are performed manually.
