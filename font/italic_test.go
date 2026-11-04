package font

import (
	"bytes"
	"testing"
)

func TestItalicRasterAndCache(t *testing.T) {
	face := loadTestFace(t)
	italic := face.Italic()
	if italic == face || italic.Italic() != italic || face.Italic() != italic {
		t.Fatal("italic variant is not cached")
	}
	if italic.CacheKey() == face.CacheKey() {
		t.Fatal("italic shares regular raster identity")
	}
	for _, r := range "Hég" {
		id := face.GlyphIndex(r)
		regular, err := face.RasterizeGlyph(id, 40)
		if err != nil {
			t.Fatal(err)
		}
		slanted, err := italic.RasterizeGlyph(id, 40)
		if err != nil {
			t.Fatal(err)
		}
		if regular.Advance != slanted.Advance {
			t.Fatal("synthesis changed advance")
		}
		if regular.Bounds == slanted.Bounds && bytes.Equal(regular.Image.Pix, slanted.Image.Pix) {
			t.Fatalf("%c did not slant", r)
		}
		scaled, _ := NewFace(italic, &FaceOptions{Size: 40})
		bounds, _, ok := scaled.GlyphBounds(r)
		rasterBounds, _, _, _, _ := scaled.Glyph(Point26_6{}, r)
		if !ok || bounds != rasterBounds {
			t.Fatalf("%c bounds %v != raster %v", r, bounds, rasterBounds)
		}
	}
	if allocs := testing.AllocsPerRun(100, func() { face.Italic(); italic.RasterizeGlyph(italic.GlyphIndex('H'), 40) }); allocs != 0 {
		t.Fatalf("warm allocations: %v", allocs)
	}
}
