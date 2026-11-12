# Tree storage optimization

UI layout now reads parent/child links and cached depths from `scene.Hierarchy`.
It retains the layout containers in parent-first order, rather than a second
parent/child index, an input copy of that index, a depth map and a separate
ancestor path. The ordered children already stored in each container's size
snapshots are reused until hierarchy, sibling order or query scope changes.
Only the largest container needs temporary child sorting storage. Component
migrations with unchanged values do not invalidate topology, and sibling order
changes do not require rebuilding container depths.

Hierarchy depth and inherited flags share the existing invalidation walk and
ancestor workspace. Packing cache state into flags keeps each hierarchy node at
48 B, including the new depth. Depth lookup includes ancestors outside the
current query scope, detects cycles and rejects stale entity generations.
Temporary UI arrays participate in delayed peak reclamation.

Keyed element VNodes now use one pending buffer on the keyed list. Reconciliation
matches that input against the retained DOM and key index; it does not need an
old VNode description. Committing clears all references in the pending buffer
while retaining capacity. Scalar bindings and child component snapshots retain
their existing reconciliation semantics. Patch renders preserve untouched
slots; ordinary renders remove omitted slots.

## Memory measurements

Measured on Linux amd64 against the working tree immediately before this change:

| Measurement | Before | After |
| --- | ---: | ---: |
| UI topology struct and slice capacities, 10,000 layout nodes in chains of 16 | 795,000 B | 180,576 B |
| Pending/previous VNode buffer capacities, one list of 1,000 items | 180,224 B | 90,112 B |
| Shared hierarchy node size | 48 B | 48 B |

The UI figure excludes map buckets, shared hierarchy storage and per-container
size snapshots. The previous depth map is also removed, but its heap footprint
is not included in the comparison. The VNode figure counts buffer backing
storage only, excluding DOM, attributes and nested child arrays. Committed
buffers no longer retain those VNode references. These figures are not total
application memory measurements.

Stable hierarchy refresh, forced hierarchy rebuild, and warmed keyed component
commits remain allocation-free. Timing varies with machine load; no fixed
keyed-commit speedup is claimed. Reordering children still requires sorting
that container's children when its ordered view is refreshed.

## Verification

Regressions cover scope filtering, sibling order ties, ancestors without Layout,
disabled layouts returning after hierarchy changes, reparenting, entity slot
reuse, cycles and repair, peak reclamation, omitted keyed slots, patch/full
render transitions, caller-owned VNode storage and immediate reference release.

```sh
GOMAXPROCS=2 go test -p 1 ./scene ./module ./ui ./ui/html ./editor ./project -count=1 -timeout=180s
GOMAXPROCS=2 go test -p 1 ./ui ./ui/html -run '^TestUILayoutTopologyStorage$' -bench '^(BenchmarkUILayoutHierarchy|BenchmarkUILayoutHierarchyMaps|BenchmarkHTMLComponentKeyedCommit)$' -benchmem -benchtime=300ms -count=3 -v
GOMAXPROCS=2 fieldalignment ./scene ./ui ./ui/html
```

All six packages passed their full tests. Targeted race checks passed for
hierarchy depth/cycles, scoped layout, cache reclamation, keyed reference
release and patch/full transitions. Fieldalignment reported no diagnostics.
Android x86_64 device regressions passed for scene hierarchy, UI layout and HTML
components, using NDK 27.2.12479018 and `-trimpath`. Test fonts were copied to a
temporary device directory; the test binaries and directory were removed.

The editor is not launched for validation. HTML paint-index consolidation and
text-tree unification remain separate changes.

## Numeric layout identities and indexed bounds (batch 2)

Layout boxes now use a nonzero `uint64` ID assigned only when a box is created.
Retained IDs survive sibling insertion, reordering, reparenting and movement of
an ancestor. Retired IDs are not reused, including across document replacement
within the same cache. A standalone build assigns deterministic IDs in creation
order. IDs are local to a builder/cache, not global document identifiers.
`Box.StableID` consequently changes from `string` to `uint64`.

