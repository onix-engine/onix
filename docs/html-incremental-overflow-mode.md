# Local HTML overflow mode changes: phase 11

Existing boxes switching among `visible`, `hidden`, `clip`, `auto` and `scroll` retain layout and rebuild only affected clipping contexts. Changes to `OverflowX`/`OverflowY` no longer directly force full rendering.

The phase-10 membership invalidation and parent-first routing expand context descendants, clear old scroll/clip routes and create or remove `ClipRect2D`, `UIScroll` and `HTMLScrollContent` as required. Unrelated controls and clips retain identities.

Regressions cover single/both axes, nested clipping, flex geometry against full rendering, focusable nodes, absolute descendants, whitespace and 1,000 unrelated controls. Consecutive UI frames preserve live input, selection and applicable scroll positions; non-scrolling modes clear old offsets.

Linux amd64, Intel Core i5-11500, Go 1.26 results are recorded below. Recreating a clip across `visible` transitions allocates; `hidden` retains the entity while changing mode components. Shared ownership, reparenting, tree styles and z-index still fall back in this phase. See [phase 12](html-incremental-stack-order.md).

## Commands

```sh
go test ./ecs -run '^$' -bench '^BenchmarkHTMLOverflowMode$' -benchmem -count=3 -benchtime=300ms
```

## Recorded results

| Unrelated controls | Mode | Median | Allocations |
| ---: | --- | ---: | ---: |
| 10 | hidden | 21.66 μs | 72 B / 2 allocs |
| 10 | visible | 22.39 μs | 680 B / 9 allocs |
| 1,000 | hidden | 91.23 μs | 72 B / 2 allocs |
| 1,000 | visible | 130.34 μs | 680 B / 9 allocs |

Raw commands and measurements: [html-incremental-overflow-mode-benchmarks.txt](html-incremental-overflow-mode-benchmarks.txt).

These are historical measurements for the described paths, not editor frame times. Initialization and warm-up are excluded unless stated otherwise. Allocation guarantees apply only to the explicitly tested warm paths. Editor and screenshot checks are performed manually.
