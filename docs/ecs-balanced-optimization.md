# ECS CPU and memory optimization

Measured on 2026-10-24 with Go 1.27.0, Linux amd64, Intel i5-11500 and
GOMAXPROCS=6. Baseline ECS sources are from
`53411652cb8a7c28394aac1ffeb115ba7d636e59`, with the same new benchmarks added.
Before/after binaries ran alternately, three times each, for 300 ms per case.
Numbers below are medians, not whole-engine frame-time predictions.

Command queue descriptors are pointer-free and use world-local stream IDs.
Typed insertion/patch payloads remain in reusable streams, while prepared
removal plans use a reusable pointer slice with direct application. Discard,
error, panic and successful flush clear payload references and cancel pending
spawn reservations. The descriptor is 24 bytes on amd64, down from 64 bytes
(62.5% smaller); this excludes payload arrays and shared stream metadata.

Patch/removal Reserve only prepares source tables that have already held rows
or have been preallocated. Replacement-only transitions do not grow row storage,
and migrations reserve destination columns without growing world entity slots.
Reserve source spawners first when warming an empty scene. Previously unused
source shapes still require preparation or first-use warming. The request remains
a capacity per migration target, rather than a global budget across targets.

Added/Changed traversal skips unchanged 4096-row groups, then 64-row blocks,
then tests exact row ticks. Block bounds occupy the front of the existing bounds
arrays; lazily rebuilt group summaries occupy their tails. Array growth moves
and invalidates only cached summaries while preserving leaf bounds. The two
tick kinds add a logical 16 bytes per 4096 rows, before allocator rounding and
spare capacity. No extra per-column slice headers or group allocation streams
are needed. Inverted lower bounds allow min/max accumulation with two max
operations. Row writes and public revisions remain immediately visible.
Unfiltered traversal skips empty tables before binding columns.

| Benchmark | Before ns/op | After ns/op | Time change |
| --- | ---: | ---: | ---: |
| Changed, 10,000 rows, one change | 1,115 | 675.8 | -39.4% |
| Changed, 100,000 rows, no changes | 5,638 | 327.5 | -94.2% |
| Changed, 100,000 rows, one change | 7,135 | 782.3 | -89.0% |
| Changed, 100,000 rows, one write and fresh window each frame | 7,152 | 852.5 | -88.1% |
| Changed, 100,000 rows, one change per 64 rows | 363,621 | 285,836 | -21.4% |
| Changed, 100,000 rows, all rows changed | 611,498 | 523,886 | -14.3% |
| 1,000 queued inserts and flush | 58,920 | 53,008 | -10.0% |
| 1,000 queued spawns, flush and despawn | 103,161 | 95,683 | -7.2% |
| 1,000 prepared removals, including spawn/despawn | 91,809 | 93,321 | +1.6% |
| One recycled insert command and flush | 120.2 | 75.5 | -37.2% |
| Mixed patch commands | 411.2 | 366.6 | -10.8% |
| Read two columns, 10,000 rows | 51,170 | 51,998 | +1.6% |
| Write one/read one column, 10,000 rows | 74,184 | 75,211 | +1.4% |
| Acquire mutable access to both columns, 10,000 rows | 96,072 | 105,298 | +9.6% |
| Recycled mixed Patch | 237.1 | 249.9 | +5.4% |
| Recycled eight-column migration round trip | 358.8 | 390.8 | +8.9% |
| Cold patch reservation with unused registered shapes | 99,075 | 17,080 | -82.8% |

All measured warmed cases retain 0 B/op and 0 allocs/op, including the fresh
per-frame change-window benchmark, which reuses its SystemContext storage.
The cold reservation benchmark includes world and plan construction, one used
source shape, five unused shapes, and a capacity request of 1024. It falls from
303,199 to 47,249 B/op (-84.4%) and from 235 to 111 allocations/op. These are
allocated bytes, not a measurement of process RSS.

The tradeoff is additional summary invalidation on writes and row movement.
Dense mutable access and migration show the regressions listed above; this
change favors sparse-change systems and lower queue/reservation memory.
Block bounds may retain removed ticks, so historical windows and tick wraparound
can conservatively scan extra rows. Exact row filtering remains authoritative.
Only three samples were collected; small percentage differences should be
treated as directional rather than statistically established.

Validation passed for the full ECS suite on Linux and the connected Android
x86_64 environment, selected regression/concurrent-world tests under the race
detector, and `go vet -p=1 ./ecs ./internal/ecsgen`. Generated query, spawn and
patch adapters reproduce byte-for-byte with `go generate ./ecs`.

New regressions compare query output to exact row ticks across block/group
boundaries, cached-summary growth, empty-table reuse, migration, swap removal,
saved windows, multiple filters and tick wraparound. Command tests cover mixed
spawn ranges, partial discard and removal-plan reference cleanup after errors
and panics. Reservation tests check destination capacity without growing unused
source tables or entity metadata.

The `scene` and `ui/html` integration suites passed. The UI suite still fails
`TestBindControlExpandsGenericTextControlMode` and `TestHTMLInputEditsAndEmitsEvents`;
physics still fails `TestDynamicPoseRetainsPrecisionBetweenECSWrites`. All three
failures reproduce with baseline ECS production sources overlaid onto the same
current integration sources, with matching failure values.

Raw records: [before](ecs-balanced-before.txt), [after](ecs-balanced-after.txt),
[Linux validation and baseline failure reproduction](ecs-balanced-validation.txt),
[Android full suite](ecs-balanced-android-tests.txt).
