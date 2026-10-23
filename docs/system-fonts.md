# System default fonts

Text2D, UI controls, HTML and the editor automatically use the system UI font without a configured file path. `text.DefaultFont()` discovers and parses one process-shared instance and returns it without warm allocations. Faces and glyph caches are shared by font/size. The engine owns the default font and face; callers must not close them. Use `font.LoadDefault()` or `font.Load(path)` for independently owned fonts.

No available font returns a cached nil. Explicit fonts override the default. HTML resolution prefers `Surface.Font`, then the World's `*text.Font` resource, then the shared default. Close an owned face only after all users finish. Explicitly packaged fonts belong in `resources/`; system font paths are not written to project JSON.

`font.SystemFonts()` returns a read-only cached installed-font list containing family, path, collection index, weight, italic and monospace flags. Enumeration reads metadata only; `SystemFont.Load()` opens a caller-owned face. TTF, OTF, TTC and OTC are supported. New installations appear after process restart; cached access allocates nothing.

Android parses system font XML, preferring normal weight 400 in the default sans-serif family, retaining filenames and collection indexes across system/product/vendor/data directories. This uses ordinary XML/file access and preserves NDK API 24 support. Apple uses CoreText's `CTFontCreateUIFontForLanguage`, `kCTFontURLAttribute` and PostScript name to identify the real file and collection face. Windows uses font registration and directories. Linux uses fontconfig plus font directories; enumeration includes system and user fonts.

There is no per-glyph cross-font fallback, variable-axis instantiation or complex-script shaping. Supply a font covering the desired script when the default lacks it; Roboto does not cover all Chinese text. First parsing, glyph creation and capacity growth allocate. Warm default-font lookup is the allocation-free guarantee, not first rendering.

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
// Call face.Close() after all users of the owned font finish.
```
