# Internal and project traversal review

CSS same-type sibling positions use one pass per direction across all tags with a scalar single-tag fast path, replacing O(S×T) rescans. Stylesheet compilation indexes keyframes by name and conservative property metadata; track selection checks same-name candidates in reverse order, preserving last matching conditional definitions and important precedence. Conditions precompile and root size resolves once per call.

Grid track offsets/totals compute once and item spans use endpoints: O(I×(C+R)) becomes O(C+R+I). Dirty layout propagation stops at already-marked ancestors, O(D+B), with parent/mark reset on reattachment. Model parent validation uses visiting/validated states, O(N), retaining invalid-parent, cycle and finite-transform checks. glTF builds a direct-parent index once, O(N+E+J), preserving joint order, non-joint parents and skin offsets.

FBX connection passes first identify node targets then select each curve's first valid target in original order, O(E+O+K), preserving last node/model association. Per-import world-matrix caches eliminate repeated ancestor arrays/multiplication while retaining cycles, parent×local order and parent relationships independent from reference traversal.

Scene cloning snapshots parent→children once per operation, O(E+Σ d log d), then visits source nodes only; sorting remains deterministic. A shallow Widget-copy bug is fixed by copying values before rewriting clone TargetID, so source templates and repeated clones stay independent. Batch deletion relies on existing deduplication rather than redundant ancestor filtering, while final reverse deletion sorting preserves slot reuse. Counts scan Parent without building/sorting lists. Read-only Parent access does not mark change.

Go 1.26/Linux amd64/i5-11500 comparisons use ten production files from `ae4153f0b81a4ccf23bdac9923dde60366054e84`, preserving earlier ECS/UI changes, three sequential 200ms -cpu=1 runs. Distinct-tag CSS stresses the former worst case; animation measures actual cascade excluding track compilation. Grid/dirty use retained layout, with DOM-built deep trees. Model validation uses minimal geometry; FBX hierarchy cases rebuild importer/cache per run, animation curves have two keys, glTF pre-reserves output and scene cloning includes creation/deletion.

Warm CSS/Grid/dirty cases remain zero-allocation. Animation stays 27 allocations/about 218 KB. Model validation uses stack state up to 128 nodes; 1,000 adds 1024 B/1 allocation. glTF adds 8192 B for parent indexing (36952 B/6→45144 B/7). FBX curves retain 2051 allocations, bytes 627808→693408; hierarchy drops 41022992 B/26594→732920 B/7041. Clone/delete drops 2084953 B/8108→2073153 B/7103. These indexes trade cold memory for traversal work.

Targeted default tests passed. Release CSS/style/layout and generic prepared project regressions passed, while historical source-JSON/preprocessing tests still reject input under release; baseline overlays reproduced that limitation. Necessary selector backtracking, layout preflight, occupancy/flex algorithms, same-name candidates, compression algorithms, stable canonical sorting and actual model data traversal remain. No claim makes entire scene operations strictly linear. No full repository/editor/screenshot tests ran.

## Examples and commands

```sh
go test ./internal/css ./internal/style ./internal/layout ./project ./scene ./ui ./ui/html ./ui/markdown -p=1 -count=1
```

```sh
go test -tags=onix_release ./project -p=1 -count=1 -run 'Test(ModelHierarchyValidationChecksSharedAncestorsAndCycles|CloneHierarchySnapshotAndWidgetTargetsStayIndependent|RemoveNodesCollectsOverlappingRootsAndCyclesOnce|ReleaseRuntime|SceneCatalogRequiresPreparedSpawnPlanInRelease|PrefabResourcesRequirePreparedSpawnPlanInRelease)'
```

## Recorded measurements

| 1,000-item workload | Before | After | Speedup |
| --- | ---: | ---: | ---: |
| Distinct-tag sibling numbering, 1,000 nodes | 3065.07 μs | 69.17 μs | 44.31× |
| Cascade 1,000 animation names/keyframes | 3788.35 μs | 453.82 μs | 8.35× |
| Retained Grid, 1,000 columns/items | 1344.64 μs | 866.67 μs | 1.55× |
| Dirty propagation on 1,000-node chain | 2696.11 μs | 485.17 μs | 5.56× |
| Validate 1,000 model parents | 1390.12 μs | 50.35 μs | 27.61× |
| Resolve 1,000 glTF joint parents | 2051.78 μs | 83.23 μs | 24.65× |
| Import 1,000 FBX animation curves | 11297.25 μs | 978.45 μs | 11.55× |
| FBX 1,000-node hierarchy/world matrices | 70885.69 μs | 1004.86 μs | 70.54× |
| Clone/delete 1,000 scene nodes | 8644.36 μs | 2854.25 μs | 3.03× |

Performance figures describe the recorded workloads and environments, not complete application frame rates. Warm allocation guarantees exclude first use and capacity growth.
