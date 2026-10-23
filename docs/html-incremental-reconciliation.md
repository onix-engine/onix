# Local HTML primitive reconciliation: phase 3

Phase 3 (`d5e42bc`) reconciles primitives belonging to affected boxes, skipping ECS writes, node-index rebuilding and global group cleanup for unchanged boxes. The retained layout tree determines geometry dependencies. See [phase 4](html-incremental-contexts.md) for clipping, scrolling and events.

A reusable `Changed` list records each reflowed or translated box once during existing layout traversals, including repeated flex measurement, relative translation and bottom-position correction. Style changes add paint-only boxes. Full rendering builds a box-to-own-entities/group index during primitive generation, excluding child-box primitives.

Local synchronization preflights interaction, clip and stacking dependencies before ECS writes; rewrites only selected boxes using DOM identity and slots; and retires only their unused slots and node indexes. Stable membership preserves the owner list. Wrapping or paint membership changes rebuild it in document order while retaining other components.

Unaffected controls need no snapshots and retain values, selection, input state and render revisions. Late event binding can still find existing surfaces. Event membership changes invalidate the index and require one full refresh before local updates resume. Index storage grows with boxes and primitives; initialization and capacity growth can allocate.

Historical full fallbacks include topology, ordinary content attributes, environment and z-index changes; affected controls, focus/edit hosts and shared ownership; document events, external bindings and generated overflow clips. Zero-sized contexts are checked through DOM/style dependencies, not just previous entity existence. Ancestors still inspect direct children and translations visit moved descendants. Membership changes rebuild owner lists.

Fresh-render comparisons cover wrapping, empty text, paint changes, flex, positioning, translation, stacking fallback, control state, late events and recovery after topology updates. Tests with 10 and 1,000 unrelated nodes reconcile two boxes and rewrite one retained text entity. Benchmarks compare `736886c`, three 200ms runs on Linux amd64/i5-11500/Go 1.26. They preserve existing CSS semantics.

## Commands

```sh
go test ./ecs/... ./html/layout ./html/style ./html/dom ./html/css ./project
go vet ./ecs/... ./html/layout ./html/style ./html/dom ./html/css
go test ./ecs -run 'TestHTMLPartial|TestHTMLWarmTextMutationAllocations' -count=3
```

## Recorded results

| Unrelated divs | Phase 2 | Phase 3 | Speedup | Phase 3Allocations |
| --- | ---: | ---: | ---: | ---: |
| 10 | 21.46 µs | 7.87 µs | 2.7× | 0 B / 0 allocs |
| 1,000 | 1662.64 µs | 61.62 µs | 27.0× | 0 B / 0 allocs |

Raw commands and measurements: [html-incremental-reconciliation-benchmarks.txt](html-incremental-reconciliation-benchmarks.txt).

These are historical measurements for the described paths, not editor frame times. Initialization and warm-up are excluded unless stated otherwise. Allocation guarantees apply only to the explicitly tested warm paths. Editor and screenshot checks are performed manually.
