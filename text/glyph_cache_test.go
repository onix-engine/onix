package text

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
)

func TestGlyphBitmapPreservesAntialiasedMaskEdge(t *testing.T) {
	mask := image.NewAlpha(image.Rect(0, 0, 3, 1))
	mask.SetAlpha(0, 0, color.Alpha{A: 0})
	mask.SetAlpha(1, 0, color.Alpha{A: 128})
	mask.SetAlpha(2, 0, color.Alpha{A: 255})

	bitmap := glyphBitmap(mask, image.Point{}, mask.Bounds())
	if got, want := bitmap.Bounds().Dx(), mask.Bounds().Dx()+glyphBitmapPadding*2; got != want {
		t.Fatalf("expected padded bitmap width %d, got %d", want, got)
	}
	if got, want := bitmap.Bounds().Dy(), mask.Bounds().Dy()+glyphBitmapPadding*2; got != want {
		t.Fatalf("expected padded bitmap height %d, got %d", want, got)
	}
	if coverage := bitmap.AlphaAt(0, 0).A; coverage != 0 {
		t.Fatalf("transparent glyph padding coverage = %d, want zero", coverage)
	}

	got := bitmap.AlphaAt(1+glyphBitmapPadding, glyphBitmapPadding).A
	if got < 120 || got > 136 {
		t.Fatalf("expected antialiased mask edge to be preserved, got alpha %d", got)
	}
}

func TestDilateGlyphBitmapExpandsMaskWithAntialiasedEdge(t *testing.T) {
	bitmap := image.NewRGBA(image.Rect(0, 0, 9, 9))
	for offset := 0; offset < len(bitmap.Pix); offset += 4 {
		bitmap.Pix[offset+0] = 255
		bitmap.Pix[offset+1] = 255
		bitmap.Pix[offset+2] = 255
	}
	bitmap.Pix[bitmap.PixOffset(4, 4)+3] = 255

	dilateGlyphBitmap(bitmap, 2)

	if alpha := bitmap.Pix[bitmap.PixOffset(6, 4)+3]; alpha != 255 {
		t.Fatalf("outline alpha at radius = %d, want 255", alpha)
	}
	if alpha := bitmap.Pix[bitmap.PixOffset(6, 6)+3]; alpha == 0 || alpha == 255 {
		t.Fatalf("diagonal outline edge alpha = %d, want antialiased coverage", alpha)
	}
	if alpha := bitmap.Pix[bitmap.PixOffset(8, 4)+3]; alpha != 0 {
		t.Fatalf("pixel outside outline radius has alpha %d", alpha)
	}
}

func TestDilateGlyphBitmapMatchesPointKernel(t *testing.T) {
	for _, width := range []float32{0.5, 1, 2, 3.25} {
		got := image.NewRGBA(image.Rect(0, 0, 17, 13))
		for y := range got.Bounds().Dy() {
			for x := range got.Bounds().Dx() {
				offset := got.PixOffset(x, y)
				got.Pix[offset], got.Pix[offset+1], got.Pix[offset+2] = 255, 255, 255
				if (x*7+y*11)%13 == 0 {
					got.Pix[offset+3] = uint8((x*31 + y*17) % 256)
				}
			}
		}
		want := image.NewRGBA(got.Bounds())
		copy(want.Pix, got.Pix)
		dilateGlyphBitmap(got, width)
		dilateGlyphBitmapByPointKernel(want, width)
		if !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("width %v dilation differs", width)
		}
	}
}

func dilateGlyphBitmapByPointKernel(bitmap *image.RGBA, width float32) {
	bounds := bitmap.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	source := make([]uint8, w*h)
	for y := range h {
		for x := range w {
			source[y*w+x] = bitmap.Pix[bitmap.PixOffset(x, y)+3]
		}
	}
	radius := int(math.Ceil(float64(width + 1)))
	for y := range h {
		for x := range w {
			alpha := source[y*w+x]
			if alpha == 0 {
				continue
			}
			for dy := -radius; dy <= radius; dy++ {
				targetY := y + dy
				if targetY < 0 || targetY >= h {
					continue
				}
				for dx := -radius; dx <= radius; dx++ {
					targetX := x + dx
					if targetX < 0 || targetX >= w {
						continue
					}
					coverage := min(max(width+1-float32(math.Hypot(float64(dx), float64(dy))), 0), 1)
					if coverage == 0 {
						continue
					}
					value := uint8(float32(alpha) * coverage)
					offset := bitmap.PixOffset(targetX, targetY) + 3
					bitmap.Pix[offset] = max(bitmap.Pix[offset], value)
				}
			}
		}
	}
}

