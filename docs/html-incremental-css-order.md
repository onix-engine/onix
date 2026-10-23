# Local CSS order reconciliation: phase 15

CSS `order` changes distinguish order-only tree changes from display/position changes. Layout performs required topology sorting; rendering reuses structural preflight, local primitive reconciliation and owner-list maintenance.

`style.NodeChange.Reorder` identifies order as the only changed tree property. Simultaneous width, color or overflow changes still trigger their normal effects. The `Tree` marker remains so layout rebuilds ordered children instead of retaining stale flex order. Existing-box order-only changes pass structural preflight; unsupported tree changes and shared ownership fall back.

Changed-box queues, clipping dependencies and paint-order updates reuse existing traversals. Focus remains in DOM order. Fresh-render comparisons cover all flex directions, wrapping, negative/equal order, mixed style changes, concurrent deletion and ordinary blocks unaffected by order. Live values, selection, clip identity and scrolling survive. With 1,000 unrelated inputs, only the two reordered inputs snapshot/update.

Benchmarks compare `f512ec0`, three 500ms runs on Linux amd64/i5-11500. A fixed-height flex container toggles order 0/1. Current measurements are 7 allocs/op and 21,787–21,791 B/op; this phase is not allocation-free. Topology and owner maintenance remain global. See [phase 16](html-incremental-flex-scratch.md).

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

| Unrelated inputs | Phase 14 | Phase 15 | Speedup |
| ---: | ---: | ---: | ---: |
| 10 | 98.56 μs | 41.04 μs | 2.40× |
| 1,000 | 7,652.17 μs | 652.00 μs | 11.74× |

Raw commands and measurements: [html-incremental-css-order-benchmarks.txt](html-incremental-css-order-benchmarks.txt).

These are historical measurements for the described paths, not editor frame times. Initialization and warm-up are excluded unless stated otherwise. Allocation guarantees apply only to the explicitly tested warm paths. Editor and screenshot checks are performed manually.
