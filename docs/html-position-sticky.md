# CSS sticky positioning

`position: sticky` keeps an element in normal flow and adjusts its rendered
position after scroll input, before global transforms and hit testing. It also
establishes a stacking context when `z-index` is `auto` and a containing block
for absolutely positioned descendants.

```html
<div style="height:200px;overflow:auto">
  <div style="position:sticky;top:0;background:white;z-index:1">Header</div>
  <div style="height:1000px">Content</div>
</div>
```

The implementation supports `top/right/bottom/left` and `inset`, including
percentages and `calc()`, independently on each axis. An axis with two `auto`
insets scrolls normally. Sticky movement respects margins and containing-block
limits, reduces the end inset for oversized boxes, and composes through nested
sticky elements, controls, and overflow clips. Table cells and rows are bounded
by the table, allowing sticky `th`, `tr`, and `thead` headers.

The nearest CSS `overflow:auto/scroll/hidden` ancestor supplies the scroll
reference; `overflow:clip` does not. Without one, the HTML surface viewport is
used. Geometry cases were checked against Chromium, accounting for Onix's
overlay scrollbars. Native external `SetNodeScroll` bindings and unboxed inline
elements such as plain `span` do not yet supply sticky layout geometry.

Sticky subtrees stay mounted outside the ordinary scroll paint buffer. Each
sticky box is solved once per scroll update, and scroll/sticky translations are
applied in one entity traversal. Warm buffered synchronization and scrolling
allocate zero objects. A sticky descendant prevents its ancestors from being
flattened into a static CPU opacity/filter texture; existing per-primitive
opacity fallback applies to those ancestors.

Validation covers geometry, scrolling in both directions, nested containers,
table bounds, style changes, retained controls, paint culling, allocation counts,
and a wheel/input-picking integration test. The editor was not launched.

Reference: [CSS Positioned Layout §3.4](https://www.w3.org/TR/css-position-3/#stickypos-insets).