func BenchmarkDilateGlyphBitmap(b *testing.B) {
	source := image.NewRGBA(image.Rect(0, 0, 48, 48))
	for y := 8; y < 40; y++ {
		for x := 8; x < 40; x++ {
			if (x+y)%3 == 0 {
				source.Pix[source.PixOffset(x, y)+3] = 255
			}
		}
	}
	bitmap := image.NewRGBA(source.Bounds())
	b.ReportAllocs()
	b.SetBytes(int64(len(bitmap.Pix)))
	b.ResetTimer()
	for range b.N {
		copy(bitmap.Pix, source.Pix)
		dilateGlyphBitmap(bitmap, 2)
	}
}

func TestGlyphAtlasUsesFixedPages(t *testing.T) {
	cache := NewGlyphCache(nil, 0, 0)
	bitmap := image.NewRGBA(image.Rect(0, 0, 300, 300))
	for want := range 2 {
		page, _, _ := cache.allocate(bitmap)
		if page.index != want {
			t.Fatalf("atlas page = %d, want %d", page.index, want)
		}
	}
	if got := cache.AtlasCount(); got != 2 {
		t.Fatalf("expected two fixed atlas pages, got %d", got)
	}
	for _, slot := range cache.pool.atlases {
		atlas := slot.page.Value()
		if got := atlas.size; got != 512 {
			t.Fatalf("expected 512px atlas page, got %d", got)
		}
		if atlas.texture.Width != atlas.size || atlas.texture.Height != atlas.size {
			t.Fatalf("atlas texture size = %dx%d, want %dx%d", atlas.texture.Width, atlas.texture.Height, atlas.size, atlas.size)
		}
		if atlas.texture.Format() != render.TextureFormatR8 {
			t.Fatalf("atlas texture format = %v, want R8", atlas.texture.Format())
		}
	}
}

func TestFontSizesShareAtlasPages(t *testing.T) {
	value := NewFont(&font.Face{})
	first := value.GlyphCache(14, 72)
	second := value.GlyphCache(28, 72)
	firstPage, _, _ := first.allocate(image.NewRGBA(image.Rect(0, 0, 20, 20)))
	secondPage, _, _ := second.allocate(image.NewRGBA(image.Rect(0, 0, 40, 40)))
	firstTexture, firstOK := first.Atlas(firstPage.index)
	secondTexture, secondOK := second.Atlas(secondPage.index)
	if !firstOK || !secondOK || firstTexture != secondTexture || first.pool != second.pool {
		t.Fatalf("font-size atlas pages were not shared: first=%p second=%p", firstTexture, secondTexture)
	}
	t.Cleanup(func() { _ = value.Dispose() })
}

func TestNextPowerOfTwo(t *testing.T) {
	for value, want := range map[int]int{0: 1, 1: 1, 2: 2, 3: 4, 512: 512, 513: 1024} {
		if got := nextPowerOfTwo(value); got != want {
			t.Fatalf("nextPowerOfTwo(%d) = %d, want %d", value, got, want)
		}
	}
}

func TestGlyphAtlasFlushesPendingRegionsAsOneBatch(t *testing.T) {
	driver := &glyphUploadDriver{}
	if err := render.Install(driver); err != nil {
		t.Fatalf("install render driver: %v", err)
	}
	t.Cleanup(render.Uninstall)
	atlas := newGlyphAtlas(32)
	atlas.texture.RenderHandle()
	atlas.upload(0, 0, image.NewRGBA(image.Rect(0, 0, 4, 5)))
	atlas.upload(8, 0, image.NewRGBA(image.Rect(0, 0, 3, 6)))

	atlas.flushUploads()

	if driver.updateCalls != 1 || driver.updateCount != 2 {
		t.Fatalf("glyph uploads = %d calls / %d regions, want 1 / 2", driver.updateCalls, driver.updateCount)
	}
}

