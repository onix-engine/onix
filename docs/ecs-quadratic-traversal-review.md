# ECS quadratic traversal review

Scheduler ready-node selection uses a generic min-heap, preserving lexical priority even for newly unlocked names: O(S²+D) becomes O(S log S+D). Small bundles up to 32 items retain inline insertion sort; large reversed inputs use existing in-place heap sort, O(C log C), allocation-free after reservation.

maskWith reuses capacity and grows geometrically rather than exact-copying each word: O(T+W²) becomes amortized O(T+W). Re-enabled hidden capacity clears stale bits. Removal masks record the highest ID and OR bits directly, avoiding sorting/deduplication while preserving registration order. Patch plans retain sorted write IDs and use forward cursors for component merging and replacement binding, O(C+K), preserving caller write order, write-over-remove priority, hooks and ticks. Mask equality compares common prefixes and longer zero tails in one pass, preserving empty/zero-tail equality.

S counts systems, D dependency edges, C source components, K patch writes, T type descriptors and W mask words; hash lookup is expected constant time. Patch plans retain O(K) IDs; first migrations and mask growth still allocate.

Go 1.26/Linux amd64/i5-11500 overlay comparisons use three sequential 200ms -cpu=1 runs. Bundle/type tests use synthetic IDs to isolate preparation. Mask growth spans 1,000 separate 64-bit words, not 1,000 adjacent types/entities. Old generic sorting already optimized reverse input, so type-mask measurements do not represent its general worst case.

Cold mask growth falls from 4,273,912 B/1000 allocations to 25,208 B/12 allocations; retained-capacity bit setting is allocation-free. Scheduler compilation remains cold with 12 allocations for 1,000 systems. Reserved bundles and existing query/migration/patch paths remain warm allocation-free. First patch migration and mask equality have no separate timing.

Tests cover ordering, duplicate edges, recompilation, missing/cyclic dependencies, sort thresholds, duplicate components, hidden bits, zero tails, patch semantics and swapped entities. Nested archetype-row iteration visits actual rows; dependency loops visit actual edges; collision checks, message buffers, rollback and per-column cleanup remain necessary. Targeted ECS/scene/UI/HTML/markdown checks passed without full-repository/editor screenshot testing.

## Examples and commands

```sh
go test ./ecs -run '^$' -bench 'Benchmark(ScheduleCompile|BundlePreparation|MaskGrowth|ComponentTypeMask)Traversal' -cpu=1 -benchtime=200ms -count=3 -benchmem
```

## Recorded measurements

| Workload | Before | After | Speedup |
| --- | ---: | ---: | ---: |
| Compile 1,000 independent ready systems | 1989.46 μs | 282.22 μs | 7.05× |
| Prepare 1,000 reversed bundle entries | 450.02 μs | 77.08 μs | 5.84× |
| Grow empty mask to 1,000 words | 413.30 μs | 7.51 μs | 55.07× |
| Build mask from 1,000 reversed type descriptors | 5.18 μs | 3.50 μs | 1.48× |
| Prepare eight reversed bundle entries | 58.37 ns | 57.84 ns | 1.01× |

Performance figures describe the recorded workloads and environments, not complete application frame rates. Warm allocation guarantees exclude first use and capacity growth.
