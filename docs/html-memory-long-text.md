# Long-text parsing and layout memory

Phase 3 reduces transient allocations in long plain-text fragments, paragraphs,
preformatted blocks and wrapping of long words. It builds on the shared motion
styles from [phase 2](html-memory-shared-motion.md).

## Changes

Text tokenization now yields source views through `iter.Seq[textToken]`. Layout,
intrinsic measurement and control labels consume tokens directly. Paragraphs
no longer build or retain a complete token array, and controls can stop scanning
when their visible line is full. `pre-line` detects line breaks while scanning
the text, without a separate search for each line's end.

The existing line-metric traversal also counts glyph storage that cannot be
reused. New glyph fragments share one allocation per line, with each fragment's
slice capacity limited to its own range. This reduces small heap objects without
merging fragments or changing their text, geometry, DOM ownership or hit testing.
Retained reflows reuse the same storage.

Long-word wrapping measures each consumed run once instead of repeatedly summing
the remaining suffix. Runs share immutable shaped glyphs plus an origin offset;
valid UTF-8 text uses source substrings instead of rebuilding a string per line.
Invalid UTF-8 still produces replacement characters. This also fixes the stalled
wrap loop when a long word starts an indented first line.

Uncached long shaped runs use disjoint ranges in layout scratch storage. Nested
layout has a separate scratch level, so shaping a control cannot overwrite text
waiting on its parent's line. Output fragments copy their glyphs into owned
storage. Retained document contexts reuse the scratch buffers; pooled contexts
drop buffers with capacity above 4,096 glyphs.

Document-versus-fragment detection checks a source prefix before buffering and
decoding a long text token. Comment parsing and entity-prefixed whitespace still
use the HTML tokenizer, and full parsing keeps the existing HTML5 tree builder.
Whitespace checks on tokenizer text use a byte view without a string copy.

## Measurements

Linux amd64, Go 1.27.3, Intel i5-11500. The baseline is the phase 2 workspace
before these changes; its parser and text layout match `037da31`. Bytes/op are
cumulative Go heap allocations, rather than retained heap, process RSS or GPU
memory.

Layout benchmarks prepare DOM/styles and warm the short-token shaping cache
before timing. Fresh runs create a new layout tree. Retained runs mark the text
box dirty and assert that formatting actually ran; they do not measure an
unchanged-tree shortcut. Viewport size is 640 × 480.

| Layout fixture | Before | After |
| --- | ---: | ---: |
| Chinese paragraph, fresh | 5,558,330 B/op, 9,775 allocs/op | 4,609,686 B/op, 176 allocs/op |
| English paragraph, fresh | 7,868,736 B/op, 24,621 allocs/op | 5,755,532 B/op, 597 allocs/op |
| 16,384-character word, fresh | 2,818,391 B/op, 3,261 allocs/op | 993,004 B/op, 250 allocs/op |
| Preformatted text, fresh | 4,919,269 B/op, 14,379 allocs/op | 3,745,848 B/op, 1,051 allocs/op |
| Long word, retained reflow | 864,881 B/op, 1,390 allocs/op | 0 B/op, 0 allocs/op |

Chinese, English and preformatted retained reflows also report 0 B/op and
0 allocs/op, preserving their earlier warm behavior. Fresh layout allocation
volume falls by 17.1%, 26.9%, 64.8% and 23.9% respectively.

Parser benchmarks process approximately 377 KB of mixed English/Chinese text
and entities, including DOM conversion and element state initialization.

| Parse fixture | Before | After |
| --- | ---: | ---: |
| Plain fragment | 4,875,968 B/op, 37 allocs/op | 2,438,367 B/op, 23 allocs/op |
| Comment followed by plain fragment | 4,892,329 B/op, 39 allocs/op | 2,451,041 B/op, 29 allocs/op |
| HTML document with article | 1,395,113 B/op, 49 allocs/op | 1,395,103 B/op, 49 allocs/op |

The full DOM and output fragments still scale with document text size. Streaming
tokenization removes intermediate storage; it does not virtualize document
layout or remove glyphs needed for text painting and caret lookup.

## Validation

```sh
go test ./html/dom ./html/style ./html/layout ./ui/html ./ui ./module -count=1
go vet ./html/dom ./html/style ./html/layout ./ui/html ./ui ./module
go test ./html/dom -run '^$' -fuzz '^FuzzHTMLDocumentDetection$' -fuzztime=3s -parallel=2
go test -p 2 -race ./html/layout -run '^Test(BuildContextReusePreservesLayout|LongTextGlyphStorageAndReflow|ShapeTokenCachesShapedResult)$' -count=1
go test -p 1 ./html/dom ./html/layout -run '^$' -bench '^(BenchmarkParseLongText|BenchmarkLayoutLongText|BenchmarkBuildTextDocument|BenchmarkBuildControlsDocument)$' -benchmem -benchtime=200ms -count=1
git diff --check
```

SHA-256 layout recordings were generated before the production changes using
the bundled Noto Sans SC regular font. They preserve fragment text, line indices,
rectangles and every glyph position/advance for Chinese, long UTF-8 words,
`pre-wrap`, `pre-line`, ellipsis and mixed inline controls/images. Tests also
cover cancellation and zero allocations during token scanning, dirty retained
reflows, width changes, shrinking text, neighboring glyph slice ownership,
indented word wrapping and invalid UTF-8. Existing incremental, whitespace,
fallback font, caret and compositing tests pass. The document detection fuzzer
compared approximately 51,000 inputs against the original tokenizer-only rule.

The full DOM package and focused layout/UI text tests passed on Android x86_64,
API 35, including the layout recordings and warm allocation checks. Binaries used
NDK 27.2.12479018, `x86_64-linux-android24-clang`,
`GOOS=android GOARCH=amd64 CGO_ENABLED=1` and `-trimpath`. Temporary device test
binaries and fonts were removed afterward. Visual acceptance remains manual.