func TestGlyphAtlasSurvivesRenderDeviceReplacement(t *testing.T) {
	first := &glyphUploadDriver{}
	if err := render.Install(first); err != nil {
		t.Fatalf("install first render driver: %v", err)
	}
	t.Cleanup(render.Uninstall)
	atlas := newGlyphAtlas(32)
	bitmap := image.NewRGBA(image.Rect(0, 0, 2, 2))
	bitmap.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	atlas.upload(3, 4, bitmap)
	atlas.flushUploads()

	second := &glyphUploadDriver{}
	if err := render.Install(second); err != nil {
		t.Fatalf("install replacement render driver: %v", err)
	}
	if handle := atlas.texture.RenderHandle(); !handle.Valid() {
		t.Fatal("expected glyph atlas to upload to the replacement render device")
	}
	if second.createCalls != 1 {
		t.Fatalf("replacement device texture creates = %d, want 1", second.createCalls)
	}
	offset := 4*32 + 3
	if len(second.createdPixels) <= offset || second.createdPixels[offset] != 255 {
		t.Fatal("replacement device did not receive the cached glyph pixels")
	}
}

func TestFontDisposeEvictsGlyphCachesAndDisposesAtlases(t *testing.T) {
	value := NewFont(&font.Face{})
	first := value.GlyphCache(14, 72)
	second := value.GlyphCache(18, 72)
	first.allocate(image.NewAlpha(image.Rect(0, 0, 300, 300)))
	second.allocate(image.NewAlpha(image.Rect(0, 0, 300, 300)))
	textures := []*graphics.Texture{first.pool.atlases[0].texture, first.pool.atlases[1].texture}
	if first.pool != second.pool {
		t.Fatal("font glyph caches did not share their atlas pool")
	}

	if err := value.Dispose(); err != nil {
		t.Fatalf("dispose font: %v", err)
	}
	for index, texture := range textures {
		if texture.Valid() {
			t.Fatalf("glyph atlas %d remains valid after font disposal", index)
		}
	}
	if first.AtlasCount() != 0 || second.AtlasCount() != 0 {
		t.Fatalf("disposed glyph caches retained atlases: %d/%d", first.AtlasCount(), second.AtlasCount())
	}
	if replacement := value.GlyphCache(14, 72); replacement == first {
		t.Fatal("disposed font returned its evicted glyph cache")
	}
	if err := value.Dispose(); err != nil {
		t.Fatalf("dispose font twice: %v", err)
	}
}

type glyphUploadDriver struct {
	next          uint32
	createCalls   int
	createdPixels []byte
	updateCalls   int
	updateCount   int
	destroyCalls  int
}

func (d *glyphUploadDriver) RendererInfo() render.RendererInfo { return render.RendererInfo{} }
func (d *glyphUploadDriver) BeginFrame(render.Color) error     { return nil }
func (d *glyphUploadDriver) SetViewport(render.Rect)           {}
func (d *glyphUploadDriver) Viewport() render.Rect             { return render.Rect{} }
func (d *glyphUploadDriver) SetBlend(render.BlendState) error  { return nil }
func (d *glyphUploadDriver) SetScissor(*render.Rect)           {}
func (d *glyphUploadDriver) CreateTexture(descriptor render.TextureDescriptor, pixels []byte) (render.TextureHandle, error) {
	if err := descriptor.Validate(pixels); err != nil {
		return render.TextureHandle{}, err
	}
	d.next++
	d.createCalls++
	d.createdPixels = append(d.createdPixels[:0], pixels...)
	return render.NewHandle[render.TextureResource](d.next, 1), nil
}
func (d *glyphUploadDriver) UpdateTextures(updates []render.TextureUpdate) error {
	d.updateCalls++
	d.updateCount += len(updates)
	return nil
}
func (d *glyphUploadDriver) DestroyTexture(render.TextureHandle) error {
	d.destroyCalls++
	return nil
}
func (d *glyphUploadDriver) DrawSprites(render.SpriteBatch) error { return nil }
func (d *glyphUploadDriver) DrawShapes(render.ShapeBatch) error   { return nil }
func (d *glyphUploadDriver) Present() error                       { return nil }
func (d *glyphUploadDriver) ResizeSurface(int, int) error         { return nil }
func (d *glyphUploadDriver) SetVSync(bool)                        {}
func (d *glyphUploadDriver) Destroy()                             {}

