# Local flex reflow with stable outer geometry: phase 19

A single flex container's order change can reflow from cached containing-block inputs without visiting unrelated root siblings when its external flow geometry remains unchanged.

Eligibility requires one actual reordered container, valid caches and every dirty layout node inside that subtree. The target and its ordinary block ancestors must be statically positioned; positioned/flex ancestors and multiple reordered containers retain the original traversal.

Reflow uses cached containing/absolute inputs and compares old/new `FlowMarginBox` and `BorderBox`. Changed outer geometry preserves ancestor dirtiness and immediately resumes root layout before ECS writes. Stable outer geometry skips ancestor/sibling layout but still records ancestors as rendering dependencies for focus and descendant overflow ranges.

`LocalFlexReflows` counts attempts. With 1,000 unrelated inputs, one local reflow visits at most ten boxes and updates only two moved controls. Fresh-render tests cover auto-height changes, external simultaneous resizing, unsupported ancestors and absolute overflow changes inside stable outer geometry. Existing order, wrapping, state and warm allocation regressions remain.

Benchmarks compare `669ea62`, three 500ms runs on Linux amd64/i5-11500. All current samples are 0 B/op and 0 allocs/op. Multiple roots, outside dirtiness, unsupported ancestors or outer geometry changes fall back; a failed local attempt adds layout work. Ancestor context dependencies may still expand rendering scope. See [phase 20](html-incremental-multi-flex-reflow.md).

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

| Unrelated inputs | Phase 18 | Phase 19 | Speedup |
| ---: | ---: | ---: | ---: |
| 10 | 30.58 μs | 28.31 μs | 1.08× |
| 1,000 | 98.07 μs | 31.15 μs | 3.15× |

Raw commands and measurements: [html-incremental-local-flex-reflow-benchmarks.txt](html-incremental-local-flex-reflow-benchmarks.txt).

These are historical measurements for the described paths, not editor frame times. Initialization and warm-up are excluded unless stated otherwise. Allocation guarantees apply only to the explicitly tested warm paths. Editor and screenshot checks are performed manually.
