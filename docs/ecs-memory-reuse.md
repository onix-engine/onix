# ECS temporary-memory reuse

Recorded 2026-10-17 against `a11936948c69f23b71af3e0047db542008af8951`. Spawn keeps stack arrays for at most eight components; larger bundles and dynamic-constructor scratch borrow independent generic slice pools. Each active/nested call owns its scratch. Success/panic clears references and returns wrapper pointers, avoiding slice boxing. Arrays retain at most 1024 elements.

Query iterators pool only overflow tick bindings; up to eight change conditions stay on stack. Nested iterators borrow independently and return on finish/break/return/panic. Views still own filter snapshots and World plans own their data. Constructing a new view above eight change conditions may allocate; benchmarks reuse saved views.

Scheduler recompilation shares a name index, contiguous nodes/edges and min-heap instead of per-source adjacency allocations. Schedule retains results and clears removed-system tails. Lexical order, duplicates, Before/After, missing dependency and cycle semantics remain. Temporary graph retention caps at 4096 names/nodes/ready entries and 16384 edges, clearing names on return.

Three reference-retention fixes clear failed CommandSpawn entries before truncating, track FrameScratch's uncleared high-water length across skipped ReleaseUnused calls and clear old compiled-system tails. All three regressions fail on the original implementation. User-owned columns/entities/bundles/messages/views/scratch retain their own lifetimes. sync.Pool can be cleared by GC; first use, growth and oversized workloads may allocate.

Go 1.26/Linux amd64/i5-11500 uses CPU 0, six alternating 200ms runs. SpawnScratch includes spawn/despawn after warm registration/storage, excluding repeated bundle-description construction. Dynamic remaining allocations are componentValue boxing; only scratch arrays changed. Scheduler cases force dirty recompilation rather than normal clean execution.

Static 16-component scratch falls 384→0 bytes; dynamic 896 B/18→256 B/16 allocations; reused 32-condition iteration 1408 B/1→0; 1,000-system dependency compilation approximately 197 KiB/1011→0, with median time down 61.8%. Small one/eight-condition iterations rise about 5.6/8.9ns; controls vary approximately ±4%. No universal speedup or new-view zero-allocation claim is made.

ECS/race/vet/root/UI checks and Android 12 x86_64 ECS tests passed. Tests cover reference clearing, caps, GC, nesting/panic, snapshots, early exit, scheduler recovery and concurrent independent owners. Historical broader checks reproduced two missing-font scene failures and an existing physics precision assertion under the baseline overlay. The application fixture dependency was subsequently removed. No full-repository/editor/screenshot tests were run.

## Examples and commands

```sh
go test ./ecs -run '^$' \
  -bench 'Benchmark(SpawnScratch|QueryBindingScratch|ScheduleDependencyScratch|ScheduleCompileTraversal|CommandSpawnPreparedRecycled|CommandInsertRecycled|FilteredQueryTenThousand|Read2TenThousand|Query2TenThousand|MessageReaderAllOneHundred)$' \
  -benchmem -benchtime=200ms -count=6 -cpu=1
```

## Recorded measurements

| Benchmark | ns/op, before → after | Time change | B/op, before → after | allocs/op, before → after |
| --- | ---: | ---: | ---: | ---: |
| `CommandSpawnPreparedRecycled` | 101.90 → 104.75 | +2.8% | 0 → 0 | 0 → 0 |
| `SpawnScratch/components=2/dynamic=false/prepared=false` | 125.20 → 122.30 | -2.3% | 0 → 0 | 0 → 0 |
| `SpawnScratch/components=2/dynamic=false/prepared=true` | 74.63 → 74.89 | +0.3% | 0 → 0 | 0 → 0 |
| `SpawnScratch/components=2/dynamic=true/prepared=false` | 190.35 → 187.90 | -1.3% | 32 → 32 | 2 → 2 |
| `SpawnScratch/components=2/dynamic=true/prepared=true` | 135.50 → 140.55 | +3.7% | 32 → 32 | 2 → 2 |
| `SpawnScratch/components=8/dynamic=false/prepared=false` | 421.05 → 403.95 | -4.1% | 0 → 0 | 0 → 0 |
| `SpawnScratch/components=8/dynamic=false/prepared=true` | 226.40 → 221.35 | -2.2% | 0 → 0 | 0 → 0 |
| `SpawnScratch/components=8/dynamic=true/prepared=false` | 701.70 → 719.35 | +2.5% | 128 → 128 | 8 → 8 |
| `SpawnScratch/components=8/dynamic=true/prepared=true` | 446.85 → 450.20 | +0.7% | 128 → 128 | 8 → 8 |
| `SpawnScratch/components=16/dynamic=false/prepared=false` | 1026.00 → 870.25 | -15.2% | 384 → 0 | 1 → 0 |
| `SpawnScratch/components=16/dynamic=false/prepared=true` | 415.35 → 427.45 | +2.9% | 0 → 0 | 0 → 0 |
| `SpawnScratch/components=16/dynamic=true/prepared=false` | 1691.00 → 1572.00 | -7.0% | 896 → 256 | 18 → 16 |
| `SpawnScratch/components=16/dynamic=true/prepared=true` | 1010.50 → 972.70 | -3.7% | 512 → 256 | 17 → 16 |
| `QueryBindingScratch/1` | 124.70 → 130.30 | +4.5% | 0 → 0 | 0 → 0 |
| `QueryBindingScratch/8` | 189.80 → 198.70 | +4.7% | 0 → 0 | 0 → 0 |
| `QueryBindingScratch/9` | 277.05 → 234.40 | -15.4% | 64 → 0 | 1 → 0 |
| `QueryBindingScratch/16` | 448.25 → 321.40 | -28.3% | 448 → 0 | 1 → 0 |
| `QueryBindingScratch/32` | 852.50 → 508.85 | -40.3% | 1408 → 0 | 1 → 0 |
| `ScheduleDependencyScratch/100` | 27898.00 → 11755.50 | -57.9% | 14080 → 0 | 107 → 0 |
| `ScheduleDependencyScratch/1000` | 372849.50 → 142419.00 | -61.8% | 201744 → 0 | 1011 → 0 |
| `CommandInsertRecycled` | 131.10 → 129.45 | -1.3% | 0 → 0 | 0 → 0 |
| `ScheduleCompileTraversal/100` | 21663.00 → 16969.50 | -21.7% | 12496 → 0 | 8 → 0 |
| `ScheduleCompileTraversal/1000` | 320130.00 → 230158.00 | -28.1% | 185760 → 0 | 12 → 0 |
| `FilteredQueryTenThousand/With` | 44669.00 → 43834.50 | -1.9% | 0 → 0 | 0 → 0 |
| `FilteredQueryTenThousand/Changed` | 72406.50 → 70719.00 | -2.3% | 0 → 0 | 0 → 0 |
| `Query2TenThousand` | 109132.50 → 104743.00 | -4.0% | 0 → 0 | 0 → 0 |
| `Read2TenThousand` | 56226.50 → 57032.50 | +1.4% | 0 → 0 | 0 → 0 |
| `MessageReaderAllOneHundred` | 2041.50 → 2075.00 | +1.6% | 0 → 0 | 0 → 0 |

Raw records: [ecs-memory-reuse-after.txt](ecs-memory-reuse-after.txt), [ecs-memory-reuse-android.txt](ecs-memory-reuse-android.txt), [ecs-memory-reuse-before.txt](ecs-memory-reuse-before.txt).

Performance figures describe the recorded workloads and environments, not complete application frame rates. Warm allocation guarantees exclude first use and capacity growth.
