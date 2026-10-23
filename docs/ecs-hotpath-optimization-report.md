# ECS stalls and unchanged-state synchronization

Recorded 2026-10-01. Character wrapping accumulates advances, kerning, spacing and tabs instead of remeasuring prefixes, treating inline objects as indivisible. Windowed text retains line breaks, alignment, baselines and object geometry, binary-searching its first line and rebuilding only visible glyphs/outlines; measurement-to-draw transitions reuse line layout.

UI writes compare generic values before committing; pure text measurement borrows references. Dimensions, style, position, visibility, layers and scroll parameters only mark Changed when different. Selection layers invalidate matrices only when changed. Changed queries collect dirty inputs once and propagate affected subtrees; no input/topology/viewport changes return immediately.

Tab synchronization returns without Parent traversal when no Tab exists and rebuilds indexes on Parent versions. Icon/label widths share a query; blocker reads merge three queries and skip unchanged inputs. Numeric button text placement joins the common text phase. World transforms reuse workspace SystemContext and only inspect Parent changes on Transform entities; unrelated UI writes do not recompute world matrices.

Go 1.26/Linux amd64/i5-11500 results use three optimized runs; baselines are approximate values recorded before edits. Existing SelectorLabelMetrics and TraversalValueSurfaces retain zero allocations. Small transform timing differences are not a speed claim; removing allocations and unrelated invalidation is the benefit.

Topology/component migration conservatively validates all UI inputs and rebuilds links but recalculates only invalid subtrees. Viewports resolve all nodes and respect simultaneous reparenting. External global-transform writes/reinsertion are repaired, cycles report errors and can recover, reused entity slots reject old parents. Text preserves Unicode, kerning, tabs, spacing, alignments, strokes, fractional raster scale and explicit line-height bounds. Blocker priority stays Size2D→Text2D→ScrollArea. Before/after-layout stages refresh dependent measurements separately.

Targeted regressions cover window/full text equivalence, inline objects, parent/viewport changes, cycle recovery, global matrices, scrollbar geometry, unchanged controls and warm zero allocation. ECS/physics/root/project/vet checks passed. No GPU timing, device, editor or screenshot tests were performed.

## Examples and commands

```sh
go test ./ecs ./ui -p=1 -run '^$' -bench '^BenchmarkHotpath' -benchmem -benchtime=500ms -count=3
```

```sh
go test ./ecs/... ./physics -p=1 -count=1
go test . ./project/... -p=1 -count=1
go vet ./ecs/...
git diff --check
```

## Recorded measurements

| Workload | Approximate baseline | Current median | After B/op / allocs/op |
| --- | ---: | ---: | ---: |
| 4,000 characters / width 600 / forced character wrapping | 16.65 ms | 1.357 ms | 0 / 0 |
| 10,000 UI nodes / move one | 1.721 ms | 1.952 μs | 0 / 0 |
| 400 lines / change approximately 40-line paint window | 2.964 ms | 309.843 μs | 0 / 0 |
| 10,000 Parent nodes / no Tab | 88 μs | 214.7 ns | 0 / 0 |
| 10,000 static controls / repeat size sync | Not separately recorded | 93.86 ns | 0 / 0 |
| 10,000 world transforms / move one | 1.45 μs, 32 B / 1 alloc | 1.393 μs | 0 / 0 |

Performance figures describe the recorded workloads and environments, not complete application frame rates. Warm allocation guarantees exclude first use and capacity growth.
