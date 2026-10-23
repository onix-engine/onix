# HTML/CSS engine improvements: first batch

This batch retains Onix layout and ECS reconciliation while implementing six areas identified during comparison with litehtml. W3C/WHATWG are the standards targets; these changes do not establish full Chromium compatibility.

CSS parsing recovers per declaration, rejects invalid values, handles important precedence and comments, and supports attribute operators with i/s flags and general-sibling selectors. Paint adds layered linear gradients, elliptical corners and blurred outer-shadow masks. Eligible static boxes/text composite before ordered filters and group opacity, preserving input and focus targets.

Stylesheet and inline declarations share typed preparation. Common scalar values parse early; em/rem/vw/vh/calc retain font/viewport context, including correct distinction between em and numeric exponents. Warm reuse covers interned styles, sibling indexes, list glyphs, table measurement and gradient/filter buffers. Resizing recreates textures; detachment releases them.

Sibling attribute changes invalidate the parent subtree; duplicate/contained roots merge. Color changes refresh composite textures. Complex conditional dependencies conservatively broaden scope. HTML5 trees use existing x/net/html, retain empty head elements, add outside list markers/numbering, basic tables, float/clear/BFC, and feed post-layout content sizes into container queries.

## Algorithms and costs

One parent scan caches forward/reverse/type sibling positions in map-inline values. Tables measure column constraints before cell layout and row-height allocation. Cached column starts and accumulated row-group rectangles avoid rescans; row spans stay inside their row group and rowspan=0 extends to its end. Floats share exclusion ranges within their formatting context; new BFCs isolate and contain their floats.

Gradient/corner parameters prepare before pixel drawing. Consecutive color filters merge into one pixel pass but clamp after each function. Blur uses separable Gaussian convolution on premultiplied colors before returning to straight alpha, avoiding dark transparent edges; kernels truncate at three standard deviations. CPU cost grows with pixels/kernel width despite warm buffer reuse. Container queries allow at most four feedback rounds and still recascade globally; cyclic sizing is not fully specified.

Tests cover recovery, precedence, selectors, HTML5 tree repair, relative lengths, lists, row/column spans, groups, floats, incremental layout, query feedback, compositing/filter order, input bounds and texture release. Warm table, blur, drop-shadow and outer-shadow paths have allocation assertions; initialization, new glyphs, growth and topology can allocate.

## Boundaries

CSS syntax, escapes, recovery, cascade layers, custom properties and modern selectors remain partial. Fragment parsing uses body context, without arbitrary table/select contexts or a complete comment/doctype/namespace model. Tables lack collapsed borders, col/colgroup widths, anonymous boxes, baseline and full intrinsic sizing; float edge placement and complex line/intrinsic behavior are incomplete. Lists support outside/common numeric/alphabetic/Roman markers, not complete inside/image/counter/hidden/flex numbering.

Background origin/clip/size/repeat combinations and inset shadows remain incomplete. CPU effects cover static boxes/text only; images, native controls, overflow, shadow subtrees, non-gradient images, non-solid borders and child transforms/z-index retain existing painting without general group-filter correctness. Inline effects, full SVG filter graphs/linearRGB and default drop-shadow currentColor remain unfinished. Compositing-context changes require full primitive reconciliation; there are no retained GPU effect layers or local offscreen repaint.

References: [HTML parsing](https://html.spec.whatwg.org/multipage/parsing.html), [tables](https://www.w3.org/TR/CSS22/tables.html), [floats](https://www.w3.org/TR/CSS22/visuren.html), [filters](https://www.w3.org/TR/filter-effects-1/). Recorded checks excluded editor, screenshot and device testing.

## Examples and commands

```sh
go test ./internal/dom ./internal/css ./internal/style ./internal/layout ./ecs ./ui -count=1
git diff --check
```
