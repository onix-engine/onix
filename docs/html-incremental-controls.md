# Local HTML controls, focus and edit hosts: phase 5

With stable topology and interaction properties, native controls, focusable nodes and edit hosts can reconcile geometry, style and text locally. An independent input resize no longer snapshots and restores all document controls.

Boxes retain inherited control/focus entities, control rectangles and their context owner. The queue includes required owners and descendants, then reconciles parent before child. Only queued live controls contribute values, dirty flags, open state and selections to snapshots; only their regenerated controls are restored and synchronized to DOM live values. Local retirement clears matching snapshots while the full path retains global cleanup.

Focus bounds carry the current generation, reset on first emission and merge only current fragments. Removed targets leave the node index with swap-index repair; DOM order still determines navigation. All control/focus primitives and bounds are generated before clip-coordinate conversion. A zero-height child's focus entity may later transfer back to its parent; cleanup recognizes its current owner and preserves routes. Only updated controls advance `HTMLControl.RenderRevision`; unchanged edit values retain selection.

Regression coverage includes input, textarea, checkbox, radio, select, nested buttons, focusable blocks, links and edit hosts; shrinking focus bounds, ownership transfers, dirty textarea values, selections and unrelated revisions. Fresh rendering and headless TextField tests check geometry and input/focus/edit routes. With 1,000 unrelated controls, one resize snapshots and updates one control. The warmed ordinary input resize asserts zero allocations.

Benchmarks compare phase 4 `1842f0a`, three 200ms runs on Linux amd64/i5-11500/Go 1.26. Historical full fallbacks remain for topology, interaction/content attributes, environment, clipping, z-index and shared primitive ownership. Global pseudo-class invalidation retains its existing restoration path. Shared inline contexts may require whole-subtree reconciliation. First use, text shaping and capacity growth may allocate.

## Commands

```sh
go test ./ecs/... ./html/layout ./html/style ./html/dom ./html/css ./project
go vet ./ecs/... ./html/layout ./html/style ./html/dom ./html/css
```

## Recorded results

| Unrelated inputs | Phase 4 | Phase 5 | Speedup | Phase 5Allocations |
| --- | ---: | ---: | ---: | ---: |
| 10 | 76.93 µs | 12.73 µs | 6.0× | 0 B / 0 allocs |
| 1,000 | 7.64 ms | 79.61 µs | 96.0× | 0 B / 0 allocs |

Raw commands and measurements: [html-incremental-controls-benchmarks.txt](html-incremental-controls-benchmarks.txt).

These are historical measurements for the described paths, not editor frame times. Initialization and warm-up are excluded unless stated otherwise. Allocation guarantees apply only to the explicitly tested warm paths. Editor and screenshot checks are performed manually.
