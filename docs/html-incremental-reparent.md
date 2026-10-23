# Local HTML reparenting: phase 13

Moving a node with `Remove` plus `Append` before one sync retains boxes/entities and refreshes only the moved subtree and both contexts. Both parents must remain in the document; retired source parents, changed box ownership and shared primitives retain full fallback in this phase.

The existing parent-first topology pass updates depths while previous parent pointers remain available through entity synchronization. A moved root enters the queue with invalid clip routes even if style, clip membership and document coordinates are unchanged. Old and new parents enter the dependency queue so focus bounds, events and inherited control contexts update.

Merged invalidation roots clear moved-subtree routes, then rebuild clip, scroll and external bindings parent-first. Retained nested clips preserve scroll positions, inputs preserve live values and selection. Existing owner-list and paint-order traversals handle ordering without a separate document entity index.

Fresh-render comparisons cover different depths, inheritance, stacking, clip/focus/edit contexts, nested clips, absolute and geometrically unchanged descendants, and external bindings. Headless UI tests scroll before moving and verify subsequent frames, identity, values, selection, baselines and parents. With 1,000 unrelated clipped controls, one moved input and two parent clips update.

Benchmarks compare `bf6945c`, three 500ms runs on Linux amd64/i5-11500. One input moves between fixed-height clipped containers. Both versions report 6 allocs/op; baseline 160–300 B/op, current 160–164 B/op. Structural topology, DOM order and owner traversal remain global; context dependencies can expand scope. See [phase 14](html-incremental-retired-context.md).

## Commands

```sh
go test ./ecs/... ./internal/layout ./internal/style ./internal/dom ./internal/css ./project
go vet ./ecs/... ./internal/layout ./internal/style ./internal/dom ./internal/css
git diff --check
```

```sh
go test ./ecs -run '^$' -bench '^BenchmarkHTMLReparent$' -benchmem -count=3 -benchtime=500ms
```

## Recorded results

| Unrelated clipped controls | Phase 12 | Phase 13 | Speedup |
| ---: | ---: | ---: | ---: |
| 10 | 117.13 μs | 33.76 μs | 3.47× |
| 1,000 | 12,157.88 μs | 656.95 μs | 18.51× |

Raw commands and measurements: [html-incremental-reparent-benchmarks.txt](html-incremental-reparent-benchmarks.txt).

These are historical measurements for the described paths, not editor frame times. Initialization and warm-up are excluded unless stated otherwise. Allocation guarantees apply only to the explicitly tested warm paths. Editor and screenshot checks are performed manually.
