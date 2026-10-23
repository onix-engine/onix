package font

import (
	"image"
	"testing"
)

func TestFontTextEdgesUseOS2Metrics(t *testing.T) {
	face, err := Load("../testdata/fonts/Noto_Sans_SC/static/NotoSansSC-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = face.Close() })
	if face.CapHeight() != 733 || face.XHeight() != 543 {
		t.Fatalf("Noto Sans SC cap-height=%v x-height=%v", face.CapHeight(), face.XHeight())
	}
	if allocs := testing.AllocsPerRun(100, func() {
		_ = face.CapHeight()
		_ = face.XHeight()
	}); allocs != 0 {
		t.Fatalf("text edges allocated %g times", allocs)
	}
	face.capHeight, face.xHeight = 0, 0
	for _, edge := range []struct {
		character rune
		height    int16
	}{{'H', face.CapHeight()}, {'x', face.XHeight()}} {
		bounds, err := face.GlyphBounds(face.GlyphIndex(edge.character))
		if err != nil || edge.height != bounds.YMax {
			t.Fatalf("%q fallback height=%v bounds=%v err=%v", edge.character, edge.height, bounds, err)
		}
	}
}

func TestGlyphBoundsMatchRasterization(t *testing.T) {
	cff, err := Parse(buildTestOTFCFF())
	if err != nil {
		t.Fatal(err)
	}
	defer cff.Close()
	for _, face := range []*Face{loadTestFace(t), cff} {
		for _, dpi := range []float64{72, 96, 117} {
			scaled, err := NewFace(face, &FaceOptions{Size: 15.5, DPI: dpi})
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range " AjfyéÅ中。\u0301" {
				bounds, advance, ok := scaled.GlyphBounds(r)
				dr, _, _, rasterAdvance, rasterOK := scaled.Glyph(Point26_6{}, r)
				if bounds != dr || advance != rasterAdvance || ok != rasterOK {
					t.Fatalf("%q dpi=%v: bounds=%v advance=%v ok=%v; raster=%v %v %v", r, dpi, bounds, advance, ok, dr, rasterAdvance, rasterOK)
				}
			}
		}
	}
}

func TestTrueTypeGlyphBoundsDoNotCacheOutlinesOrBitmaps(t *testing.T) {
	face := loadTestFace(t)
	scaled, _ := NewFace(face, &FaceOptions{Size: 16})
	if allocs := testing.AllocsPerRun(100, func() {
		for _, r := range "AjfyéÅ中。 " {
			if _, _, ok := scaled.GlyphBounds(r); !ok {
				panic("missing bounds")
			}
		}
	}); allocs != 0 {
		t.Fatalf("bounds allocated %g times", allocs)
	}
	if len(face.outlineCache) != 0 || len(face.glyphBitmapCache) != 0 {
		t.Fatal("bounds measurement populated contour or bitmap caches")
	}
}

func TestGlyphBoundsRejectInvalidOffsets(t *testing.T) {
	for _, face := range []*Face{
		{Loca: []uint32{2, 1}, Glyf: make([]byte, 10)},
		{Loca: []uint32{0, 11}, Glyf: make([]byte, 10)},
		{Loca: []uint32{0, 9}, Glyf: make([]byte, 10)},
		{},
	} {
		if _, err := face.GlyphBounds(0); err == nil {
			t.Fatal("invalid glyph bounds accepted")
		}
	}
	var scaled *ScaledFace
	if bounds, advance, ok := scaled.GlyphBounds('A'); bounds != (image.Rectangle{}) || advance != 0 || ok {
		t.Fatal("nil face returned glyph bounds")
	}
}
