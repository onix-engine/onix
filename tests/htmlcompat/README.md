# HTML/CSS comparison with Chromium

The first batch contains 30 original minimal cases, 94 border boxes and five text
origins. Later batches group features against measured Chromium geometry. This is
not the complete WPT suite or a claim of full HTML/CSS compatibility.

Both engines read resources/batch1.html and repository NotoSansSC-Regular.ttf at
1200x650, DPR 1, Linux. The recorded browser is Chromium 153.0.8010.36; each visual-report.json
stores exact version and font/page SHA256. Historical results below refer to that
recorded environment rather than a new visual run.

| Row | Cases, left to right (six per row) |
| --- | --- |
| 1 | content-box, border-box, percentage width, min-width, max-width, block auto margins |
| 2 | relative flow retention, four-inset absolute, percentage height, block stacking, flex grow/shrink |
| 3 | flex gap, two-axis centering, space-between, column, wrap, row-reverse |
| 4 | order, flex auto margin, align-self:end, overflow:hidden, rounded border, leaf opacity |
| 5 | z-index occlusion, whitespace collapse, pre newlines, wrapping, text centering, CJK wrapping |

## Manual visual reproduction

Run commands below from the repository root. capture.py requires agent-browser and
its Chromium, Xvfb, xwd, Python 3 and Pillow. It creates a temporary HTTP server,
browser session and Xvfb, builds a native application, waits for frame three, captures,
then cleans processes up. Run capture commands only as a deliberate manual visual
check; this repository forbids agents from editor screenshot testing. Ordinary capture
checks Chromium geometry against saved expectations. Use --update-browser only after
reviewing browser/page changes. go run ./tests/htmlcompat opens the native page manually.

## Workbench fixture

resources/workbench.html is a self-contained compatibility page with relative fixture
icons, without display patches or font overrides. Saved chromium-control-display.json
records UA inline-block for ordinary-flow button/input/select/textarea and new-path.
Flex items find-query/dialog buttons blockify to block. Onix preserves those UA defaults
and blockifies through Flex/Grid layout context rather than changing UA display.

The image provider resolves real SVG/PNG resources; TestHTMLBrowserWorkbenchImages and
capture reports check visible pixels for all 13 images. Geometry covers absolute flex
height from top/bottom insets, cross-axis centering, percentage inline-block widths,
intrinsic border-box control height, atomic inline-blocks and DOM-ordered mixed
h2/p/input/block-action flow. Tolerances: stable boxes .1 CSS px, font-dependent controls
1 px, dialog card 1.5 px. Hidden templates have browser zero-size boxes but no Onix
boxes and are excluded. Empty case sets cannot imply visual success: use UA display,
selected geometry, dialog order and centering assertions; screenshots/diffs diagnose.

## Text metrics

line-height.html compares ordinary and flex-button text centers while preserving
line-height:normal. Web fonts use selected ascent/descent/line gap; drawing must not
apply a second line-height compensation after HTML layout. text-box.html covers four
text-box-trim values and flex label centering with cap/alphabetic edges, trimming
outer whitespace without changing multiline baseline spacing. Supported edges: auto,
text, and text/cap/ex paired with text/alphabetic. Read OS/2 cap/x-height or H/x outline
bounds without rasterizing. Saved DOM/text origins use .1px tolerance. Ellipsis should
clip horizontally while preserving vertical CJK/descender overflow.

## Geometry and acceptance

Batch 1 checks all 94 boxes and five origins within .1 CSS px, plus leaf opacity.
Batch 6 adds 19 cases/56 boxes for flex axes, grid placement/spanning/alignment,
display and relative/absolute positioning. Batch 7 adds 18 cases/50 boxes for implicit/
auto/stretch grid tracks, flex basis/order/wrap-reverse, flow-root/inline-block/contents
and nested positioning. Retained grid changes and warm resize have regressions.
Dynamic checks include adjacent-sibling class edits, canvas background, opacity and
positioned paint order.

Historical first-batch visual checks passed 30/30 structural cases. Nontext compares
bidirectional 3x3 neighborhood color ranges with channel tolerance 16/255 and one-device-
pixel raster edge allowance. Fixed black-text/blue-background cases extract ink below
brightness 90 and compare contours within one pixel; this is not a general font/color
criterion. Text origins also have geometry assertions. Raw diff.png and reports retain
AA, rounded-edge and fractional-flex differences: structural success is not zero pixel error.
Outputs include chromium.png, onix.png, diff.png, boundary-diff.png and visual-report.json;
chromium-layout.json/chromium-text.json hold geometry.

