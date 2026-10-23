# Local styles for structural HTML mutations: phase 7

Insertion, removal, clearing and structural `SetText` now invalidate the parent whose child list changed and its current descendants. This covers `:empty`, positional pseudo-classes and inheritance. Removed branches do not cascade; overlapping roots merge and disjoint roots remain separate.

Structural invalidation is separate from SVG filter invalidation. Ordinary changes preserve filter caches; inserting/removing a filter subtree or modifying filter internals still invalidates definitions and document-wide styles. Ownership is checked before detaching nodes so references elsewhere remain correct. `Clear` and structural `SetText` combine resource detection, detachment and retained-reference cleanup in one child traversal.

Conditional rules and other document-dependent selectors retain `NeedsDocumentStyle` fallback. Ordinary attributes still recascade globally. Initial parsing and structural updates share `syncHTMLDocumentIndex`, computing IDs and order together while reusing the map. Stripped metadata no longer leaves initial order gaps.

This phase optimizes cascading only: layout topology, DOM ordering, deleted-style cleanup and ECS reconciliation still use complete paths because insertion changes later paint and focus order. Tests compare fresh rendering for 1,000 unrelated branches, pseudo-classes, inheritance, merged roots, filter references and reattachment. Warm detach/reattach invalidation alone has a zero-allocation assertion, excluding `SyncHTML`.

Benchmarks compare phase 6 `a5dfc73`, three 500ms runs on Linux amd64/i5-11500/Go 1.26. A span alternates removal and attachment. `styled/last-op` is 1 or 3 nodes, versus 22/24 or 2,002/2,004 in the baseline. Allocation ranges reflect all three samples and whether the final operation attached or removed. Structural synchronization still allocates. See [phase 8](html-incremental-structure-rendering.md).

## Commands

```sh
go test ./ecs/... ./html/layout ./html/style ./html/dom ./html/css ./project
go vet ./ecs/... ./html/layout ./html/style ./html/dom ./html/css
git diff --check
```

## Recorded results

| Unrelated divs | Phase 6 | Phase 7 | Speedup | Phase 7Allocations |
| --- | ---: | ---: | ---: | ---: |
| 10 | 57.78 µs | 26.21 µs | 2.2× | 6,352–6,353 B / 11 allocs |
| 1,000 | 5.32 ms | 2.17 ms | 2.5× | 6,329–6,352 B / 10–11 allocs |

Raw commands and measurements: [html-incremental-structure-styles-benchmarks.txt](html-incremental-structure-styles-benchmarks.txt).

These are historical measurements for the described paths, not editor frame times. Initialization and warm-up are excluded unless stated otherwise. Allocation guarantees apply only to the explicitly tested warm paths. Editor and screenshot checks are performed manually.
