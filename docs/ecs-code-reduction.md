# ECS code reduction

Recorded 2026-10-18. Production ECS code decreases by 72 lines and generators by one without changing public APIs. Query forwarding layers are removed at the generator and regenerated. Component append/move/swap-remove share storage/tick/version handling. Scheduler configuration and context share one table; topological sorting directly emits execution order, eliminating separate context/name caches and a traversal. FrameValue, ScopeValue and Local share generic cache logic while preserving their owning lifetimes. MessageReader.Read reuses ReadInto validation.

Unfiltered/filtered/entity-list query specializations, small stack bundles, migration caches and scratch pools remain because they serve hot paths. Go 1.26/Linux amd64/i5-11500 measurements use CPU 0, six alternating 150ms runs of separately compiled binaries and an overlay baseline. Medians have no significance claim. All listed warm paths remain 0 B/op and 0 allocs/op; dynamic bundles and copied messages retain their allocations.

Three initially slower cases were rechecked for six 800ms runs: Changed query 80,945→79,776.50ns; dynamic prepared 16-component bundle 1,137.50→1,125ns; eight-component bundle 492→504.40ns (128 B/8 allocations). A third binary restoring only component storage measured 468.35ns against 474.65ns for full changes, with overlapping ranges 456–482.3 and 464.3–501.9ns. These results neither establish stable regression nor identical timing.

ECS/race/vet/root/HTML/UI/markdown checks passed, as did Android 12 x86_64 ECS tests with Go 1.26/NDK API 24/GOMAXPROCS=2. Temporary device files were removed. No whole-repository or editor screenshot testing was performed.

## Recorded measurements

| Workload | Before ns/op | After ns/op |
| --- | ---: | ---: |
| Recompile 100 independent systems | 17,802.50 | 16,746.50 |
| Recompile 1,000 independent systems | 250,284.50 | 240,154.00 |
| Recompile 1,000-system chain | 145,081.00 | 133,654.00 |
| Component migration | 413.20 | 392.90 |
| PreparedRemoval | 131.15 | 121.80 |
| Direct Patch | 309.00 | 303.70 |
| Two-component write query, 10,000 entities | 109,585.00 | 108,587.00 |
| Two-component read query, 10,000 entities | 59,794.50 | 58,483.50 |
| Combined access to three cache types | 48.75 | 48.89 |

Performance figures describe the recorded workloads and environments, not complete application frame rates. Warm allocation guarantees exclude first use and capacity growth.
