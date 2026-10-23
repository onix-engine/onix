# Local primitives for structural HTML mutations: phase 8

Ordinary insertion, removal, clearing and reattachment reconcile only new boxes and boxes whose geometry or content changed. Unchanged primitives, control revisions and input state survive; document-order updates are handled separately. This phase requires stable retained parentage, no CSS overflow clips and no shared primitive ownership. Following flow boxes still update if they move.

Topology cleanup produces `Removed` during its existing traversal; rendering consumes this retirement list and clears temporary references. Each box retains its previous parent and base paint level. A structural preflight traverses the current tree, validates ownership, records new boxes and queues them; unsupported changes use full reconciliation.

Control, focus and edit dependencies extend the normal changed queue. Live boxes synchronize before removed groups retire, preserving entities whose ownership transferred. Retirement clears node, focus, snapshot and event indexes. Owner-list rebuilding also adjusts retained `UIStackIndex` and `HTMLFocusable.Order` by base-level differences, preserving relative levels within primitives and nested stacking contexts. The existing UI control pass updates select layer caches without rebuilding options or open state. External bindings replay only on reconciled primitives.

`OrderUpdated` counts order maintenance. New styles, boxes and entities can allocate. Fresh-render and headless UI regressions cover blocks, controls, focus/edit hosts, stacking, remove/reattach, flow movement, event retirement/recovery, external bindings and fallback. With 1,000 unrelated inputs, removing a child inside a fixed-height container snapshots and updates no native controls and reconciles at most three boxes.

Global topology, ID/order and style-cleanup traversals remain, as do owner-list maintenance and preflight. Benchmarks compare `2caf767`, three 500ms runs on Linux amd64/i5-11500/Go 1.26, with a child toggled inside a 40px container. This differs from phase 7's auto-height text benchmark. See [phase 9](html-incremental-structure-clipping.md).

## Commands

```sh
go test ./ecs/... ./html/layout ./html/style ./html/dom ./html/css ./project
go vet ./ecs/... ./html/layout ./html/style ./html/dom ./html/css
git diff --check
```

## Recorded results

| Unrelated inputs | Phase 7 | Phase 8 | Speedup | Phase 8Allocations |
| --- | ---: | ---: | ---: | ---: |
| 10 | 81.24 µs | 17.58 µs | 4.6× | 6,816–6,817 B / 13 allocs |
| 1,000 | 7.67 ms | 795.46 µs | 9.6× | 6,807–6,811 B / 13 allocs |

Raw commands and measurements: [html-incremental-structure-rendering-benchmarks.txt](html-incremental-structure-rendering-benchmarks.txt).

These are historical measurements for the described paths, not editor frame times. Initialization and warm-up are excluded unless stated otherwise. Allocation guarantees apply only to the explicitly tested warm paths. Editor and screenshot checks are performed manually.
