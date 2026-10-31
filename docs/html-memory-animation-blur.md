# HTML animation baselines and streaming blur

Static elements retain an immutable style pointer and their last displayed
foreground color in a 48-byte map value on amd64. They no longer allocate a
512-byte animation record each. Keeping the displayed color preserves the
starting value when a transition is introduced while an ancestor animates its
inherited color. Only elements with animations or enabled transitions own the
complete animation runtime. Removing those declarations releases that runtime;
hidden, detached and replaced nodes also retire their baselines.

Gaussian blur retains a ring of horizontally filtered rows and one accumulation
row instead of a floating-point image. Intermediate storage is
`32 * width * (min(height, 2*ceil(3*sigma)+1) + 1)` bytes before allocation rounding.
The vertical pass accumulates contiguous rows to keep memory accesses local.
Each pixel keeps the same floating-point accumulation order, premultiplied
filtering and final straight-alpha conversion as the original algorithm.

## Measurements

Linux amd64, Go 1.27.3, Intel i5-11500, compared with `037da31`.
Bytes/op are cumulative Go heap allocations, not process RSS or GPU memory.

| Benchmark | Before | After |
| --- | ---: | ---: |
| Static preparation, warmed map storage | 512 B/op, 1 alloc/op | 0 B/op, 0 allocs/op |
| Populate 1,000 static divs and their section, including ancestor caches | 661,484 B/op, 1,048 allocs/op | 336,473 B/op, 47 allocs/op |
| Cold 1024×1024 blur, sigma 5 | 33,554,688 B/op | 1,048,832 B/op |
| Warm 1024×1024 blur, sigma 5 | 0 B/op, 0 allocs/op | 0 B/op, 0 allocs/op |

The blur measurement includes the kernel allocation. Static map entries also
have map overhead; 48 bytes describes the value itself. Full-render stylesheet
reuse and hover benchmarks continue to report zero warm allocations.

## Validation

```sh
go test ./ui/html ./ui ./module -count=1
go vet ./ui/html ./ui ./module
go test ./ui/html -run '^$' -bench '^(BenchmarkHTMLGaussianBlur|BenchmarkHTMLStaticAnimationPreparation|BenchmarkHTMLStaticAnimationPopulation|BenchmarkHTMLFullRenderStylesheetReuse|BenchmarkHTMLHoverStyleReuse)$' -benchmem -benchtime=200ms -count=1
git diff --check
```

Blur pixel recordings were generated with the original full-image convolution.
They cover wrapped rows, changing image sizes and kernels, one-dimensional
images, oversized kernels, and subimages with nonzero origins and larger strides.
Existing tests cover filter order, transparent edges, shadows, grouped opacity,
static transforms, interrupted/reversed transitions and inherited animated colors.

Focused animation, transform, blur, filter and composite tests also passed on the
connected Android x86_64 device. Its test binary used NDK 27.2.12479018,
`x86_64-linux-android24-clang`, `GOOS=android GOARCH=amd64 CGO_ENABLED=1` and
`-trimpath`, with the regular and bold Noto Sans SC test fonts copied alongside it.
Visual acceptance remains manual.
