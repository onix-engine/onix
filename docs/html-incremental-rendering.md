# Incremental HTML synchronization: phase 1

Phase 1 (`0f1b6e4`) coalesces changes to each `HTMLNode` and removes descendant roots already covered by an ancestor. Class and style changes recalculate the affected subtree when selector dependencies permit; conditional rules and document-dependent pseudo-classes retain document-wide cascading.

Paint-only properties update indexed entities without layout or reconciliation. Changes to paint structure reuse layout and reconcile generated entities. Text, geometry, topology and other attributes still use full layout in this phase. Replacing HTML reparses the DOM and creates new node identities.

Entities are keyed by DOM node, role, interaction context and fragment slot. Earlier sibling insertion does not change later entity identities. Generic writers construct bundles only for new entities; retained entities update components and remove obsolete optional components and input routes. Text caches, live control values, selection and scroll position survive. `RenderRevision` refreshes authored control properties; select option or geometry changes rebuild the popup while retaining the select.

ID and document-order indexes are maintained together and rebuilt lazily after structural or ID changes. Orphan tracking avoids a full generated-entity scan on stable frames; visibility propagates only when container visibility changes.

Regression coverage includes dirty-root merging, inherited colors, paint membership, wrapping, flex sizing, visibility, routing cleanup, control type changes, selection and immediate ID lookup. Results are compared with a fresh rendering. Linux amd64, Intel Core i5-11500, Go 1.26; median of three runs against `97d4d36`.

The original full-layout boundary is historical: see [phase 2](html-incremental-layout.md). Paint reconciliation still traverses the retained layout tree in phase 1. Zero allocations apply to warmed common color changes, not all CSS parsing or growth.

## Commands

```sh
go test ./ecs/... ./internal/css ./internal/dom ./internal/style ./internal/layout ./project
go vet ./ecs/... ./internal/style ./internal/layout ./internal/dom ./internal/css
go test ./ecs -run '^$' -bench '^BenchmarkHTMLLocalClassMutation$' -benchmem -benchtime=100ms -count=3
```

## Recorded results

| Column 1 | Column 2 | Column 3 | Column 4 | Column 5 |
| --- | ---: | ---: | ---: | ---: |
| 10 | 78.7 µs | 4.73 µs | 285 | 0 |
| 1,000 | 8.27 ms | 4.89 µs | 26,817 | 0 |
| 10,000 | 104.0 ms | 4.48 µs | 269,876 | 0 |

These are historical measurements for the described paths, not editor frame times. Initialization and warm-up are excluded unless stated otherwise. Allocation guarantees apply only to the explicitly tested warm paths. Editor and screenshot checks are performed manually.
