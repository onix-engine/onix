# Retained HTML layout: phase 2

Phase 2 (`736886c`) retains the box tree and uses changed nodes and containing-block inputs to choose reflow scope. DOM identity and ECS reuse from phase 1 remain in effect. See [phase 3](html-incremental-reconciliation.md) for local primitive reconciliation.

Text changes invalidate layout separately and usually recascade only their subtree. Geometry dirtiness propagates through the nearest box to ancestors. Clean subtrees with unchanged containing and absolute-positioning inputs are skipped. Static block subtrees that only move translate existing boxes and fragments without rebuilding glyph buffers; translation still visits descendants.

Topology, display, order and position changes reconcile parentage and ordering by DOM identity, releasing removed references. Affected flex formatting contexts are remeasured together so temporary measurements never become final caches. Font, viewport and resource changes force reflow and refresh topology and image-size caches. Layout errors invalidate all geometry before retry.

Text, image and control fragments, common glyph buffers and line scratch arrays reuse capacity and clear unused reference tails. Unchanged stylesheet cache keys are retained. The first layout of a retained box clears previously used dimensions, while repeated flex measurements in the same pass preserve intermediate results; bottom-positioned absolute children cannot read stale heights.

Independent `BuildWithFonts` comparisons cover geometry, text, structure, visibility, moves, positioning, flex wrapping, percentages, controls and 100 deterministic mixed mutations. Tests assert identity preservation, subtree skipping and translation reuse. Warm static resizing, unchanged layout and common small text changes have zero-allocation assertions.

Recorded Linux amd64/i5-11500/Go 1.26 medians use three 200ms runs. One large-document phase-2 sample reported 22 B/op and rounded to 0 allocs/op; the table does not establish universal zero allocation. Ancestors still inspect direct children, structural updates remain global, and ECS reconciliation still traverses the tree. Text shaping, growing fragments and controls may allocate. Existing engine semantics are preserved; this is not a claim of complete Chromium layout support.

## Commands

```sh
go test ./ecs/... ./html/layout ./html/style ./html/dom ./html/css ./project
go vet ./ecs/... ./html/layout ./html/style ./html/dom ./html/css
go test ./ecs -run TestHTMLWarmTextMutationAllocations -count=5
```

## Recorded results

| Unrelated divs | Full layout | Incremental layout | Full layoutAllocations | Incremental layoutAllocations |
| --- | ---: | ---: | ---: | ---: |
| 10 | 22.75 µs | 1.05 µs | 178 | 0 |
| 1,000 | 2.44 ms | 53.34 µs | 16,919 | 0 |

| Unrelated divs | Phase 1 | Phase 2 | Phase 1Allocations | Phase 2Allocations |
| --- | ---: | ---: | ---: | ---: |
| 10 | 68.19 µs | 21.32 µs | 137 | 0 |
| 1,000 | 6.60 ms | 1.54 ms | 12,918 | 0 |

Raw commands and measurements: [html-incremental-layout-benchmarks.txt](html-incremental-layout-benchmarks.txt).

These are historical measurements for the described paths, not editor frame times. Initialization and warm-up are excluded unless stated otherwise. Allocation guarantees apply only to the explicitly tested warm paths. Editor and screenshot checks are performed manually.
