package text

import (
	"bytes"
	"image"
	"runtime"
	"testing"

	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

func memoryTestFont(t *testing.T) *Font {
	t.Helper()
	face, err := font.Load("../testdata/fonts/Noto_Sans_SC/static/NotoSansSC-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	value := NewFont(face)
	t.Cleanup(func() {
		if err := value.Dispose(); err != nil {
			t.Fatal(err)
		}
	})
	return value
}

func TestFontSizeCacheBudgetPreservesLiveLayouts(t *testing.T) {
	value := memoryTestFont(t)
	first := value.GlyphCache(16, 72)
	glyph := first.Glyph('A')
	for size := 1; size < glyphSizeCacheLimit; size++ {
		value.GlyphCache(float64(100+size), 72)
	}
	hot := value.GlyphCache(101, 72)
	value.GlyphCache(200, 72)
	if len(value.caches) != glyphSizeCacheLimit || value.caches[glyphCacheKey{16, 72}] != nil || value.GlyphCache(101, 72) != hot {
		t.Fatal("size cache did not evict the least recently used entry")
	}
	if first.Glyph('A') != glyph || !glyph.page.texture.Valid() {
		t.Fatal("size eviction invalidated an existing layout")
	}
	if value.GlyphCache(16, 72) == first {
		t.Fatal("evicted size remained in the font cache")
	}
	if allocs := testing.AllocsPerRun(100, func() { value.GlyphCache(16, 72) }); allocs != 0 {
		t.Fatalf("warm size lookup allocated %g times", allocs)
	}
}

func TestGlyphRecordBudgetIncludesPhasesAndOutlines(t *testing.T) {
	cache := NewGlyphCache(nil, 0, 0)
	t.Cleanup(func() { _ = cache.Dispose() })
	for character := rune(0); character < glyphCacheEntryLimit*2; character++ {
		cache.Glyph(character)
		cache.GlyphGrayAt(character, 1, 0, font.LCDColor(character&511))
		cache.OutlineGlyph(character, 1)
		entries := len(cache.glyphs) + len(cache.phasedGlyphs) + len(cache.outlinedGlyphs)
		if entries > glyphCacheEntryLimit || entries != cache.entries {
			t.Fatalf("glyph records = %d (tracked %d), limit %d", entries, cache.entries, glyphCacheEntryLimit)
		}
	}
	if allocs := testing.AllocsPerRun(100, func() {
		cache.Glyph(glyphCacheEntryLimit*2 - 1)
		cache.GlyphGrayAt(glyphCacheEntryLimit*2-1, 1, 0, 511)
		cache.OutlineGlyph(glyphCacheEntryLimit*2-1, 1)
	}); allocs != 0 {
		t.Fatalf("warm bounded glyph lookup allocated %g times", allocs)
	}
}

func TestGlyphClockEvictsColdRecordsWithoutClearingHotEntries(t *testing.T) {
	cache := NewGlyphCache(nil, 0, 0)
	t.Cleanup(func() { _ = cache.Dispose() })
	for i := rune(0); i < glyphCacheEntryLimit; i++ {
		cache.Glyph(i)
	}
	cache.Glyph(glyphCacheEntryLimit)
	if len(cache.glyphs) != glyphCacheEntryLimit {
		t.Fatal("overflow cleared the cache")
	}
	for i := rune(1); i <= 128; i++ {
		cache.Glyph(1)
		cache.Glyph(glyphCacheEntryLimit + i)
		if _, ok := cache.glyphs[1]; !ok {
			t.Fatal("hot glyph was evicted before cold entries")
		}
	}
	if n := testing.AllocsPerRun(100, func() { cache.Glyph(1) }); n != 0 {
		t.Fatalf("hot lookup allocated %g", n)
	}
}

func TestGlyphAtlasReclamationBudget(t *testing.T) {
	cache := NewGlyphCache(nil, 0, 0)
	t.Cleanup(func() { _ = cache.Dispose() })
	cache.pool.byteLimit = defaultGlyphAtlasSize * defaultGlyphAtlasSize
	bitmap := image.NewAlpha(image.Rect(0, 0, 300, 300))
	for range 40 {
		cache.allocate(bitmap)
	}
	cache.FlushUploads()
	runtime.GC()
	before := cache.pool.pages
	cache.FlushUploads()
	if released := before - cache.pool.pages; released > 8 {
		t.Fatalf("one sweep released %d pages", released)
	}
	for range 10 {
		cache.FlushUploads()
	}
	if cache.pool.pages != 1 {
		t.Fatalf("incremental sweep retained %d pages", cache.pool.pages)
	}
}

func fillMemoryTestPages(cache *GlyphCache) {
	bitmap := image.NewAlpha(image.Rect(0, 0, 300, 300))
	for range 3 {
		cache.allocate(bitmap)
	}
}

func TestGlyphAtlasBudgetPreservesPinnedPages(t *testing.T) {
	value := memoryTestFont(t)
	cache := value.GlyphCache(16, 72)
	cache.pool.byteLimit = defaultGlyphAtlasSize * defaultGlyphAtlasSize
	glyph := cache.Glyph('A')
	texture := glyph.page.texture
	fillMemoryTestPages(cache)
	if glyph.page.cached || cache.pool.cachedBytes > cache.pool.byteLimit {
		t.Fatal("cold atlas page was not removed from the cache budget")
	}
	runtime.GC()
	cache.FlushUploads()
	if got, ok := cache.Atlas(glyph.Atlas); !ok || got != texture || !got.Valid() || cache.Glyph('A') != glyph {
		t.Fatal("cache eviction invalidated a glyph still owned by text")
	}
	runtime.KeepAlive(glyph)
}

func TestGlyphAtlasReclaimsColdPagesAndRegeneratesGlyphs(t *testing.T) {
	value := memoryTestFont(t)
	cache := value.GlyphCache(16, 72)
	cache.pool.byteLimit = defaultGlyphAtlasSize * defaultGlyphAtlasSize
	original, texture := func() (Glyph, *graphics.Texture) {
		glyph := cache.Glyph('A')
		return cache.glyphs['A'].glyph, glyph.page.texture
	}()
	bitmap, err := value.RasterizeGlyph(value.GlyphIndex('A'), 16)
	if err != nil {
		t.Fatal(err)
	}
	want := append([]byte(nil), glyphBitmap(bitmap.Image, image.Point{}, bitmap.Image.Bounds()).Pix...)
	fillMemoryTestPages(cache)
	runtime.GC()
	cache.FlushUploads()
	if texture.Valid() || cache.pool.pages != 1 {
		t.Fatalf("cold pages retained resources: valid=%v pages=%d", texture.Valid(), cache.pool.pages)
	}
	glyph := cache.Glyph('A')
	if glyph.Width != original.Width || glyph.Height != original.Height || glyph.OffsetX != original.OffsetX || glyph.OffsetY != original.OffsetY || glyph.Advance != original.Advance {
		t.Fatal("regenerated glyph changed layout geometry")
	}
	updates := glyph.page.pending
	if len(updates) == 0 || !bytes.Equal(updates[len(updates)-1].Pixels, want) {
		t.Fatal("regenerated glyph bitmap differs from the original")
	}
	page, _, _ := cache.allocate(image.NewAlpha(image.Rect(0, 0, 300, 300)))
	if page.index >= 3 || len(cache.pool.atlases) > 3 {
		t.Fatal("reclaimed atlas slots were not reused")
	}
}

func TestGlyphAtlasLifetimeIncludesPendingCanvasDraws(t *testing.T) {
	driver := &glyphUploadDriver{}
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	value := memoryTestFont(t)
	cache := value.GlyphCache(16, 72)
	cache.pool.byteLimit = defaultGlyphAtlasSize * defaultGlyphAtlasSize
	canvas := graphics.NewCanvas2D(render.ActiveDevice(), 100, 100)
	texture := func() *graphics.Texture {
		glyph := cache.Glyph('A')
		cache.FlushUploads()
		glyph.page.texture.RenderHandle()
		canvas.BindTexture(glyph.page.texture)
		canvas.DrawMatrices(mgl32.Ident4(), mgl32.Ident3(), 1)
		glyph.KeepAlive(canvas)
		return glyph.page.texture
	}()
	fillMemoryTestPages(cache)
	runtime.GC()
	cache.FlushUploads()
	if !texture.Valid() || driver.destroyCalls != 0 {
		t.Fatal("queued sprite lost its atlas before submission")
	}
	if err := canvas.FinishFrame(); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	cache.FlushUploads()
	if texture.Valid() || driver.destroyCalls != 1 {
		t.Fatal("submitted sprite still pinned its cold atlas")
	}
}

func TestGlyphAtlasBudgetCountsLCDChannelsAndOversizedPages(t *testing.T) {
	cache := NewGlyphCache(nil, 0, 0)
	t.Cleanup(func() { _ = cache.Dispose() })
	cache.pool.byteLimit = 4 * defaultGlyphAtlasSize * defaultGlyphAtlasSize
	gray, _, _ := cache.allocate(image.NewAlpha(image.Rect(0, 0, 300, 300)))
	lcd, _, _ := cache.allocateFormat(image.NewRGBA(image.Rect(0, 0, 300, 300)), true)
	if gray.cached || !lcd.cached || cache.pool.cachedBytes != cache.pool.byteLimit {
		t.Fatal("LCD atlas bytes were not counted at four bytes per pixel")
	}
	large, _, _ := cache.allocate(image.NewAlpha(image.Rect(0, 0, 1100, 1)))
	if large.cached || !lcd.cached || cache.pool.cachedBytes != cache.pool.byteLimit || !large.texture.Valid() {
		t.Fatal("oversized atlas was cached or lost its returned page")
	}
	runtime.KeepAlive(gray)
	runtime.KeepAlive(lcd)
	runtime.KeepAlive(large)
}
