# ECS simplification and developer APIs

Recorded 2026-10-03 against `0fdbd194`. Items 2–7 reduce production code by 204 lines (284 added/488 removed), excluding tests/docs; regressions and benchmarks add 254 lines.

Get, GetRef and compare-before-write share generic componentAt lookup while keeping inline reads. GetMut retains its direct path to avoid extra location-return overhead. Public `World.SetIfChanged[T comparable]` and `CommitComponent` replace private UI helpers: equal values do not mark Changed or advance write ticks; actual writes preserve Added and update Changed/version. Mutable access itself still marks change.

Controls share UI/interaction/surface/part bundle construction; Button/Panel/Input/Textarea install root components together, with generic text-input creation and precomputed capacities. FindPart[P,T] caches entities in a World index, rebuilding on Relation/Part versions including migration. Cursor/selection and same-stage scrollbar measurement reuse it. First-match semantics include missing display components; measurement caches refresh before and after layout independently.

Top-level keyed slots and nested children share matching, node reuse, removal, nested-state cleanup and scratch. Slots preserve static children and alone maintain their node indexes at commit. Duplicate keys, tag replacement, scope classes and cleanup semantics remain. HTML definitions/mounting/DOM APIs live in ui/html; ui forwarding aliases/constants are removed, while operations that synchronize form state remain in UI.

SetIfChanged returns whether a write occurred, not entity existence. Missing/stale components return false without insertion. Indexes never retain component pointers across migration; first initialization/growth/entity creation can allocate. Input retains password behavior, textarea plain text and existing disabled/style/default semantics.

Go 1.26/Linux amd64/i5-11500 medians use three 300ms runs without significance testing. Cold button time falls about 49%, allocations 4693→3875 (about 17%); fontless scrollbar cases isolate relation/surface cost, not full rendering. Alternating 500ms component checks show Get 21.17→19.96ns, GetRef 20.81→19.93ns, 10,000 GetMut 268617→244986ns. GetMut implementation is unchanged, so opposite timing fluctuations establish neither gain nor regression. Alternating keyed slot/children medians are 4249→3823ns and 3868→2879ns, warm allocation-free.

Targeted ECS/UI/physics/root/project/vet/editor-build checks and regressions for ticks, stale handles, indexes, real-font refresh, defaults and keyed cleanup passed. See the raw record for complete comparisons.

## Examples and commands

```go
// Write existing components only when changed; missing/stale entities return false.
world.SetIfChanged(entity, scene.Visibility{Visible: true})

// Commit changes using the query tick.
for _, access := range world.Query[scene.Visibility]().All {
	ecs.CommitComponent(access, scene.Visibility{Visible: true})
}

// HTML uses github.com/diiyw/onix/ui/html.
page := html.New("<button>Play</button>")
```

```sh
go test ./ecs ./ui -p=1 -run '^$' \
  -bench '^(Benchmark(Refactor.*|GetMutTenThousand|QueryAccess2TenThousand|TraversalValueSurfaces|HotpathStaticControls|SpawnBundle2OneThousand|Spawner2OneThousand))$' \
  -benchmem -benchtime=300ms -count=3
```

```sh
go test ./ecs/... ./physics . ./project/... -p=1 -count=1
go vet ./ecs/... ./physics ./project/...
go build -p=1 ./editor
git diff --check
```

## Recorded measurements

| Workload | Before ns/op | After ns/op | After B/op / allocs/op |
| --- | ---: | ---: | ---: |
| Get by value | 20.52 | 19.90 | 0 / 0 |
| GetRef borrowed read | 20.63 | 21.19 | 0 / 0 |
| 10,000 GetMut operations | 253,901 | 267,476 | 0 / 0 |
| 10,000 QueryAccess2 rows | 88,656 | 89,766 | 0 / 0 |
| 32 reversed keyed slot nodes | 4,208 | 3,900 | 0 / 0 |
| 32 reversed keyed children | 3,635 | 2,973 | 0 / 0 |
| New World plus 50 buttons | 1,120,079 | 568,762 | 755,881 / 3,875 |
| 100 TextField scrollbar surfaces | 2,126,016 | 1,761,699 | 0 / 0 |
| ValuePart surface sync | 182,325 | 169,871 | 0 / 0 |
| Static control size sync | 95.26 | 97.58 | 0 / 0 |

Performance figures describe the recorded workloads and environments, not complete application frame rates. Warm allocation guarantees exclude first use and capacity growth.
