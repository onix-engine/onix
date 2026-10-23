# Local zero-size clip membership: phase 10

Existing boxes crossing between zero and positive size now create/retire clips and rebuild descendant routes locally, for both style and structural size changes. Full-render padding-box semantics remain: both dimensions must be positive; zero content height with positive padding can retain a clip.

`ClipChanged` replaces the new-box-only `Fresh` distinction. Full and partial rendering share `htmlBoxNeedsOverflowClip`. One queue traversal handles clip movement, membership and control/focus dependency expansion. Even unchanged document coordinates need new local coordinates and parentage when the clip context changes.

Nested invalidation roots merge; route cleanup visits only outermost changed DOM subtrees, including invisible inline nodes. Once all primitives and focus bounds are complete, routes rebuild parent-first, explicitly removing stale routes without a clip ancestor. Retired clips leave indexes and are destroyed by group cleanup. Descendants attach to the new clip, a valid outer clip or the HTML owner. Retained nested clips preserve scroll positions; recreated clips begin with new-entity state. Owner membership changes rebuild the list. External bindings replay only on updated primitives.

Fresh/full comparisons and headless frames cover one/both zero dimensions, all clipping modes, nested changes, padding, absolute descendants, focus/edit contexts, route cleanup, live input and selection, and structural size collapse. With 1,000 unrelated clips, only the target input snapshots/updates and no unrelated clips update.

Overflow mode, tree styles, z-index, reparenting and shared ownership still fall back. Context-wide route updates and owner rebuilding can exceed a single box. New entities allocate. Benchmarks compare `f07362d`, three 500ms runs on Linux amd64/i5-11500/Go 1.26: a 120px outer box isolates a target switching between 40px and 0px. Both versions report 9 allocs/op; baseline 669–680 B/op, current 680–681 B/op.

## Commands

```sh
go test ./ecs/... ./html/layout ./html/style ./html/dom ./html/css ./project
go vet ./ecs/... ./html/layout ./html/style ./html/dom ./html/css
git diff --check
```

## Recorded results

| Unrelated clipped controls | Phase 9 | Phase 10 | Speedup |
| --- | ---: | ---: | ---: |
| 10 | 111.44 μs | 23.22 μs | 4.80× |
| 1,000 | 11,732.77 μs | 144.34 μs | 81.29× |

Raw commands and measurements: [html-incremental-clip-membership-benchmarks.txt](html-incremental-clip-membership-benchmarks.txt).

These are historical measurements for the described paths, not editor frame times. Initialization and warm-up are excluded unless stated otherwise. Allocation guarantees apply only to the explicitly tested warm paths. Editor and screenshot checks are performed manually.
