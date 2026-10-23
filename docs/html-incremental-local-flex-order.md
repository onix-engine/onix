# Local flex ordering: phase 17

Order-only changes no longer rebuild the full box-tree topology or run global structural preflight. Changed nodes go directly to the layout cache, which checks their immediate flex containers. Actual insertion/removal and display/position changes retain topology handling.

The existing style pass collects `LayoutOrderNodes` without setting `LayoutTopology` for order alone. Per-pass marks deduplicate containers. Only actual relative-order changes sort, dirty the parent and record `Reordered`. Sorting uses CSS order and retained DOM sibling indexes so returning items to equal order restores DOM order, rather than stabilizing the previous sorted order.

Local rendering uses changed boxes and style dependencies without inspecting all box ownership. Owner lists rebuild only if relative ordering or entity membership changes. Changing numeric order without changing relative order skips rebuilding. Node/container temporary lists clear references after every sync while retaining capacity.

`TopologyVisited` and `OrderContainers` measure work. With 1,000 unrelated inputs, topology visits are zero, one flex container is checked and only two moved controls update. Fresh-layout regressions cover equal-order restoration, repeated changes, deduplication, nested flex, directions, wrapping, mixed DOM changes and fallback. Warm zero-allocation assertions continue passing.

Benchmarks compare `0370a93`, three 500ms runs on Linux amd64/i5-11500. Both versions report 0 allocs/op; baseline 0–7 B/op, current 0–1 B/op. Actual reordering still rebuilds the full owner list and ancestor layout checks still inspect nearby containers. See [phase 18](html-incremental-owner-spans.md).

## Commands

```sh
go test ./ecs/... ./internal/layout ./internal/style ./internal/dom ./internal/css ./project
go vet ./ecs/... ./internal/layout ./internal/style ./internal/dom ./internal/css
git diff --check
```

```sh
go test ./ecs -run '^$' -bench '^BenchmarkHTMLOrder$' -benchmem -count=3 -benchtime=500ms
```

## Recorded results

| Unrelated inputs | Phase 16 | Phase 17 | Speedup |
| ---: | ---: | ---: | ---: |
| 10 | 36.72 μs | 29.25 μs | 1.26× |
| 1,000 | 617.18 μs | 149.37 μs | 4.13× |

Raw commands and measurements: [html-incremental-local-flex-order-benchmarks.txt](html-incremental-local-flex-order-benchmarks.txt).

These are historical measurements for the described paths, not editor frame times. Initialization and warm-up are excluded unless stated otherwise. Allocation guarantees apply only to the explicitly tested warm paths. Editor and screenshot checks are performed manually.
