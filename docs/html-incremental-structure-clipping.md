# Clipping during structural HTML mutations: phase 9

Insertion, removal, clearing and reattachment inside CSS overflow containers can reconcile locally. Entire new or removed clip subtrees create or retire clip entities; retained clips update changed bounds and routes without updating unrelated controls.

Structural preflight accepts overflow boxes. A per-pass `Fresh` flag permits first clip creation and resets after either full or partial synchronization. Queued primitives and routes emit parent-first after focus bounds merge, including the document's first clip. Clip movement extends descendant dependencies.

Structural changes recalculate descendant ranges bottom-up even if the parent's border box is unchanged. Bounds caches also stay current while no clips exist, allowing correct first-clip creation later. Retirement removes clip and entity indexes. Deleted-style cleanup removes routes in the same traversal, including nodes with no visible primitive. Owner lists include retained and new clips and update their paint order. Scroll positions survive within range and clamp when content shrinks; `HTMLScrollContent` retains the unscrolled baseline for UI frames.

Tests compare fresh/full rendering for all supported clipping modes, nested clips, first/last clip membership, fixed-size parent range changes, scroll preservation, absolute descendants, focus/edit hosts and external bindings. They verify cleanup and headless consecutive frames. With 1,000 unrelated clip containers, only one target clip updates, no unrelated controls are snapshotted and at most three boxes reconcile.

Zero/positive size membership changes, overflow mode changes, tree-related styles, z-index, reparenting and shared ownership remain full fallbacks in this phase. Structural indexing and topology remain global; creating entities still allocates. Benchmarks compare `ca6483d`, three 500ms runs on Linux amd64/i5-11500/Go 1.26. An 80px child toggles inside a 40px scrolling box. Both versions report 15 allocs/op. See [phase 10](html-incremental-clip-membership.md) and [phase 11](html-incremental-overflow-mode.md).

## Commands

```sh
go test ./ecs/... ./internal/layout ./internal/style ./internal/dom ./internal/css ./project
go vet ./ecs/... ./internal/layout ./internal/style ./internal/dom ./internal/css
git diff --check
```

## Recorded results

| Unrelated clip containers | Phase 8 | Phase 9 | Speedup | Phase 9Allocations |
| --- | ---: | ---: | ---: | ---: |
| 10 | 114.74 µs | 21.57 µs | 5.3× | 6,888 B / 15 allocs |
| 1,000 | 12.39 ms | 869.86 µs | 14.2× | 6,888–6,898 B / 15 allocs |

Raw commands and measurements: [html-incremental-structure-clipping-benchmarks.txt](html-incremental-structure-clipping-benchmarks.txt).

These are historical measurements for the described paths, not editor frame times. Initialization and warm-up are excluded unless stated otherwise. Allocation guarantees apply only to the explicitly tested warm paths. Editor and screenshot checks are performed manually.
