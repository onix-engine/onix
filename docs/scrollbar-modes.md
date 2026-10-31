# Scrollbar rendering modes

Native `ui.ScrollArea` and CSS `overflow` containers support overlay scrollbars
and classic scrollbars that reserve content space. `ReserveScrollbarSpace`
selects classic rendering; its zero value keeps overlay rendering.

```go
area := ui.SpawnScrollArea(world, ui.ScrollAreaOptions{
    Width: 120, Height: 80,
    ContentWidth: 120, ContentHeight: 160,
    ReserveScrollbarSpace: true,
})
metrics, _ := ui.GetScrollAreaMetrics(world, area)
// Default 10px native bars leave a 110x70 content viewport.
```

The same option is available on `ui.ScrollArea`, `ui.ListOptions[T]`,
`html.Surface` (from `ui/html`), and `layout.Cache` (from `html/layout`).
HTML selects the scrollbar mode for the surface, independently of CSS
`overflow:auto` / `overflow:scroll` and `scrollbar-width` styling:

```go
surface := html.New(`<div style="width:120px;height:80px;overflow:auto;scrollbar-width:thin"><div style="height:160px"></div></div>`)
surface.ReserveScrollbarSpace = true
```

A vertical gutter reduces the available content width; a horizontal gutter
reduces its height. Layout checks for overflow introduced on the opposite axis
and reflows block, flex, and grid contents using the reduced space. The outer
box retains its specified dimensions. HTML `scrollbar-width:none` reserves no
space and keeps scrolling available. Mode and thickness changes also update
retained scroll state and clamp offsets to the new range.

Content clipping excludes the gutters. Scrollbars remain subject to ancestor
clipping while accepting input in their own gutters. Warm native measurement,
HTML scrollbar synchronization, and retained block reflow allocate no memory.

HTML sizing follows [CSS Overflow §4](https://www.w3.org/TR/css-overflow-3/#scrollbars).
Visual acceptance is manual; tests do not launch the editor.