The cached parent path is removed. Parent pointers, source-order indices and the
renderer's existing structure traversal already track topology. Geometry hashes
now describe geometry independently of identity and ancestry, so a retained
tree can match a fresh tree despite having a different allocation history.
Same-geometry moves still refresh render ancestry, clipping, ownership and
stacking order. Standalone tree construction sets child parents and propagates
float flags while appending children, combining the previous two child passes.

`Node.Bounds()` uses the existing layout cache's node-to-box map through
`Cache.BoxForNode`, and reads the render state by reference. This removes the
recursive layout-tree search and a render-state value copy without adding an
index. Display, removal, document replacement and offscreen paint culling keep
their existing layout membership semantics. Paint emission membership continues
to use its own generation-aware records.

Measured on Linux amd64, Go 1.27.3, against a saved copy of the working tree
immediately before batch 2:

| Storage measurement | Before | After |
| --- | ---: | ---: |
| Raw `Box` size, including its retained cache | 760 B | 736 B |
| Raw embedded retained cache size | 280 B | 264 B |
| Retained path string contents, 1,026 boxes, depth-1 fixture | 7,086 B | 0 B |
| Retained path string contents, 1,026 boxes, depth-8 fixture | 13,460 B | 0 B |

The embedded cache size is already included in `Box`; do not add these rows.
The cache gains one 8 B ID counter. Raw structure sizes are not allocator heap
measurements: both old and new boxes occupy the same Go allocation size class.
Removing paths eliminates their string backing allocations as well as the
pointer-bearing string fields. String-content counts exclude allocation rounding.

The following benchmarks use 1,024 `div` elements arranged in depth-1 or depth-8
subtrees, plus the document/container boxes. Rotation moves the first subtree
to the end after warming the cache. Tree construction excludes style computation
and geometry layout.

| Operation | Before B/op | After B/op | Before allocs/op | After allocs/op |
| --- | ---: | ---: | ---: | ---: |
| Retained topology rotation, depth 1 | 15,552 | 0 | 1,948 | 0 |
| Retained topology rotation, depth 8 | 16,448 | 0 | 1,052 | 0 |
| Standalone box-tree construction, depth 1 | 813,000 | 797,448 | 2,977 | 1,028 |
| Standalone box-tree construction, depth 8 | 812,761 | 796,296 | 2,977 | 1,924 |

Bounds benchmarks query the last sibling. Median times from three 300 ms runs:

| Sibling count | Before | After |
| --- | ---: | ---: |
| 128 | 596 ns | 57 ns |
| 1,024 | 4.58 µs | 52 ns |
| 8,192 | 65.3 µs | 55 ns |

Bounds lookup remains at 0 B/op and 0 allocs/op. Host load affects timing; these
are local benchmark results, not application frame-time estimates. No fixed
standalone tree-build speedup is claimed.

Batch-2 regressions cover stable/unique IDs, insertion, reordering, subtree and
ancestor movement, retirement and document replacement, fresh/retained geometry
equivalence, warm allocation-free topology rotation, boxless nodes, indexed
bounds before/after synchronization, and same-geometry render-order/clip changes.

```sh
GOMAXPROCS=2 go test -p 1 ./html/layout ./ui/html ./ui ./editor ./project -count=1 -timeout=180s
GOMAXPROCS=2 go test -p 1 ./html/layout ./ui/html -run '^$' -bench '^(BenchmarkLayoutTopologyRotation|BenchmarkBuildBoxTree|BenchmarkHTMLNodeBounds)$' -benchmem -benchtime=300ms -count=3
GOMAXPROCS=2 fieldalignment ./html/layout ./ui/html ./ui
```

All five packages passed their complete tests. Targeted race checks passed for
identity changes, index retirement, bounds, same-geometry moves and existing box
membership handling. Fieldalignment and diff whitespace checks were clean.
Android x86_64 device regressions passed for numeric IDs, fresh/retained layout,
indexed bounds, reparenting, clipping, focus/input context, owner ordering and
offscreen bounds. The test binaries and fonts were removed from the device.
