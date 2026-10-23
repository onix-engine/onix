# Owner-list spans for CSS order: phase 18

When CSS order actually changes and entity membership remains stable, only the affected flex subtree's owner-list span is rewritten. Nested roots merge into the outermost root; independent roots update separate spans.

Full rendering, structural updates and membership rebuilding share box preorder: own primitives, clip entity, then descendants. Existing traversals record `OwnerStart`/`OwnerEnd`, including valid empty spans. Full rendering establishes order during clip construction without another full indexing pass.

Stable per-box primitive counts allow subtree order to change without shifting unrelated spans. Rewriting preserves backing storage and is bounded by the old span; a size mismatch falls back to complete rebuilding before unrelated data can be overwritten. Reorder marks merge nested roots and clear after synchronization. Membership changes, shared ownership or invalid spans retain full rebuilding. Ordinary paint changes with neither reordering nor membership changes do not add rebuilding.

`OwnerListBoxes` verifies traversal scope. Tests with 1,000 unrelated clipped controls visit only the target subtree and preserve prefix/suffix entity order. Regressions cover first/repeated reorder, clips, nested/independent roots, membership recovery, style fallback, deletion-shifted spans and backing-array reuse. Existing unclipped warm order checks stay allocation-free. Clipped round trips allocate four times in both phase 17 and phase 18; no full-pipeline zero-allocation claim is made for them.

Benchmarks compare `d3f2dd2`, three 500ms runs on Linux amd64/i5-11500. Large-scene time falls about 32%; small-scene median is slightly higher. Baseline 0 B/op, current 0–1 B/op. Structural/membership changes remain global, shared primitives bypass spans, and ancestor layout still checks surrounding containers. See [phase 19](html-incremental-local-flex-reflow.md).

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

| Unrelated inputs | Phase 17 | Phase 18 | allocs/op |
| ---: | ---: | ---: | ---: |
| 10 | 29.30 μs | 30.15 μs | 0 → 0 |
| 1,000 | 143.90 μs | 98.34 μs | 0 → 0 |

Raw commands and measurements: [html-incremental-owner-spans-benchmarks.txt](html-incremental-owner-spans-benchmarks.txt).

These are historical measurements for the described paths, not editor frame times. Initialization and warm-up are excluded unless stated otherwise. Allocation guarantees apply only to the explicitly tested warm paths. Editor and screenshot checks are performed manually.
