package font

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func testColorPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.SetRGBA(x, y, color.RGBA{R: 255, G: 128, A: 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func testColorTables(t *testing.T, format string) colorGlyphTables {
	data := testColorPNG(t)
	put16, put32 := binary.BigEndian.PutUint16, binary.BigEndian.PutUint32
	if format == "sbix" {
		table := make([]byte, 36+len(data))
		put16(table, 1)
		put32(table[4:], 1)
		put32(table[8:], 12)
		strike := table[12:]
		put16(strike, 8)
		put32(strike[4:], 16)
		put32(strike[8:], 16)
		put32(strike[12:], uint32(24+len(data)))
		put16(strike[18:], uint16(65534)) // bottom = -2
		copy(strike[20:], "png ")
		copy(strike[24:], data)
		return colorGlyphTables{sbix: table}
	}
	cblc := make([]byte, 80)
	put16(cblc, 3)
	put32(cblc[4:], 1)
	put32(cblc[8:], 56)
	put32(cblc[12:], 24)
	put32(cblc[16:], 1)
	put16(cblc[48:], 1)
	put16(cblc[50:], 1)
	cblc[52], cblc[53], cblc[54], cblc[55] = 8, 8, 32, 1
	put16(cblc[56:], 1)
	put16(cblc[58:], 1)
	put32(cblc[60:], 8)
	put16(cblc[64:], 1)
	put16(cblc[66:], 17)
	put32(cblc[68:], 4)
	put32(cblc[76:], uint32(9+len(data)))
	cbdt := make([]byte, 13+len(data))
	put16(cbdt, 3)
	copy(cbdt[4:], []byte{8, 8, 0, 6, 8})
	put32(cbdt[9:], uint32(len(data)))
	copy(cbdt[13:], data)
	return colorGlyphTables{cblc: cblc, cbdt: cbdt}
}

func TestColorBitmapGlyphsMeasureRasterizeAndCache(t *testing.T) {
	for _, format := range []string{"sbix", "CBDT"} {
		t.Run(format, func(t *testing.T) {
			face := loadTestFace(t)
			face.color = testColorTables(t, format)
			for _, size := range []float64{4, 8, 13.5} {
				for phase := uint8(0); phase < 4; phase++ {
					bitmap, err := face.RasterizeGlyphAt(1, size, phase, phase)
					if err != nil || bitmap == nil || bitmap.Color == nil || bitmap.Bounds.Empty() {
						t.Fatalf("size=%g phase=%d: %v %v", size, phase, bitmap, err)
					}
					bounds, ok := face.colorBounds(1, size, phase, phase)
					if !ok || bounds != bitmap.Bounds {
						t.Fatal("measurement and rasterization disagree")
					}
					colored := false
					for i := 0; i < len(bitmap.Color.Pix); i += 4 {
						if bitmap.Color.Pix[i] > bitmap.Color.Pix[i+1] && bitmap.Color.Pix[i+3] > 0 {
							colored = true
						}
					}
					if !colored {
						t.Fatal("lost color channels")
					}
					if allocations := testing.AllocsPerRun(50, func() {
						_, _ = face.RasterizeGlyphAt(1, size, phase, phase)
						_, _ = face.colorBounds(1, size, phase, phase)
					}); allocations != 0 {
						t.Fatalf("warm color glyph allocated %g", allocations)
					}
				}
			}
		})
	}
}

func TestInstalledEmojiFontLoadsWithoutOutlines(t *testing.T) {
	installed, err := SystemFonts()
	if err != nil {
		t.Skip(err)
	}
	for _, source := range installed {
		if source.Family != "Noto Color Emoji" && source.Family != "Apple Color Emoji" && source.Family != "Segoe UI Emoji" {
			continue
		}
		face, err := source.Load()
		if err != nil {
			t.Fatal(err)
		}
		defer face.Close()
		scaled, _ := NewFace(face, &FaceOptions{Size: 16})
		for _, r := range "✅❌🙂🎉🚀" {
			id := face.GlyphIndex(r)
			bitmap, err := face.RasterizeGlyph(id, 16)
			if err != nil || id == 0 || bitmap == nil || bitmap.Color == nil {
				t.Fatalf("%s %q: id=%d bitmap=%v err=%v", source.Family, r, id, bitmap, err)
			}
			bounds, advance, ok := scaled.GlyphBounds(r)
			dr, _, _, wantAdvance, wantOK := scaled.Glyph(Point26_6{}, r)
			if bounds != dr || advance != wantAdvance || !ok || !wantOK {
				t.Fatal("emoji bounds differ from rasterization")
			}
		}
		return
	}
	t.Skip("no installed color emoji font")
}

func TestUnicodeSbixFontFallback(t *testing.T) {
	// Exercise loading, coverage discovery, fallback and rasterization together:
	// a bitmap-only font with Unicode-platform cmap records, as used on Apple.
	tables := []struct {
		tag  string
		data []byte
	}{
		{"head", buildTestHeadTable()}, {"hhea", buildTestHheaTable()},
		{"maxp", buildTestMaxpTable()}, {"hmtx", buildTestHmtxTable()},
		{"cmap", testUnicodeEmojiCmap()}, {"sbix", testColorTables(t, "sbix").sbix},
	}
	data := make([]byte, 12+len(tables)*16)
	binary.BigEndian.PutUint32(data, 0x00010000)
	binary.BigEndian.PutUint16(data[4:], uint16(len(tables)))
	for i, table := range tables {
		record := data[12+i*16:]
		copy(record, table.tag)
		binary.BigEndian.PutUint32(record[8:], uint32(len(data)))
		binary.BigEndian.PutUint32(record[12:], uint32(len(table.data)))
		data = append(data, table.data...)
		data = append(data, make([]byte, align4(len(data))-len(data))...)
	}
	path := filepath.Join(t.TempDir(), "unicode-sbix.ttf")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	primary, _ := fallbackFixtureFace(t)
	source := &fallbackSource{font: SystemFont{Path: path}}
	primary.fallback.sources = []*fallbackSource{source}
	t.Cleanup(func() {
		if source.face != nil {
			_ = source.face.Close()
		}
	})
	scaled, _ := NewFace(primary, &FaceOptions{Size: 16})
	for _, r := range "✅❌🙂" {
		owner, id := primary.ResolveGlyph(r)
		if owner == primary || id == 0 {
			t.Fatalf("%U resolved to the primary missing-character glyph", r)
		}
		dr, mask, _, advance, ok := scaled.Glyph(Point26_6{}, r)
		bitmap, colored := mask.(*image.RGBA)
		if !ok || !colored || dr.Empty() || advance == 0 {
			t.Fatalf("%U lost its sbix bitmap or metrics", r)
		}
		pixel := bitmap.RGBAAt(bitmap.Rect.Dx()/2, bitmap.Rect.Dy()/2)
		if pixel.R == 0 || pixel.R <= pixel.G || pixel.A == 0 {
			t.Fatalf("%U lost its bitmap colors: %v", r, pixel)
		}
	}
	if allocations := testing.AllocsPerRun(100, func() {
		for _, r := range "✅❌🙂" {
			scaled.Glyph(Point26_6{}, r)
		}
	}); allocations != 0 {
		t.Fatalf("cached Unicode sbix fallback allocated %g", allocations)
	}
}

func TestLayeredColorGlyphUsesPaletteAndOutlineBounds(t *testing.T) {
	face := loadTestFace(t)
	colr, cpal := make([]byte, 24), make([]byte, 18)
	put16, put32 := binary.BigEndian.PutUint16, binary.BigEndian.PutUint32
	put16(colr[2:], 1)
	put32(colr[4:], 14)
	put32(colr[8:], 20)
	put16(colr[12:], 1)
	put16(colr[14:], 1)
	put16(colr[18:], 1)
	put16(colr[20:], face.GlyphIndex('A'))
	put16(cpal[2:], 1)
	put16(cpal[4:], 1)
	put16(cpal[6:], 1)
	put32(cpal[8:], 14)
	copy(cpal[14:], []byte{0, 128, 255, 255})
	face.color = colorGlyphTables{colr: colr, cpal: cpal}
	bitmap, err := face.RasterizeGlyph(1, 24)
	if err != nil || bitmap == nil || bitmap.Color == nil || bitmap.Bounds.Empty() {
		t.Fatalf("layered glyph: %v %v", bitmap, err)
	}
	colored := false
	for i := 0; i < len(bitmap.Color.Pix); i += 4 {
		if bitmap.Color.Pix[i] > bitmap.Color.Pix[i+1] && bitmap.Color.Pix[i+3] > 0 {
			colored = true
		}
	}
	if !colored {
		t.Fatal("layered glyph lost palette colors")
	}
}

func TestEmojiFormatControlsHaveNoStandaloneGlyph(t *testing.T) {
	face := loadTestFace(t)
	scaled, _ := NewFace(face, &FaceOptions{Size: 16})
	for _, r := range "\ufe0e\ufe0f\u200d\U000e0067\U000e007f" {
		bounds, advance, ok := scaled.GlyphBounds(r)
		dr, _, _, rasterAdvance, rasterOK := scaled.Glyph(Point26_6{}, r)
		if !ok || !rasterOK || !bounds.Empty() || !dr.Empty() || advance != 0 || rasterAdvance != 0 {
			t.Fatalf("%U rendered a standalone glyph", r)
		}
	}
}
