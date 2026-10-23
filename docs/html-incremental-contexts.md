# Local HTML clipping, scrolling and events: phase 4

Phase 4 (`1842f0a`) allows ordinary text, size and paint changes to update only affected boxes, clip entities and routes when clipping hierarchy and event bindings are stable. Unrelated scroll branches and controls retain state. See [phase 5](html-incremental-controls.md).

Clip entities belong to their box groups and have explicit identities. Slot cleanup retains them, and owner-list rebuilding includes them after fragment or event membership changes. Full and partial rendering share clip creation and routing. Retained `UIScroll` positions use the existing clamping rules when content or viewports shrink.

The changed queue updates cached descendant bounds bottom-up and clip entities top-down. Unchanged bounds skip recalculation; actual range changes propagate upward using cached sibling ranges. Moving a clip origin queues descendants even if their document coordinates are unchanged; queue deduplication avoids repeated nested scans. Only selected event targets and clip routes are rebound. New fragments get routes; unchanged handlers and input entities survive.

Per-box metadata adds persistent storage proportional to box count. Warm queues, groups and route maps reuse capacity. Tests cover overflow modes, fragment membership, nested clips, viewport changes, shrinking scroll ranges, absolute descendants, zero-sized clips, events, selection and headless scrolling frames. Fresh-render comparisons verify primitives, layout, routes and cached ranges. With 1,000 unrelated scroll containers, equal-height text updates reconcile three boxes, update one clip and rebind one route without rescanning content ranges. The stable-click warm path asserts zero allocations.

Historical fallbacks remain for topology, content attributes, environment, z-index, clip membership/mode changes, affected native/focus/edit contexts and shared ownership. Event binding changes require one complete refresh. External node offset/scroll bindings are not yet local. Ancestor checks, moved clip descendants and owner-list rebuilding retain document-dependent costs. Benchmarks compare `d5e42bc`, three 200ms runs on Linux amd64/i5-11500/Go 1.26.

## Commands

```sh
go test ./ecs/... ./html/layout ./html/style ./html/dom ./html/css ./project
go vet ./ecs/... ./html/layout ./html/style ./html/dom ./html/css
go test ./ecs -run TestHTMLContextWarmScrollTextDoesNotAllocate -count=5
```

## Recorded results

| Unrelated scroll containers | Phase 3 | Phase 4 | Speedup | Phase 3Allocations | Phase 4Allocations |
| --- | ---: | ---: | ---: | ---: | ---: |
| 10 | 49.26 µs | 11.84 µs | 4.2× | 11 | 0 |
| 1,000 | 5.26 ms | 77.57 µs | 67.8× | 1,001 | 0 |

Raw commands and measurements: [html-incremental-contexts-benchmarks.txt](html-incremental-contexts-benchmarks.txt).

These are historical measurements for the described paths, not editor frame times. Initialization and warm-up are excluded unless stated otherwise. Allocation guarantees apply only to the explicitly tested warm paths. Editor and screenshot checks are performed manually.
