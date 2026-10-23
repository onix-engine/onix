# HTML paint culling

Layout boxes, scroll ranges and control state remain complete while ordinary visuals and static effect textures are generated only near CSS overflow clip windows. Surface dimensions constrain layout without adding implicit clipping to overflow:visible. See [visibility](html-visibility.md) for the second phase.

Windows intersect independently per axis in shared ancestor-clip/descendant-scroll coordinates. Container backgrounds, borders and shadows use inherited windows, not their own content scroll offset. Each axis overscans by max(viewport size, 256px), updating membership only after half the margin is consumed. Partial repaint retains the original anchor/window so repeated updates cannot consume the margin without checking descendants.

Existing scroll indexes select roots; reused ancestor caches merge nesting and visit only selected descendants. Ordinary paint checks windows before creating entities/textures. Shadows use actual texture bounds and overflowing descendants remain eligible when their parent is outside. Static-effect bounds integrate with compositing eligibility, including descendants and filter expansion. Off-window groups create no raster buffer/texture; returning groups restore one current-style surface and keep descendants suppressed. Eligible opacity-zero groups retain focus/input but skip textures.

Size/transform policy changes refresh affected descendants. New external position/scroll bindings restore potentially culled content on the next sync. Transforms, animation, external bindings and non-composited-filter descendants conservatively retain paint because their ranges are not fully mapped; external bindings disable early culling for that Surface. Physical clipping still applies. Controls/edit hosts retain interaction entities.

This is not layout virtualization, content-visibility, occlusion culling or texture tiling. Selected roots still inspect every descendant and membership changes rebuild owner lists. Crossing overscan boundaries allocates. Stable warmed scrolling inside the buffer is the allocation-free guarantee.

A 1,000-row measurement records about 1.03µs/op, 0 B/op and 0 allocs/op inside the buffer, versus about 441µs/op, 50,699 B/op and 561 allocs/op for alternating boundary crossings. No old-version speed comparison was made. Tests cover axes, root deduplication, range restoration, effects, controls, bindings, resizing and invisible-layout queries; editor/screenshots were not run.

References: [RenderingNG](https://developer.chrome.com/docs/chromium/renderingng-architecture), [CullRect](https://github.com/chromium/chromium/blob/main/third_party/blink/renderer/platform/graphics/paint/cull_rect.h), [containment](https://drafts.csswg.org/css-contain-2/#content-visibility).

## Examples and commands

```sh
GOMAXPROCS=2 go test -p 1 ./ui/html ./ui ./html/style ./html/layout -count=1 -timeout=180s
GOMAXPROCS=2 go test -p 1 ./ui/html -run '^$' -bench '^BenchmarkHTMLPaintCull' -benchmem -benchtime=200x -count=1 -timeout=120s
```
