# Local moves from retired HTML contexts: phase 14

A subtree can move to a new parent while its old parent is removed in the same mutation batch. Surviving entities update parentage, clipping and event/focus ownership before existing retirement cleanup removes the old context.

The existing `RemovedBoxes` collection marks retired boxes without a new set or global lookup. Dependency expansion skips retired parents and contexts, preventing old-tree regeneration or duplicate visits to moved descendants. Live new parents still participate normally; moved subtrees retain phase-13 depth and route invalidation. After live synchronization, group retirement destroys old focus, event and clip entities and clears indexes. Retired flags reset after either success or fallback.

Fresh-render tests cover normal/focus/edit source and destination combinations, nested retired parents, absolute descendants, identity retention, index/reference cleanup and reattachment. Headless UI tests scroll first, move and remove, then synchronize two frames to verify values, selection, coordinates, events and retained nested scrolling. With 1,000 unrelated controls, only one moved input and one destination clip update.

Benchmarks compare `f7291fe`, three 500ms runs on Linux amd64/i5-11500. Each operation is a full round trip: move/remove then restore/reattach, including two `SyncHTML` calls. Both versions report 47 allocs/op because retired source contexts must be recreated. Shared ownership and tree-style/box-ownership changes still fall back; global structural traversals remain. See [phase 15](html-incremental-css-order.md).

## Commands

```sh
go test ./ecs/... ./internal/layout ./internal/style ./internal/dom ./internal/css ./project
go vet ./ecs/... ./internal/layout ./internal/style ./internal/dom ./internal/css
git diff --check
```

```sh
go test ./ecs -run '^$' -bench '^BenchmarkHTMLRetiredSource$' -benchmem -count=3 -benchtime=500ms
```

## Recorded results

| Unrelated clipped controls | Phase 13 | Phase 14 | Speedup |
| ---: | ---: | ---: | ---: |
| 10 | 182.45 μs | 82.83 μs | 2.20× |
| 1,000 | 12,580.36 μs | 1,743.24 μs | 7.22× |

Raw commands and measurements: [html-incremental-retired-context-benchmarks.txt](html-incremental-retired-context-benchmarks.txt).

These are historical measurements for the described paths, not editor frame times. Initialization and warm-up are excluded unless stated otherwise. Allocation guarantees apply only to the explicitly tested warm paths. Editor and screenshot checks are performed manually.