Batch 6 covers fixed/fr/repeat tracks, gaps, explicit/auto placement, spans, inline-grid,
flex rows/columns/wrap/absolute children and display variants. Batch 7 adds skipped grid
lines, intrinsic auto sizing/stretch, fixed flex basis/reverse wrap, display:contents
flattening and relative right/bottom/absolute auto-width/z-index. Named lines, minmax,
adaptive tracks, dense placement and full CSS Grid are not claimed.

Selector fixes cache sibling relations once, ignore text nodes and backtrack ancestors.
Other fixes include start/end alignment, block/flex auto margins, definite-height
propagation, absolute constraints, canvas background, positioned order, leaf opacity,
text alignment and GPOS kerning. Kerning queries raw pair/class-pair buffers (types 2
and extension 9), Latin/DFLT horizontal kern, without expanding pair matrices. Queries
and context-free selector matching assert zero allocations. This is not full shaping.
Group opacity for overlapping subtrees, complex stacking, tables, RTL/vertical writing,
dynamic scrolling, native form appearance and multiple DPRs remain outside these batches.

## Rasterization rounds

text.html has 18 cases: 12/16/24/32px Latin/CJK/numerals, black/white/color/alpha text,
quarter-pixel X phases and leaf opacity; 36 boxes/18 origins have Chromium assertions.
Historical round 2 smoke metrics passed 18/18 at mean per-channel absolute error <=1.5/255
on 380x95 cases. White space counts in that denominator, so inspect raw diffs and counts
above 16/255 too. This does not establish pixel identity.

Round 2 replaced 4x4 point coverage with horizontal area integration and 16 vertical
samples, retained 8-bit grayscale, cached quarter-pixel horizontal phases and snapped
baselines under standard axis-aligned device projection. Text with known opaque
backgrounds uses RGB coverage/channel blending; transparent, outlined, shadowed,
transformed or custom-projection text uses grayscale. LCD uses fourfold horizontal
sampling and Skia's 12-tap FIR, without screenshot-specific color fitting. Vulkan/Metal
create LCD pipelines lazily; GLES/WebGL/WebGPU support matching blend factors. Only
Linux Vulkan visuals were run; Metal was unverified. Cascade propagates opacity/background
state in its existing traversal; grayscale/LCD atlases and cache queries have warm
zero-allocation tests. Cold glyph/atlas/pipeline creation allocates.

Recorded raw RGB error reductions for the original text cases: normal 59.5%, pre 61.5%,
wrap 61.4%, center 52.2%, CJK 35.1%. text-progress.json and text-comparison.png retain
history (Chromium/before/after, nearest-neighbor zoom). Round 2 lacked hinting/gamma
integration; old results do not establish the later pure-Go path's output.

Round 3 uses one pure-Go SFNT/CFF parser, outline flattening, nonzero-winding coverage
and cache across targets, without cgo/pkg-config/system rasterizer dependencies.
Advance/GPOS metrics stay unrounded. Fourfold horizontal sampling, three-phase 12-tap
FIR and ink expansion precede gamma correction/coverage quantization. Recorded Linux
Chromium parameters: gamma 1.2, contrast .2, internal gamma 76/64. LUTs use three-bit
canonical foreground RGB intensity (luminance for grayscale); color participates in
cache keys. Preserve original masks and full grayscale precision. Opaque-background
rgba text blends per channel; opacity-group text still uses grayscale without full
offscreen group composition.

capture.py records fontRasterizer. Regenerate visuals after changing rasterizers;
committed historical screenshots, hinting-progress.json and hinting-comparison.png
are not current pure-Go validation. Recorded 11-package and font/text race tests passed;
assertions cover gamma monotonicity/endpoints, color isolation, mask immutability,
warm grayscale/LCD/translucent text allocations. Geometry baselines were not changed.
inkInteriorMeanAbsoluteRGB excludes a 2px box boundary and measures union ink pixels
whose background difference exceeds 12/255 in any channel, reporting counts too. This
reduces blank-space dilution without implying visual identity. Outline, scan conversion,
GPU/software rounding and opacity paths still differ; thresholds/browser/font were
not changed to hide them.

