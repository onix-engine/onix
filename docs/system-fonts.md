# System default fonts

Text2D, UI controls, HTML and the editor automatically use the system UI font without a configured file path. `text.DefaultFont()` discovers and parses one process-shared instance and returns it without warm allocations. Faces and glyph caches are shared by font/size. The engine owns the default font and face; callers must not close them. Use `font.LoadDefault()` or `font.Load(path)` for independently owned fonts.

No available font returns a cached nil. Explicit fonts override the default. HTML resolution prefers `Surface.Font`, then the World's `*text.Font` resource, then the shared default. Close an owned face only after all users finish. Explicitly packaged fonts belong in `resources/`; system font paths are not written to project JSON.

`font.SystemFonts()` returns a read-only cached installed-font list containing family, path, collection index, weight, italic and monospace flags. Enumeration reads metadata only; `SystemFont.Load()` opens a caller-owned face. TTF, OTF, TTC and OTC are supported. New installations appear after process restart; cached access allocates nothing.

Android parses system font XML, preferring normal weight 400 in the default sans-serif family, retaining filenames and collection indexes across system/product/vendor/data directories. This uses ordinary XML/file access and preserves NDK API 24 support. Apple uses CoreText's `CTFontCreateUIFontForLanguage`, `kCTFontURLAttribute` and PostScript name to identify the real file and collection face. Windows uses font registration and directories. Linux uses fontconfig plus font directories; enumeration includes system and user fonts.

`text.NewFont` enables per-glyph system fallback, including Chinese glyphs missing from a Latin UI font. Variable-axis instantiation and complex-script shaping are not implemented. First parsing, glyph creation and capacity growth allocate; warmed font and glyph lookups do not.

Each `text.Font` retains at most 32 recently requested size/DPI caches. Each size cache holds at most 4096 records across ordinary glyphs, subpixel/color variants and outlines. Existing text can continue using a size cache after it leaves the font's lookup cache.

Font sizes share a page pool with an 8 MiB LRU budget for cached atlas texels: R8 coverage uses one byte per pixel and LCD coverage uses four. This is a cache retention budget, not a hard GPU or process memory cap. Live glyphs, retained render snapshots and pending canvas batches keep their pages alive beyond the budget. CPU texture shadows, pending uploads and driver overhead also consume memory. Oversized pages remain available to their live glyphs without displacing smaller cached pages.

Glyph records hold weak page references; returned glyphs pin their pages. After GC identifies a cold unreferenced page, the next glyph allocation or `GlyphCache.FlushUploads` releases its texture on the owning render thread and reuses its slot. Cached metadata regenerates a collected glyph on demand. `text.Font.Dispose` releases all remaining pages and closes the owned font; call it only after all text users and pending draws finish. Low-level canvas consumers must call `Glyph.KeepAlive(canvas)` after queuing glyph geometry so the page survives until submission.

Cache lifecycle validation passed Linux font/text/graphics/scene/HTML package tests, text race checks, and text/graphics/scene vet. Android x86_64 text package tests passed on `10.0.0.174:16384` with NDK API 24. The 1000-glyph LCD benchmark retains one draw and zero warm allocations; scene glyph records remain 88 bytes on amd64. These checks exercise resource lifetime through test drivers, not Android GPU surface pixels or visual acceptance.

See [implementation validation](ecs-api-implementation.md) for recorded mobile scope. Darwin compilation of shared Apple code is not equivalent to iOS SDK compilation or device validation.

## Examples and commands

```go
label := scene.NewText2D("Hello")
button := ui.SpawnButton(world, ui.ButtonOptions{
    Width: 120, Height: 32, Label: "Hello",
})
page := html.New(`<div>Hello</div>`)
```

```go
face, err := font.Load("resources/fonts/NotoSansSC-Regular.ttf")
if err != nil {
    return err
}
custom := text.NewFont(face)
label.Font = custom
page.Font = custom
// Call custom.Dispose() after all users and pending draws finish.
```
