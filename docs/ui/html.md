# Native HTML and scrolling

Onix parses HTML into scene nodes, computes CSS styles and layout, then emits
retained UI and drawing state. To create a surface, call `html.New(source)` from
`github.com/onix-engine/onix/ui/html`.
[CSS animations and transitions](html-css-animations.md)

## DOM parsing and attributes

`parse.Parse(source)` returns an independent tree. Use a zero-value
`parse.Parser` for repeated parsing when the previous tree can be discarded.
The next `Parse` or `Reset` invalidates borrowed nodes, attributes, child slices,
and decoded strings. `Reset` clears references and keeps capacity. Assign
`parse.Parser{}` to release that capacity. Use one parser per concurrent task
and keep a parser at the same address after first use.

The parser follows HTML tree construction rules for malformed markup, tables,
templates, and SVG/MathML. A leading doctype, `html`, `head`, or `body` selects
document parsing. Other input is treated as a body fragment. The DOM omits
comments and doctype nodes. The parser treats scripting as enabled for
tokenization. Onix renders the resulting tree without executing scripts.
Nesting beyond 512 open elements returns `parse.ErrNestingDepth`.
[HTML parsing algorithms](https://html.spec.whatwg.org/multipage/parsing.html)

```go
element := node.NewElement("div", node.Attribute{Name: "id", Value: "status"})
element.SetAttr("data-state", "")
value, present := element.Attr("data-state") // An empty value can be present.
element.RemoveAttr("data-state")
```

Use `AttrValue` for a string-only lookup, `Attributes()` for unordered iteration,
and `SortedAttributes()` for deterministic output. Replace the full set with
`SetAttributes`. Duplicate attributes keep the first value. HTML names fold
ASCII case, while foreign names such as `viewBox` keep their canonical spelling.
Template clones share attribute storage and detach on mutation. Strings
borrowed from a reusable parser still expire with the parse result.

## Visibility and paint culling

`visibility: hidden` keeps layout, scroll ranges, and control state. Descendants
can restore visibility with `visibility: visible`. Hidden controls skip drawing
and hit testing, and lose keyboard focus while retaining values and selection.
Opacity and filter groups rasterize visible content, including descendants
that restore visibility. `visibility: collapse` hides ordinary boxes. Special
table and flex collapse layouts remain unsupported.
[CSS visibility](https://drafts.csswg.org/css-display-3/#visibility)

Overflow paint culling keeps the full layout tree and mounts ordinary visuals
and static effect textures near the scroll clip. Each clipped axis has an
overscan margin of at least 256 CSS pixels or the viewport size, whichever is
larger. Warmed scrolling inside the buffer reuses paint. Crossing the update
threshold can mount new paint and allocate. Transforms, animations, external
bindings, and unsupported filter groups can retain paint conservatively.
Surface dimensions constrain layout. Clipping follows CSS overflow rules.
[CSS overflow](https://www.w3.org/TR/css-overflow-3/)

## Sticky positioning

```html
<div style="height:200px;overflow:auto">
  <div style="position:sticky;top:0;background:white;z-index:1">Header</div>
  <div style="height:1000px">Content</div>
</div>
```

Sticky elements stay in flow. The renderer updates their positions before
transforms and hit testing. The nearest `overflow: auto`, `scroll`, or `hidden`
ancestor provides the scroll reference, with the surface viewport as fallback.
`overflow: clip` leaves the scroll reference to an outer ancestor or the viewport.
Insets work per axis and accept percentages and `calc()`. Table bounds apply to
sticky rows and cells. Sticky subtrees stay mounted outside the scroll paint buffer.
Plain unboxed inlines and external `SetNodeScroll` bindings lack sticky geometry.
[CSS sticky positioning](https://www.w3.org/TR/css-position-3/#stickypos-insets)

## Scrollbars

Scrollbars overlay content by default. Set `ReserveScrollbarSpace` on
`ui.ScrollAreaOptions`, `ui.ScrollArea`, `ui.ListOptions[T]`, `html.Surface`,
or `layout.Cache` to reserve classic gutters.

```go
area := ui.SpawnScrollArea(world, ui.ScrollAreaOptions{
    Width: 120, Height: 80,
    ContentWidth: 120, ContentHeight: 160,
    ReserveScrollbarSpace: true,
})
metrics, _ := ui.GetScrollAreaMetrics(world, area)

surface := html.New(`<div style="width:120px;height:80px;overflow:auto;scrollbar-width:thin"><div style="height:160px"></div></div>`)
surface.ReserveScrollbarSpace = true
```

Gutters reduce the content viewport and may trigger overflow on the opposite
axis. The outer box keeps its specified dimensions. `scrollbar-width: none`
allows scrolling with zero gutter space. Changes in mode or thickness update
the scroll range and clamp the current offset.
[CSS scrollbars and layout](https://www.w3.org/TR/css-overflow-3/#scrollbars)

## Verification

```sh
go test ./html/parse ./html/layout ./ui/html
go test ./html/parse -run '^$' -bench '^BenchmarkParserReuse$' -benchmem
go test ./ui/html -run '^$' -bench '^BenchmarkHTML(PaintCull|Visibility)' -benchmem
```

[Chromium compatibility fixtures](../../tests/htmlcompat/README.md)
