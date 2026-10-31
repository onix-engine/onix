# Native DOM parsing

`html/dom` now tokenizes UTF-8 strings and builds Onix nodes directly. It no
longer imports `golang.org/x/net/html`, builds a second DOM, or copies an entire
reader buffer. The direct `golang.org/x/net` module requirement is removed.

The lexical states, tree construction rules, foreign-content adjustments and
entity data are adapted from the Go Authors' BSD implementation; the license is
retained in `html/dom/LICENSE.go-html`. The storage model, source views,
decoding, node construction and reusable API are implemented for Onix. Parsing
follows the [HTML parsing algorithms](https://html.spec.whatwg.org/multipage/parsing.html),
including omitted tags, table insertion modes and foster parenting, formatting
reconstruction/adoption, raw text, RCDATA, script escaping, templates and foreign
integration points.

## Ownership and reuse

`dom.Parse(source)` returns an independent document, as before. Its nodes and
decoded strings remain valid after other calls to `dom.Parse`.

For repeated parsing where the preceding document can be discarded, retain a
zero-value `dom.Parser`:

```go
var parser dom.Parser
root, err := parser.Parse(source)
```

The next `parser.Parse` or `parser.Reset` invalidates the preceding result,
including node pointers, maps, child slices and decoded strings. The parser
must not be copied after use or shared concurrently. Independent parsers can run
concurrently. `Reset` clears source/tree references and retains capacity;
assigning `dom.Parser{}` releases the retained capacity.

Unchanged names, values and text borrow immutable source substrings. Changed
strings use append-only decoded storage. A sorted static table handles all 2,231
named references without a runtime entity map; numeric references include
Windows-1252 replacements, invalid scalar handling and attribute ampersand rules.
Decoded string views remain immutable throughout a parse even when buffers grow.

Nodes use geometrically growing blocks so pointers remain stable. Individual
attribute maps, child slices and buffers for joining adjacent text are reused.
The open-element, formatting, template and attribute scratch storage is retained
as well. Once the required capacities have been warmed, repeated parsing builds
the tree again with zero heap allocations, including entity decoding and tree
repair. Fresh independent documents still allocate their output storage.

Document order and control initialization share one traversal. Selects normalize
on return from their children; a radio normalization traversal runs only when
radio inputs are present.

## DOM behavior

The document-versus-body-fragment convention is preserved: a leading doctype,
`html`, `head` or `body` creates document wrappers; other input remains a body
fragment. Leading comments and decoded whitespace are handled before deciding.
The first significant token is consumed directly, including long text tokens.

Onix continues to omit comments and doctype nodes. Adjacent text exposed after
omitting comments is merged with amortized linear copying. The immediate newline
rule for `pre`, `listing` and `textarea` still observes intervening comments.

HTML names fold ASCII letters only, duplicate attributes keep the first value,
and slashes remain part of unquoted values. SVG/MathML names and attributes keep
their canonical spelling, such as `foreignObject`, `viewBox`, `stdDeviation` and
`definitionURL`. Qualified attribute names, such as `xlink:href`, retain their
prefix instead of colliding with unqualified attributes.

Doctype names compare without ASCII case sensitivity and do not expand character
references. Templates inside foreign HTML integration points continue parsing
the remaining input. Malformed HTML is repaired; nesting beyond 512 open
elements returns `dom.ErrNestingDepth`. The parser treats scripting as enabled
for tokenization; it does not execute scripts.

## Measurements

Linux amd64, Go 1.27, Intel i5-11500. The baseline is the workspace's preceding
parser, including its long-text classification optimization. These figures
measure cumulative Go heap allocations and elapsed parsing time, including DOM
construction and control initialization.

The existing long-text fixtures contain approximately 377 KB of text and entity
references. Each iteration returns a fresh independent document.

| Fixture | Before B/op | After B/op | Allocations before → after | Time before → after |
| --- | ---: | ---: | ---: | ---: |
| Plain fragment | 2,438,314 | 377,793 | 23 → 6 | 2.45 ms → 0.55 ms |
| Leading comment + fragment | 2,451,052 | 385,975 | 29 → 6 | 2.49 ms → 0.59 ms |
| HTML document with article | 1,395,112 | 378,942 | 49 → 14 | 2.20 ms → 0.54 ms |

| Retained parser fixture | Time/op | B/op | Allocs/op |
| --- | ---: | ---: | ---: |
| Plain text, approximately 328 KB | 257 µs | 0 | 0 |
| Text with entities, approximately 377 KB | 403 µs | 0 | 0 |
| 100 attributed elements with controls | 119 µs | 0 | 0 |

Android x86_64 on the connected API 35 device also reports `0 B/op` and
`0 allocs/op` for all three retained-parser benchmarks. Reuse tests additionally
cover formatting reconstruction, malformed tables, SVG, nested templates,
newlines, comments and NUL preprocessing.

## Validation

```sh
go test ./html/dom ./html/css ./html/style ./html/layout ./ui/html ./ui ./module -count=1
go vet ./html/dom ./html/css ./html/style ./html/layout ./ui/html ./ui ./module
go test -race ./html/dom -count=1
go test ./html/dom -run '^$' -fuzz '^FuzzParse$' -fuzztime=20s -parallel=2
go test ./html/dom -run '^$' -fuzz '^FuzzTokenizerProgress$' -fuzztime=10s -parallel=2
go test ./html/dom -run '^$' -bench 'Benchmark(ParseLongText|ParserReuse)' -benchmem -benchtime=300ms
git diff --check
```

All listed package checks and the Android DOM test binary pass. Development
fuzzing executed over 1.8 million inputs. Tests cover source borrowing, ownership
across independent parses, reset/shrink behavior, parents and preorder positions,
all named references, malformed input recovery and the nesting limit.

During development, an external comparison replayed 1,778 html5lib tree
construction sources against the preceding parser, with zero differences after
applying the intended comment/text and foreign-name projections. This replay uses
Onix's document/body-fragment selection rather than each fixture's contextual
fragment entry point. Representative tree outputs are frozen in
`html/dom/html5_tree_test.go`; tests have no external HTML parser dependency.
The doctype and foreign-template corrections have dedicated regression cases.

Android tests used NDK 27.2.12479018, `x86_64-linux-android24-clang`,
`GOOS=android GOARCH=amd64 CGO_ENABLED=1` and `-trimpath`. The temporary device
binary was removed. The editor was not launched.