func TestLCDGlyphCacheSeparatesFormatsAndAllocatesNothingWarm(t *testing.T) {
	face, err := font.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	defer face.Close()
	cache := NewGlyphCache(NewFont(face), 16, 72)
	defer cache.Dispose()
	gray := cache.GlyphAt('A', 1, 0)
	lcd := cache.GlyphLCDAt('A', 1)
	if gray.Atlas == lcd.Atlas {
		t.Fatal("LCD and grayscale must not share a texture format")
	}
	if gray.Advance != lcd.Advance {
		t.Fatal("LCD changed layout advance")
	}
	if !lcd.page.lcd || gray.page.lcd {
		t.Fatal("incorrect atlas format")
	}
	for phase := uint8(0); phase < 4; phase++ {
		cache.GlyphAt('A', phase, 0)
		cache.GlyphLCDAt('A', phase)
	}
	if n := testing.AllocsPerRun(100, func() {
		for phase := uint8(0); phase < 4; phase++ {
			cache.GlyphAt('A', phase, 0)
			cache.GlyphLCDAt('A', phase)
		}
	}); n != 0 {
		t.Fatalf("warm phased glyph allocations = %g", n)
	}
	if cache.GlyphLCDAt('A', 5) != lcd {
		t.Fatal("phase cache is not bounded to four x positions")
	}
}

func TestLCDGlyphCacheSeparatesForegroundGamma(t *testing.T) {
	face, err := font.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	defer face.Close()
	cache := NewGlyphCache(NewFont(face), 16, 72)
	defer cache.Dispose()
	black := cache.GlyphLCDColorAt('A', 1, font.LCDColorFromRGB(0, 0, 0))
	white := cache.GlyphLCDColorAt('A', 1, font.LCDColorFromRGB(255, 255, 255))
	if black.X == white.X && black.Y == white.Y && black.Atlas == white.Atlas {
		t.Fatal("different foregrounds reused a corrected mask")
	}
	if black.Advance != white.Advance || black.Width != white.Width || black.OffsetX != white.OffsetX {
		t.Fatal("gamma changed glyph geometry")
	}
	if got := cache.GlyphLCDColorAt('A', 1, font.LCDColorFromRGB(30, 30, 30)); got != black {
		t.Fatal("equivalent gamma colors duplicated atlas glyph")
	}
	if n := testing.AllocsPerRun(100, func() { cache.GlyphLCDColorAt('A', 1, 511); cache.GlyphLCDColorAt('A', 1, 0) }); n != 0 {
		t.Fatalf("warm color gamma cache allocations=%g", n)
	}
}

func TestGrayGammaDoesNotMutateSharedRawMask(t *testing.T) {
	face, err := font.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	defer face.Close()
	cache := NewGlyphCache(NewFont(face), 16, 72)
	defer cache.Dispose()
	raw := cache.GlyphAt('A', 1, 0)
	bitmap, err := face.RasterizeGlyphAt(face.GlyphIndex('A'), 16, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), bitmap.Image.Pix...)
	corrected := cache.GlyphGrayAt('A', 1, 0, 511)
	if !bytes.Equal(before, bitmap.Image.Pix) {
		t.Fatal("gamma corrupted the shared raw glyph cache")
	}
	if raw.X == corrected.X && raw.Y == corrected.Y && raw.Atlas == corrected.Atlas {
		t.Fatal("raw and gamma masks share atlas coordinates")
	}
	if n := testing.AllocsPerRun(100, func() { cache.GlyphGrayAt('A', 1, 0, 511) }); n != 0 {
		t.Fatalf("warm gray gamma allocations=%g", n)
	}
}
