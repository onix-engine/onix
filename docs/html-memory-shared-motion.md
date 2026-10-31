# Shared computed motion styles

Phase 2 moves transition lists, animation lists and their raw longhands out of
`ComputedStyle` into an optional immutable extension. A static style has no
extension. Different layout and paint styles with identical motion inputs share
one extension, including its resolved transition array and animation definitions.
On amd64, the inline computed style shrinks from 6,536 to 5,560 bytes (14.9%).

Transition and animation declarations are prepared with stylesheet and inline
declarations. The cascade merges only the supplied longhands into reusable
scratch storage, then resolves and interns the complete motion configuration.
Lengths and other context-dependent properties keep their existing resolution.
This removes repeated list parsing and animation-slice allocation for each node
and on each cached recomputation.

Raw longhands remain available inside the extension for explicit inheritance,
list repetition and CSS-wide resets. Copy-on-write protects shared extensions
when an internal parser edits a copied style. `Animations()` exposes immutable
definitions and `TransitionFor()` reads the resolved transition configuration.
`animation:none` retains its longhands without creating an animation runtime.

Style and motion indexes share generic open-addressed table storage. The
document motion index retains at most 2,048 configurations and is cleared with
the style index. New configurations beyond that bound still resolve correctly;
they may allocate again until the index resets. The zero-allocation result
applies to warmed, reusable configurations within the cache bounds.

## Measurements

Linux amd64, Go 1.27.3, Intel i5-11500. The baseline is the computed-style
implementation in `037da31`; phase 1 did not change that structure or cascade.
Bytes/op measure cumulative Go heap allocations, not RSS or GPU memory.

The fixture contains 256 divs with distinct widths and one shared motion rule.
DOM parsing and declaration preparation occur before timing. Cold runs discard
the document mutation cache and style map; warm runs reuse both.

| Configuration | Before | After |
| --- | ---: | ---: |
| Static, cold | 1,781,235 B/op, 278 allocs/op | 1,616,567 B/op, 278 allocs/op |
| Static, warm | 0 B/op, 0 allocs/op | 0 B/op, 0 allocs/op |
| Transition, cold | 2,369,119 B/op, 10,518 allocs/op | 1,620,675 B/op, 311 allocs/op |
| Transition, warm | 588,056 B/op, 10,240 allocs/op | 0 B/op, 0 allocs/op |
| Animation, cold | 2,012,781 B/op, 5,655 allocs/op | 1,619,355 B/op, 289 allocs/op |
| Animation, warm | 231,525 B/op, 5,376 allocs/op | 0 B/op, 0 allocs/op |

The cached full-style, full-render stylesheet reuse and hover-update benchmarks
also report 0 B/op and 0 allocs/op. Existing transform and color animation frame
benchmarks report 3 B/op and 51 B/op respectively, with allocation counts rounding
to zero.

The 976-byte reduction describes each distinct computed-style struct. Motion
extensions and allocation size classes affect total heap savings, so the
struct-size reduction is not a claim of 14.9% lower total document memory.

## Validation

```sh
go test ./html/style ./html/layout ./ui/html ./ui ./module -count=1
go vet ./html/style ./html/layout ./ui/html ./ui ./module
go test -p 1 ./html/style ./ui/html -run '^$' -bench '^(BenchmarkComputeMotionStyles|BenchmarkComputeFullStyles|BenchmarkHTMLFullRenderStylesheetReuse|BenchmarkHTMLHoverStyleReuse|BenchmarkHTMLCSSAnimationFrame|BenchmarkHTMLCSSColorAnimationFrame)$' -benchtime=200ms -count=1 -benchmem
git diff --check
```

Regression coverage includes extension sharing across distinct styles, detaching
on overrides, declaration removal, every supported motion longhand with
`inherit`/`initial`/`unset`, shorthand resets, inactive explicit defaults, bounded
cache cleanup, and zero warm allocations for inline and stylesheet motion rules.
Incremental layout continues to match full layout, including private style data.

The full style package and focused HTML animation, transition, transform, blur,
filter, shadow and compositing tests passed on Android x86_64 / API 35. Test
binaries used NDK 27.2.12479018, `x86_64-linux-android24-clang`,
`GOOS=android GOARCH=amd64 CGO_ENABLED=1` and `-trimpath`. Temporary device binaries
and test fonts were removed after validation. Visual acceptance remains manual.

The earlier animation baseline and blur changes are documented in
[phase 1](html-memory-animation-blur.md).
