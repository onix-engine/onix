# CSS animations and transitions

Each animated DOM node advances its clock once per frame using `scene.Time.Delta`.
Repeated `html.Sync` calls in the same frame share that clock. `html.Animating`
reports running or delayed animations. Same-name animations preserve elapsed
time across style, keyframe, and layout updates. Removing an animation or hiding
its subtree cancels it. Showing the subtree restarts it.

## Supported behavior

`@keyframes` accepts from/to, percentages, multiple selectors and duplicate offsets; the last matching duplicate definition wins. Component scoping retains keyframes, including media-query reparsing after viewport changes. Animation shorthand and all eight longhands are supported, with multiple animations, cycling lists, fractional/infinite iterations, negative delays, four directions, four fill modes and pause/resume.

Animated properties are transform, opacity, color, background-color and four border colors; color-bearing background/border shorthands expand. Transforms include 2D rotation, translation and scale, percentage translation, relative lengths and 2D transform-origin keywords/lengths. Missing endpoints use base style and each property selects its own surrounding keyframes. Keyframe `!important` is ignored; important base declarations override animations.

Transitions support shorthand, property/duration/timing-function/delay lists, all and shorthand expansion for those properties. Removing a matching property cancels a transition. Changing duration/easing without a new target preserves the running transition's configuration. Reversed interruptions apply the CSS reversing-shortening factor, including negative-delay scaling. Easing includes linear, ease variants, cubic-bezier, steps and jump-start/end/none/both, with correct delay, backwards-fill and endpoint before-flag handling.

Colors interpolate in premultiplied-alpha sRGB so transparent endpoints do not introduce black. Inherited animated text follows ancestors; explicit descendant colors stay independent. `currentColor` works for text/background/borders and is the default border color. Inheritance changes can start descendant color transitions. Removing animation restores base colors.

Initially transparent paint retains entities. Independent borders use per-edge entities; rounded/raster backgrounds reuse textures and pixel buffers, including rounded clip masks. Dynamic descendants prevent invalid background-coverage suppression. Ancestor transforms affect descendants without double-applying control/clip transforms. Zero scale and singular hierarchy matrices retain true zero scaling.

Frames sample time and update retained transforms, alpha and paint colors without recascade or layout. One pass selects property keyframe ranges; inherited color shares the ancestor traversal with transform/opacity. Interned styles remain immutable; raster backgrounds own paint styles. Eligible CPU-composited opacity groups refresh textures only when content changes and apply root alpha outside the texture. Groups containing color animation retain paint behavior and the compositing limitations below.

Warmed rotation, opacity, ordinary/inherited colors and rounded-border raster tests assert zero allocations. Benchmarks use 100 elements; the color case updates backgrounds and four borders. Runtime depends on device, primitive count and raster area.

## Boundaries

Geometry, margins, positions, background images/gradient stops, shadows and filters are not animated. There is no 3D/perspective/matrix or independent translate/rotate/scale property support. Interpolation uses compiled 2D components rather than complete browser function-list matching and matrix decomposition; singular nonuniform-zero-scale-then-rotation lists are rejected.

Animation/transition events, Web Animations API, composition and scroll timelines are absent. Controls, images, overflow clipping, dynamic descendants and other unsupported CPU groups need GPU offscreen compositing for exact group opacity; visual alpha on overlapping children can differ from Chromium. Color syntax remains hex, comma rgb/rgba, basic named colors and currentColor, without complete named/HSL/modern/wide-gamut syntax. CSS-wide/var() keyframes and complete simultaneous animation/transition precedence remain incomplete.

[CSS Animations](https://www.w3.org/TR/css-animations-1/) · [CSS Transitions](https://www.w3.org/TR/css-transitions-1/) · [CSS Easing](https://www.w3.org/TR/css-easing-1/) · [Color interpolation](https://www.w3.org/TR/css-color-4/#interpolation) · [Before flag](https://www.w3.org/TR/web-animations-1/#the-before-flag)

## Verification

The [animation fixture](../../tests/htmlcompat/resources/animations.html) covers
keyframes, transitions, transforms, and color.

```sh
go test ./html/css ./html/style ./ui/html
go test ./ui/html -run '^$' -bench '^BenchmarkHTMLCSS(Color)?AnimationFrame$' -benchmem
```

```sh
go run ./tests/htmlcompat tests/htmlcompat/resources/animations.html
```
