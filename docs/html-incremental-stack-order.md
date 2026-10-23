# Local HTML z-index updates: phase 12

A `z-index` change reconciles its own node locally and updates only `UIStackIndex` on unchanged descendants, preserving primitives, control revisions, clips and scroll state.

Boxes record `StackChanged` and commit the new `ZIndex` after synchronization. Existing structural order logic applies the delta between old and new base levels to primitives and clips, preserving relative levels. Nested changed roots merge and include descendants affected by stacking-context creation/removal. Already reconciled boxes skip the order delta to avoid applying it twice. Concurrent structural changes reuse owner-list rebuilding instead of adding another subtree traversal.

Shared ownership must be checked before any entity writes so fallback cannot leave partially applied ordering. Position, display and other box-tree changes still use full reconciliation. Existing stacking semantics are unchanged.

Fresh-render comparisons cover positive/negative/auto z-index, nested simultaneous changes, clamping limits, static blocks, flex items, concurrent deletion, clips and child inputs. Descendant controls are not regenerated and retained clips preserve scrolling.

Benchmarks compare `b231491`, three 500ms runs on Linux amd64/i5-11500. A class toggles z-index 1/-1. Current samples are 0 B/op and 0 allocs/op; baseline 0–98 B/op and rounded 0 allocs/op. Shared ownership, reparenting and tree-style changes retain historical fallbacks; descendant ordering costs scale with the affected subtree. See [phase 13](html-incremental-reparent.md).

## Commands

```sh
go test ./ecs/... ./internal/layout ./internal/style ./internal/dom ./internal/css ./project
go vet ./ecs/... ./internal/layout ./internal/style ./internal/dom ./internal/css
git diff --check
```

```sh
go test ./ecs -run '^$' -bench '^BenchmarkHTMLStackChange$' -benchmem -count=3 -benchtime=500ms
```

## Recorded results

| Unrelated inputs | Phase 11 | Phase 12 | Speedup |
| ---: | ---: | ---: | ---: |
| 10 | 77.131 μs | 7.490 μs | 10.3× |
| 1,000 | 8,067.747 μs | 7.774 μs | 1037.8× |

Raw commands and measurements: [html-incremental-stack-order-benchmarks.txt](html-incremental-stack-order-benchmarks.txt).

These are historical measurements for the described paths, not editor frame times. Initialization and warm-up are excluded unless stated otherwise. Allocation guarantees apply only to the explicitly tested warm paths. Editor and screenshot checks are performed manually.
