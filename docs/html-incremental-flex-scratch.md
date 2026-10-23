# Reusable flex measurement buffers: phase 16

The phase-15 order benchmark allocated about 21 KB across seven allocations per sync, mostly from full style copies and flex scratch arrays. This phase reuses measurement styles, items, lines and positions for zero allocations in that warmed scenario.

Heap sampling identified escaping style copies in `measureFlexItem`. Each item box owns a private measurement style copied from current computed style before measurement; the original style pointer is restored afterward. Column auto-basis width measurement shares this mechanism. Styles cannot share a global scratch slot because retained fragments may reference them.

Each flex container owns its item, line and position buffers, preventing nested measurement from overwriting another container's data. Generic `slices.Grow` expands capacity. Layout clears pointer slots in item/line buffers to release removed children. Reused single-line positions reset explicitly after multiline layouts. Shared computed styles and layout semantics do not change.

`TestHTMLWarmOrderDoesNotAllocate` checks a round trip containing two DOM mutations and two syncs. Fresh-layout tests cover nested flex, directions, repeated resizing and single/multiple lines, plus cleared backing-array references after child removal.

Benchmarks compare `6e7f9c8`, three 500ms runs on Linux amd64/i5-11500. Baseline 21,784–21,792 B/op becomes 0–9 B/op; every current sample rounds to 0 allocs/op, with a separate strict warm assertion. Large-scene time is essentially unchanged; allocation reduction is the gain. Per-box buffers retain peak capacity through box lifetime; first measurement, growth and new boxes allocate. See [phase 17](html-incremental-local-flex-order.md).

## Commands

```sh
go test ./ecs/... ./html/layout ./html/style ./html/dom ./html/css ./project
go vet ./ecs/... ./html/layout ./html/style ./html/dom ./html/css
git diff --check
```

```sh
go test ./ecs -run '^$' -bench '^BenchmarkHTMLOrder$' -benchmem -count=3 -benchtime=500ms
```

## Recorded results

| Unrelated inputs | Phase 15 | Phase 16 | allocs/op |
| ---: | ---: | ---: | ---: |
| 10 | 41.63 μs | 37.55 μs | 7 → 0 |
| 1,000 | 666.80 μs | 670.58 μs | 7 → 0 |

Raw commands and measurements: [html-incremental-flex-scratch-benchmarks.txt](html-incremental-flex-scratch-benchmarks.txt).

These are historical measurements for the described paths, not editor frame times. Initialization and warm-up are excluded unless stated otherwise. Allocation guarantees apply only to the explicitly tested warm paths. Editor and screenshot checks are performed manually.