References: [Chromium gamma](https://github.com/chromium/chromium/blob/153.0.8010.36/skia/BUILD.gn),
[Fontations web fonts](https://github.com/chromium/chromium/blob/153.0.8010.36/third_party/blink/renderer/platform/fonts/web_font_typeface_factory.cc),
[Skia scaler](https://github.com/google/skia/blob/main/src/core/SkScalerContext.cpp),
[gamma](https://github.com/google/skia/blob/main/src/core/SkMaskGamma.cpp),
[LCD blending](https://github.com/google/skia/blob/main/src/core/SkBlitter_ARGB32.cpp).
The Skia BSD notice is preserved in font/resources/SKIA_LICENSE.

## Positioning/flex batches 2 and 3, recorded 2026-09-15

Batch 2: 12 cases/44 boxes; batch 3: 18 cases/78 boxes. Historical Chromium geometry
and Vulkan structural visuals passed at the existing .1 CSS px/one-device-pixel edge
tolerances, including repeated batch 1. Cases cover absolute min/max and bottom/border-box
constraints, percentage placement, flex space-around/evenly, column-reverse, auto margins,
align-content, wrap-reverse, grow/max, shrink/min, percentage/border-box basis, nested
flex, sub-one factors, mixed min/max and auto cross sizes.

Apply absolute min/max after inset solving; use constrained content-box height for bottom
placement to avoid double-counted border/padding. Cross-axis auto margins beat stretch/
align-self. Separate flex base/hypothetical sizes and freeze constrained items, redistributing
free space per [Flexbox 9.7](https://www.w3.org/TR/css-flexbox-1/#resolve-flexible-lengths).
Reuse scratch entries; this specification algorithm can require repeated convergence
passes. Constraint-change and warm resize regressions assert zero allocation. Raw baselines,
screenshots/diffs/reports remain in batch2/ and batch3/.

## Pending painting and batch 4

painting-pending.html preserves original candidates and browser geometry/screenshots.
Onix currently fails parsing dashed borders for the full page. It is excluded from the
passing Go set. Pending: dashed/double borders, unequal border joins, per-corner radii,
background-clip, gradients, box-shadow and nested/relative overflow clips. Style-layer
properties alone do not establish drawing support.

Batch 4 originally had 12 cases; eight passed geometry/visual structure: overflow axes,
padding clips, nested/relative clipping, absolute z-index, percentage padding and auto
absolute sizes. Adjacent vertical margins 15px/10px collapse to max=15px. Pending fixtures
retain scrollbars, nested opacity composition and rounded overflow clipping, excluded
from pass totals. Keep the same browser baseline/tolerances for future work.

Additional references: [WPT reftests](https://web-platform-tests.org/writing-tests/reftests.html),
[adjacent siblings](https://www.w3.org/TR/selectors-4/#adjacent-sibling-combinators),
[box alignment](https://www.w3.org/TR/css-align-3/),
[GPOS](https://learn.microsoft.com/en-us/typography/opentype/spec/gpos).

## Commands

```sh
go test ./font ./internal/css ./internal/style ./internal/layout ./ecs ./ui ./project ./text ./graphics
python3 tests/htmlcompat/capture.py
```

```sh
go run ./tests/htmlcompat
```

```sh
python3 tests/htmlcompat/capture.py --update-browser
```

```sh
go test ./ui/html -run TestHTMLBrowserWorkbench
python3 tests/htmlcompat/capture.py --fixture workbench
```

```sh
python3 tests/htmlcompat/capture.py --fixture workbench --update-browser
```

```sh
go test ./ui/html -run TestHTMLBrowserLineHeight
python3 tests/htmlcompat/capture.py --fixture line-height
```

```sh
python3 tests/htmlcompat/capture.py --fixture batch6
go test ./ui/html -run TestHTMLBrowserBatch6
```

```sh
python3 tests/htmlcompat/capture.py --fixture batch7
go test ./ui/html -run TestHTMLBrowserBatch7
```

```sh
python3 tests/htmlcompat/capture.py --fixture text
go test ./font ./text ./ecs ./graphics ./render ./internal/css ./internal/style ./internal/layout ./ui ./project
```

```sh
go test ./ui/html -run 'TestHTMLBrowserBatch[123]|TestHTMLFlexConstraintMutation'
go test ./internal/layout -run TestRetainedFlexConstraints
python3 tests/htmlcompat/capture.py --fixture batch2
python3 tests/htmlcompat/capture.py --fixture batch3
```

```sh
go test ./ui/html -run TestHTMLBrowserBatch4
python3 tests/htmlcompat/capture.py --fixture batch4
```
