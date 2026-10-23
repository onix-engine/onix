# HTML visibility: phase 2

`visibility:hidden/visible` inherits through the tree. Hidden elements retain layout boxes, flow space and scroll ranges; descendants can explicitly restore visibility. This works with [overflow paint culling](html-paint-culling.md).

The style system supports defaults, inheritance, case-insensitive keywords and CSS-wide values, classifying changes as visual rather than layout. Hidden ordinary paint skips entities and textures; controls retain entities, values and selection while disabling drawing/hit testing. Clip containers retain structural visibility, geometry and scrolling, but reject their own wheel input and hide scrollbars. Hidden background colors are not reused for rounded clip masks.

Opacity/filter groups rasterize visible content only; visible descendants remain affected by ancestor effects. Entirely hidden groups allocate no raster buffer or texture. Text visibility invalidates raster caches. Hidden controls cannot take programmatic, Tab or autofocus; UI clears keyboard focus while preserving input state. Surface hide/show respects CSS and individual checkbox, indicator, selection and caret rules.

Button text preserves descendant styling, allowing visible text/images inside hidden buttons. Mixed-visibility edit hosts keep fragment drawing rather than flattening hidden text into one entity. DOM changes use local reconciliation. Pseudo-class visual changes reuse layout and update style pointers during existing paint/compositing traversals with generation deduplication. Ancestor backgrounds refresh when hidden covering children expose them.

Geometry and text measurement still run for hidden content. Recreating ordinary paint on visibility restoration may allocate; the warmed single-line-input class-toggle benchmark records 0 B/op and 0 allocs/op because text/caret/selection entities survive. Pseudo-class changes still reconcile all paint, while attribute changes can be local. Shared fragments/static groups keep complete fallback.

`collapse` parses/inherits and hides ordinary boxes; special table/flex collapse layout is absent. Visibility animation, full rich-text editing, content-visibility, occlusion and texture tiling are outside scope. Tests cover inheritance, geometry, controls, focus, surface visibility, deferred textures, groups/caches, parent-background restoration, pseudo-class layout reuse and fresh-render equivalence. Editor/screenshots were not run.

References: [CSS visibility](https://drafts.csswg.org/css-display-3/#visibility), [inheritance](https://drafts.csswg.org/css-cascade-4/#defaulting), [RenderingNG](https://developer.chrome.com/docs/chromium/renderingng-architecture).

## Examples and commands

```sh
GOMAXPROCS=2 go test -p 1 ./ui/html ./ui ./internal/style ./internal/layout -count=1 -timeout=180s
GOMAXPROCS=2 go test -p 1 ./ui/html -run '^$' -bench '^BenchmarkHTMLVisibility' -benchmem -benchtime=200x -count=1 -timeout=120s
```
