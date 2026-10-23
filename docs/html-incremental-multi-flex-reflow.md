# Multiple local flex reflows per frame: phase 20

Independent flex containers reordered in one frame can each reflow from cached inputs without visiting unrelated siblings. Nested reordered containers merge into the outermost changed root.

Per-pass marks distinguish actual reordering from inspection and merge roots through ancestor chains. A reusable root list and coverage marks avoid pairwise root comparisons. All roots first pass cache, positioning and ancestor checks, and every dirty layout node must belong to an eligible root.

Ancestor dirtiness clears only after every root retains its outer geometry. Any size change or error preserves ancestor invalidation and returns to normal layout, including repositioning previously attempted roots when needed. Shared rendering ancestors register once. Temporary root references clear after sync while capacity remains; owner spans handle final ordering.

Targets require static positioning, valid caches and ordinary static block ancestors. Unsupported dependencies retain existing layout semantics and fallback. Tests with two independent flex containers and 1,000 unrelated inputs assert `LocalFlexReflows == 2`, at most 20 layout visits and four updated inputs. Nested roots merge into one attempt. Auto-height fallback is checked under both document orders against full rendering, and warmed multi-root round trips assert zero allocation.

Benchmarks compare `28e1de7`, three 500ms runs on Linux amd64/i5-11500. Each operation changes two style attributes and calls `SyncHTML` once. Large-scene time falls about 56.1%; small-scene time is essentially unchanged. Every sample in both versions is 0 B/op and 0 allocs/op. Outside dirtiness, positioned/flex dependencies and outer geometry changes retain fallback; failed attempts and ancestor focus/overflow dependencies can add work, and first list growth allocates.

## Commands

```sh
go test ./ecs/... ./internal/layout ./internal/style ./internal/dom ./internal/css ./project
go vet ./ecs/... ./internal/layout ./internal/style ./internal/dom ./internal/css
git diff --check
```

```sh
go test ./ecs -run '^$' -bench '^BenchmarkHTMLMultiOrder$' -benchmem -count=3 -benchtime=500ms
```

## Recorded results

| Unrelated inputs | Phase 19 | Phase 20 | Speedup |
| ---: | ---: | ---: | ---: |
| 10 | 52.01 μs | 51.87 μs | 1.00× |
| 1,000 | 125.00 μs | 54.82 μs | 2.28× |

Raw commands and measurements: [html-incremental-multi-flex-reflow-benchmarks.txt](html-incremental-multi-flex-reflow-benchmarks.txt).

These are historical measurements for the described paths, not editor frame times. Initialization and warm-up are excluded unless stated otherwise. Allocation guarantees apply only to the explicitly tested warm paths. Editor and screenshot checks are performed manually.
