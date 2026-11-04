# HTML/CSS cache audit

Reviewed on 2026-11-04, covering DOM/CSS parsing, selector matching, full and
incremental style computation, retained layout, and HTML paint/resource ownership.

## Changes implemented

### Stream selector candidates

`html/style/matcher.go` now yields rules directly from the ID, class, tag and
universal buckets. The cascade already compares importance, specificity and
source order in `cascadeWeight`, so collecting and sorting candidates before
comparing those weights was redundant. Both full cascade and input-state updates
use the iterator; `cascadeScratch.candidates` and its cleanup are removed.

For k candidates, enumeration needs O(k) time and O(1) additional storage, instead
of copying O(k) records and sorting them. Declaration application still resolves
shorthands and longhands in its required order. Tests include reordered/repeated
class names, source order, shorthand overrides, inline style and `!important`.
This preserves the precedence described by
[CSS Cascade](https://www.w3.org/TR/css-cascade-5/#cascade-sort).

### Remove duplicate attribute caching and combine sibling indexing

`html/css/selector.go` reads `id` and `class` directly from the DOM attribute map.
The match-context copies added a second hash lookup and retained two string
headers plus loaded flags per node. On amd64, `nodeMatchData` shrinks from 72 to
40 bytes. Attribute-only selectors no longer allocate a node index at all.

Forward `nth-child` indices and previous-element links now share one sibling
scan and one loaded flag. Structural indices remain pass-local: removing them
would turn matching across wide sibling lists into repeated scans. Reset still
invalidates topology-dependent records and releases oversized workspaces.

### Make style interning hash composite contents

`html/style/intern.go` previously hashed background layers, shadows and filters
by their lengths only. Styles with the same comparable fields and same-length,
different composite properties all entered one probe chain. Equality checks kept
results correct, but a batch of n distinct styles could require O(n²) work.

The hash now includes each slice element, preserving element order and the
existing distinction between nil and empty slices. No new cache or larger intern
table is introduced. The document's 2,048-entry bound and immutable style sharing
remain intact. The adverse-case benchmark looks up 1,024 already-interned styles
that differ only in one shadow's blur radius.

### Store paint rasters with owned textures

`ui/html/texture_cache.go` no longer keeps a separate
`map[htmlTextureKey]*htmlPaintRaster`. Background and shadow raster metadata live
on the corresponding `htmlOwnedTexture`, reached through the existing `entries`
index. Texture replacement publishes the new entry; sweeping an old texture
cannot remove the replacement's raster. Disposal and sweep no longer maintain a
second index with the same keys and lifetime.

Existing tests cover shared pixel storage, resize detachment, removed nodes,
animated paints, shadows and paint culling. The texture-to-entry and entity-to-entry
indexes remain because they support reverse lookup and reference ownership.

### Consolidate rendered-box membership

The follow-up removes `htmlLayoutState.BoxesByNode`. Box lookup now uses the
existing `BoxRenderings` record, which already owns the same `*layout.Box`.
Each record gains an emission generation. Full emission hides records that have
not been emitted in the current generation and retires them in the existing
cleanup pass. Local emission preserves untouched records from earlier generations.
There is no second node map to allocate, clear, populate or delete from.

`LayoutCache.nodes` remains separate: it owns the current layout topology, while
the rendering records may still describe the preceding output during preflight.
Regression tests cover partial updates followed by full updates, `display:none`,
`display:contents`, hidden visibility, reparenting and document replacement. The
existing paint-culling and retired-context suites also pass.

### Bound inline-style retention and own parsed source

The global inline-style cache now clones the actual attribute string before
parsing a miss. Keys, declaration substrings and prepared values no longer pin a
larger HTML source allocation or borrow a retained parser's mutable decoding
buffer. Parser reuse previously could change a cached string and corrupt its map
key; the new regression test exercises entity decoding followed by another parse.

The cache retains at most 4,096 entries and has a 4 MiB accounted payload budget.
Accounting includes source bytes, declaration-slice capacity, normalized strings,
prepared values and prepared motion strings. It is accumulated during declaration
parsing, without another declaration traversal. Borrowed strings may be counted
more than once; the budget is not an exact process-heap limit, and bounded map
storage/allocator overhead is additional. A source longer than 64 KiB is parsed
without entering or evicting the cache. Budget eviction clears and reuses the map.

Concurrent misses recheck membership before insertion so they cannot double-count
the same entry. A new cacheable source costs one additional small string copy;
warm hits remain allocation-free. Tests cover backing-string ownership, parser
reuse, byte/count limits, oversized-source bypass and concurrent misses.

## Measurements

Linux amd64, Go 1.27.3, Intel i5-11500. Medians of three runs with
`-benchtime=200ms -benchmem`, comparing the preceding workspace against these
changes. These are package microbenchmarks, not editor frame-rate measurements.

| Benchmark | Before | After | Allocation result |
| --- | ---: | ---: | --- |
| Cascade, 64 matching candidate rules | 12.76 µs | 4.95 µs | 0 B/op, 0 allocs/op |
| Intern lookup batch, 1,024 distinct shadow styles | 253.61 ms | 3.15 ms | 0 B/op, 0 allocs/op |
| Match 5,000 nodes, fresh context | 1.39 ms | 1.04 ms | 1,444,512 → 914,684 B/op; 49 allocs/op |
| Match 5,000 nodes, reused context | 550.44 µs | 424.34 µs | 0 B/op, 0 allocs/op |
| Full cached style computation | 1.25 ms | 1.21 ms | 0 B/op, 0 allocs/op |
| Incremental hover subtree computation | 6.29 µs | 6.36 µs | 0 B/op, 0 allocs/op |
| Create one shadow raster and dispose | 744.90 µs | 732.25 µs | 13 → 11 allocs/op |
| Create 16 shadow rasters and dispose | 11.45 ms | 11.89 ms | 107 → 100 allocs/op |
| Full HTML render with reused stylesheet | 21.84 µs | 21.96 µs | 0 B/op, 0 allocs/op |

The large gains are candidate traversal and avoiding pathological hash collisions.
Raster creation remains dominated by pixel generation; the ownership change
reduces bookkeeping and allocation count, with no demonstrated rendering speedup.
Small timing differences in the other paths should not be treated as significant.
B/op measures allocation volume, not retained heap or GPU memory.

### Follow-up measurements

Same host and three-run median methodology, with the first round of changes as
the new baseline. These measurements do not show an overall rendering speedup;
the follow-up removes duplicate bookkeeping and bounds retained source memory.

| Benchmark | Before | After | Allocations per operation |
| --- | ---: | ---: | ---: |
| Inline-style cache hit | 25.16 ns | 28.18 ns | 0 → 0 |
| Inline declarations parsed directly | 11.01 µs | 11.16 µs | 51 → 51 |
| Full sync, 128 boxes | 1.21 ms | 1.17 ms | 0 → 0 |
| Full sync, 1,024 boxes | 10.15 ms | 10.33 ms | 0 → 0 |
| Structural update beside 1,000 controls | 1.66 ms | 1.65 ms | 10 → 10 |
| Buffered scroll | 892 ns | 918 ns | 0 → 0 |
| Hover cycle | 30.53 µs | 30.70 µs | 0 → 0 |

## Other caches reviewed

| Area | Assessment |
| --- | --- |
| `dom.Parser` node blocks, decoded bytes, attribute/tree scratch | Reusable output/workspace storage with explicit invalidation on the next parse. The parser already builds one DOM directly; a second parse-result cache would add retention. Keep the arena design. |
| `htmlStylesheetKey`, `MutationCache.compiled`, global `compiledStylesheetCache` | Parsed source, a document's reference to prepared rules, and a single-entry convenience cache have different roles. The compiled pointer is shared, not a duplicate copy. The global entry can thrash for alternating uncached callers, but retained documents avoid that path. |
| Prepared declarations and computed-style interning | Parse-once data and shared output values save different work. Keep both; improve hashing rather than enlarging the intern cache. |
| Global inline-style cache | Follow-up complete: cloned source on cacheable misses, a 4 MiB payload budget and oversized-source bypass supplement the 4,096-entry limit. |
| Retained layout tree and flex intrinsic measurements | Current geometry cannot replace intrinsic measurements: final stretch/layout overwrites geometry. Removing the measurement reuse would repeat nested layout. Keep these until the measurement and placement algorithms are redesigned together. |
| Text-shape cache, fragment glyphs and glyph/texture resources | These store token shaping, positioned output and raster data respectively. The shape cache already limits token length, total bytes and entry count, and clones retained token strings. Do not combine them merely because all contain text-related data. |
| `LayoutCache.nodes` and `BoxRenderings` | Follow-up complete: `BoxesByNode` has been removed. Emission generations retain full/local membership semantics; current layout topology still has its own index. |
| Container-query metrics and scratch map | These represent previous and next layout snapshots. Two maps make removed-container detection and convergence comparison explicit. A single map requires a lookup into current layout or generation marking; deleting one without that loses removals. |
| Ancestor-route caches | Reusing parent results prevents O(nodes × depth) work for scattered incremental updates. In a complete traversal, pass inherited state on the stack instead; keep memoization for arbitrary dirty-node entry points. |
| Opacity/group rasters and ordinary paint rasters | Group rasters additionally track subtree composition and versions; their dependency/lifetime rules differ from a single background or shadow. Keep them separate. |

## Follow-up priorities

1. For input-state matching, evaluate marking only the old/new focus ancestor
   paths. Current `:focus-within` recursively inspects descendants and keys its
   node results by the state pointer; alternating previous/current state queries
   can redo work. Compare both deep and wide trees before replacing it, since a
   naive ancestor walk for every node can also be quadratic.
2. When touching container-query convergence, compare metrics during the existing
   layout-tree traversal and investigate using the retained box index to remove
   stale entries. Avoid adding another whole-tree traversal just to save a map.

## Validation

Passed:

```sh
go test ./html/dom ./html/css ./html/style ./html/layout ./ui/html ./ui
go test -race ./html/style -run '^TestInlineStyleCache' -count=1
go test ./html/css ./html/style ./ui/html -run '^$' \
  -bench 'Benchmark(MatchContextStorage|ComputeFullStyles|ComputeHoverStylesIncrementalInPlace|HTMLFullRenderStylesheetReuse|CandidateCascade|HTMLRasterPopulation|StyleInternCompositeValues)$' \
  -benchtime=200ms -count=3 -benchmem
go test ./html/style ./ui/html -run '^$' \
  -bench 'Benchmark(InlineStyleCache|HTMLBoxIndexFullSync|HTMLStructuralRendering|HTMLHoverStyleReuse|HTMLPaintCullBufferedScroll)$' \
  -benchtime=200ms -count=3 -benchmem
```

No editor launch or screenshot acceptance was performed. Chromium pixel/layout
comparison and Android execution were not part of this audit; validation covers
the existing targeted package tests and the added semantic/memory regressions.
