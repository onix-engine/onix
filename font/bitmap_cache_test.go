package font

import (
	"bytes"
	"image"
	"testing"
)

func TestGlyphBitmapCacheBudgets(t *testing.T) {
	face := &Face{}
	face.ensureCachesLocked()
	bitmap := &GlyphBitmap{Image: image.NewAlpha(image.Rect(0, 0, 1024, 1024))}
	bitmap.Image.Pix[0] = 42
	for i := range 12 {
		face.cacheBitmap(glyphBitmapCacheKey{glyphID: uint16(i)}, bitmap)
		if face.bitmapBytes > glyphBitmapCacheByteLimit || len(face.glyphBitmapCache) > glyphBitmapCacheEntryLimit {
			t.Fatal("bitmap bytes exceeded the budget")
		}
	}
	if face.glyphBitmapCache[glyphBitmapCacheKey{}] != nil || bitmap.Image.Pix[0] != 42 {
		t.Fatal("eviction retained oldest bitmap or modified a returned image")
	}
	large := &GlyphBitmap{Image: image.NewAlpha(image.Rect(0, 0, glyphBitmapCacheByteLimit+1, 1))}
	if face.cacheBitmap(glyphBitmapCacheKey{glyphID: 99}, large) != large || len(face.glyphBitmapCache) != 4 {
		t.Fatal("oversized bitmap was cached or not returned")
	}
	empty := &GlyphBitmap{Image: image.NewAlpha(image.Rectangle{})}
	for i := range glyphBitmapCacheEntryLimit * 2 {
		face.cacheBitmap(glyphBitmapCacheKey{sizeBits: uint64(i + 1)}, empty)
	}
	if len(face.glyphBitmapCache) != glyphBitmapCacheEntryLimit || face.bitmapBytes != 0 {
		t.Fatal("zero-area bitmaps bypassed the entry budget")
	}
	key := glyphBitmapCacheKey{sizeBits: glyphBitmapCacheEntryLimit * 2}
	if allocs := testing.AllocsPerRun(100, func() { face.cacheBitmap(key, empty) }); allocs != 0 {
		t.Fatalf("cache hit allocated %g times", allocs)
	}
	face.Close()
	if face.bitmapBytes != 0 || face.bitmapOrder != nil || len(face.glyphBitmapCache) != 0 {
		t.Fatal("close retained bitmap storage")
	}
}

func TestGlyphBitmapEvictionPreservesRasterization(t *testing.T) {
	face := loadTestFace(t)
	glyph := face.GlyphIndex('A')
	first, err := face.RasterizeGlyphAt(glyph, 20, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	// Fill with empty entries to exercise real eviction without expensive
	// rasterization of thousands of synthetic sizes.
	face.cacheMu.Lock()
	empty := &GlyphBitmap{Image: image.NewAlpha(image.Rectangle{})}
	for i := range glyphBitmapCacheEntryLimit {
		face.cacheBitmap(glyphBitmapCacheKey{sizeBits: uint64(i + 1)}, empty)
	}
	face.cacheMu.Unlock()
	second, err := face.RasterizeGlyphAt(glyph, 20, 1, 2)
	if err != nil || first == second || first.Bounds != second.Bounds || first.Advance != second.Advance || !bytes.Equal(first.Image.Pix, second.Image.Pix) {
		t.Fatal("regenerated bitmap differs from the evicted bitmap")
	}
}
