package font

import (
	"encoding/binary"
	"image"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func loadTestFace(t *testing.T) *Face {
	t.Helper()
	face, err := LoadDefault()
	if err != nil {
		t.Fatalf("load test font: %v", err)
	}
	t.Cleanup(func() {
		if err := face.Close(); err != nil {
			t.Errorf("close test font: %v", err)
		}
	})
	return face
}

func TestFaceCloseReleasesOwnedBackingOnce(t *testing.T) {
	var releases int
	backing := &fontBacking{
		data: []byte{1, 2, 3},
		release: func() error {
			releases++
			return nil
		},
	}
	face := &Face{Data: backing.data}
	face.attachBacking(backing)

	if err := face.Close(); err != nil {
		t.Fatalf("close face: %v", err)
	}
	if err := face.Close(); err != nil {
		t.Fatalf("close face twice: %v", err)
	}
	if releases != 1 {
		t.Fatalf("backing release count = %d, want 1", releases)
	}
	if face.Data != nil || face.SFNT != nil || face.Cmap != nil {
		t.Fatal("closed face retained parsed data")
	}
}

func loadTestOTFFace(t *testing.T) *Face {
	t.Helper()
	path, err := DefaultCFFFontPath()
	if err != nil {
		t.Skipf("skip CFF font test: %v", err)
	}
	face, err := Load(path)
	if err != nil {
		t.Fatalf("load test otf font: %v", err)
	}
	t.Logf("CFF font: %s", path)
	t.Cleanup(func() {
		if err := face.Close(); err != nil {
			t.Errorf("close test otf font: %v", err)
		}
	})
	return face
}

func TestLoadTTF(t *testing.T) {
	face := loadTestFace(t)
	if face.Head.UnitsPerEm == 0 {
		t.Fatal("unitsPerEm = 0")
	}
	if face.Maxp.NumGlyphs == 0 {
		t.Fatal("numGlyphs = 0")
	}
	if face.Hhea.NumberOfHMetrics == 0 {
		t.Fatal("numberOfHMetrics = 0")
	}
}

func TestParseFontCollectionIndex(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "testdata", "fonts", "Noto_Sans_SC", "static", "NotoSansSC-Regular.ttf"))
	if err != nil {
		t.Fatalf("read test font: %v", err)
	}
	sfntData, err := parseSFNT(data, 0)
	if err != nil {
		t.Fatalf("parse test font: %v", err)
	}
	nameTable, err := sfntData.table("name")
	if err != nil {
		t.Fatalf("read test font names: %v", err)
	}
	names := fontNames(nameTable)
	if len(names) == 0 {
		t.Fatal("test font has no names")
	}
	collection := testFontCollection(data, data)
	index, err := CollectionIndex(collection, names[0])
	if err != nil || index != 0 {
		t.Fatalf("find collection face by name: index=%d err=%v", index, err)
	}
	face, err := ParseIndex(collection, 1)
	if err != nil {
		t.Fatalf("parse collection face: %v", err)
	}
	if face.Maxp.NumGlyphs == 0 {
		t.Fatal("expected second collection face to be parsed")
	}
	if _, err := ParseIndex(collection, 2); err == nil {
		t.Fatal("expected out-of-range collection index error")
	}
}

func testFontCollection(fonts ...[]byte) []byte {
	headerSize := 12 + len(fonts)*4
	offsets := make([]int, len(fonts))
	size := headerSize
	for index, data := range fonts {
		size = (size + 3) &^ 3
		offsets[index] = size
		size += len(data)
	}
	collection := make([]byte, size)
	copy(collection, "ttcf")
	binary.BigEndian.PutUint32(collection[4:8], 0x00010000)
	binary.BigEndian.PutUint32(collection[8:12], uint32(len(fonts)))
	for index, data := range fonts {
		offset := offsets[index]
		binary.BigEndian.PutUint32(collection[12+index*4:16+index*4], uint32(offset))
		copy(collection[offset:], data)
		numTables := int(binary.BigEndian.Uint16(data[4:6]))
		for tableIndex := range numTables {
			recordOffset := offset + 12 + tableIndex*16
			tableOffset := binary.BigEndian.Uint32(collection[recordOffset+8 : recordOffset+12])
			binary.BigEndian.PutUint32(collection[recordOffset+8:recordOffset+12], tableOffset+uint32(offset))
		}
	}
	return collection
}

func TestGlyphMappingAndMetrics(t *testing.T) {
	face := loadTestFace(t)
	glyphID := face.GlyphIndex('A')
	if glyphID == 0 {
		t.Fatal("glyph index for 'A' = 0")
	}

	metrics, err := face.HorizontalMetrics(glyphID)
	if err != nil {
		t.Fatalf("horizontal metrics: %v", err)
	}
	if metrics.AdvanceWidth == 0 {
		t.Fatal("advance width = 0")
	}
}

func TestExtractSimpleGlyph(t *testing.T) {
	face := loadTestFace(t)
	if face.CFF != nil {
		t.Skip("default system font is CFF-based")
	}
	glyphID := face.GlyphIndex('A')
	glyph, err := face.Glyph(glyphID)
	if err != nil {
		t.Fatalf("glyph: %v", err)
	}
	if glyph.Simple == nil {
		t.Fatalf("glyph 'A' is not simple: %#v", glyph)
	}
	if len(glyph.Simple.Points) == 0 {
		t.Fatal("simple glyph has no points")
	}
	if len(glyph.Simple.EndPtsOfContours) == 0 {
		t.Fatal("simple glyph has no contours")
	}
}

func TestExtractCompositeGlyph(t *testing.T) {
	face := loadTestFace(t)
	if face.CFF != nil {
		t.Skip("default system font is CFF-based")
	}
	for _, r := range []rune{'Å', 'Ä', 'é', 'ç', 'Ǻ'} {
		glyphID := face.GlyphIndex(r)
		if glyphID == 0 {
			continue
		}
		glyph, err := face.Glyph(glyphID)
		if err != nil {
			t.Fatalf("glyph %q: %v", r, err)
		}
		if glyph.Composite != nil {
			if len(glyph.Composite.Components) == 0 {
				t.Fatalf("composite glyph %q has no components", r)
			}
			return
		}
	}
	t.Fatal("no composite glyph found in candidate set")
}

func TestOutlineExtraction(t *testing.T) {
	face := loadTestFace(t)
	outline, err := face.Outline(face.GlyphIndex('A'))
	if err != nil {
		t.Fatalf("outline: %v", err)
	}
	if len(outline.Contours) == 0 {
		t.Fatal("outline has no contours")
	}
	if len(outline.Contours[0]) < 3 {
		t.Fatalf("first contour too small: %d", len(outline.Contours[0]))
	}
}

func TestFlattenContourHandlesConsecutiveOffCurvePoints(t *testing.T) {
	contour, err := flattenContour([]Point{
		{X: 0, Y: 0, OnCurve: true},
		{X: 10, Y: 0},
		{X: 20, Y: 10},
		{X: 20, Y: 20},
		{X: 0, Y: 20, OnCurve: true},
	}, 0.5)
	if err != nil {
		t.Fatalf("flatten contour: %v", err)
	}
	for _, want := range []OutlinePoint{{X: 15, Y: 5}, {X: 20, Y: 15}} {
		found := false
		for _, point := range contour {
			if point == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("flattened contour does not contain implied point %+v: %#v", want, contour)
		}
	}
}

func TestRasterizeGlyph(t *testing.T) {
	face := loadTestFace(t)
	bitmap, err := face.RasterizeGlyph(face.GlyphIndex('A'), 48)
	if err != nil {
		t.Fatalf("rasterize glyph: %v", err)
	}
	if bitmap == nil || bitmap.Image == nil {
		t.Fatal("bitmap is nil")
	}
	if bitmap.Image.Bounds().Dx() == 0 || bitmap.Image.Bounds().Dy() == 0 {
		t.Fatalf("bitmap bounds = %v", bitmap.Image.Bounds())
	}
	nonZero := 0
	for _, value := range bitmap.Image.Pix {
		if value != 0 {
			nonZero++
		}
	}
	if nonZero == 0 {
		t.Fatal("bitmap has no painted pixels")
	}
}

func TestRasterizeGlyphCachesBitmap(t *testing.T) {
	face := loadTestFace(t)
	glyphID := face.GlyphIndex('A')
	first, err := face.RasterizeGlyph(glyphID, 48)
	if err != nil {
		t.Fatalf("first rasterize glyph: %v", err)
	}
	second, err := face.RasterizeGlyph(glyphID, 48)
	if err != nil {
		t.Fatalf("second rasterize glyph: %v", err)
	}
	if first == nil || second == nil {
		t.Fatal("cached bitmap is nil")
	}
	if first != second {
		t.Fatal("rasterized glyph bitmap was not reused")
	}
}

func TestLoadOTFCFF(t *testing.T) {
	face := loadTestOTFFace(t)
	if face.CFF == nil {
		t.Fatal("face.CFF = nil")
	}
	if face.Maxp.NumGlyphs == 0 {
		t.Fatal("numGlyphs = 0")
	}
	if face.Hhea.NumberOfHMetrics == 0 {
		t.Fatal("numberOfHMetrics = 0")
	}
}

func TestOTFCFFOutlineAndRasterize(t *testing.T) {
	face := loadTestOTFFace(t)
	// System CFF fonts may cover scripts without Latin characters.
	var outline *Outline
	glyphID := uint16(1)
	for ; glyphID < face.Maxp.NumGlyphs; glyphID++ {
		var err error
		outline, err = face.Outline(glyphID)
		if err != nil {
			t.Fatalf("outline glyph %d: %v", glyphID, err)
		}
		if len(outline.Contours) != 0 {
			break
		}
	}
	if outline == nil || len(outline.Contours) == 0 {
		t.Fatal("font has no glyph with contours")
	}

	bitmap, err := face.RasterizeGlyph(glyphID, 48)
	if err != nil {
		t.Fatalf("rasterize glyph: %v", err)
	}
	if bitmap == nil || bitmap.Image == nil {
		t.Fatal("bitmap is nil")
	}
	if bitmap.Image.Bounds().Dx() == 0 || bitmap.Image.Bounds().Dy() == 0 {
		t.Fatalf("bitmap bounds = %v", bitmap.Image.Bounds())
	}
	nonZero := 0
	for _, value := range bitmap.Image.Pix {
		if value != 0 {
			nonZero++
		}
	}
	if nonZero == 0 {
		t.Fatal("bitmap has no painted pixels")
	}
}

func TestRasterizerUsesNonZeroWindingFill(t *testing.T) {
	img := image.NewAlpha(image.Rect(0, 0, 20, 20))
	outer := OutlineContour{{X: 2, Y: 2}, {X: 18, Y: 2}, {X: 18, Y: 18}, {X: 2, Y: 18}, {X: 2, Y: 2}}
	innerSameDirection := OutlineContour{{X: 6, Y: 6}, {X: 14, Y: 6}, {X: 14, Y: 14}, {X: 6, Y: 14}, {X: 6, Y: 6}}

	paintCoverage(img, []OutlineContour{outer, innerSameDirection})
	if got := img.AlphaAt(10, 10).A; got == 0 {
		t.Fatalf("center pixel alpha = %d", got)
	}
}

func TestRasterizerFractionalCoverage(t *testing.T) {
	for _, width := range []float64{0.01, 0.1, 0.3, 0.5, 0.9, 1} {
		img := image.NewAlpha(image.Rect(0, 0, 2, 2))
		paintCoverage(img, []OutlineContour{{{X: 0, Y: 0}, {X: width, Y: 0}, {X: width, Y: 2}, {X: 0, Y: 2}}})
		want := uint8(math.Round(width * 255))
		if got := img.AlphaAt(0, 0).A; math.Abs(float64(got)-float64(want)) > 1 {
			t.Fatalf("width %g: coverage %d, want %d", width, got, want)
		}
		if img.AlphaAt(1, 0).A != 0 {
			t.Fatal("coverage escaped shape")
		}
	}
}

func TestRasterizerTriangleAndHole(t *testing.T) {
	img := image.NewAlpha(image.Rect(0, 0, 4, 4))
	paintCoverage(img, []OutlineContour{{{X: 0, Y: 0}, {X: 4, Y: 0}, {X: 0, Y: 4}}})
	sum := 0
	for _, a := range img.Pix {
		sum += int(a)
	}
	if math.Abs(float64(sum)-8*255) > 2 {
		t.Fatalf("triangle area = %g, want 8", float64(sum)/255)
	}
	paintCoverage(img, []OutlineContour{
		{{X: 0, Y: 0}, {X: 4, Y: 0}, {X: 4, Y: 4}, {X: 0, Y: 4}},
		{{X: 1, Y: 1}, {X: 1, Y: 3}, {X: 3, Y: 3}, {X: 3, Y: 1}},
	})
	if img.AlphaAt(1, 1).A != 0 || img.AlphaAt(0, 0).A != 255 {
		t.Fatal("opposite winding must cut a hole")
	}
}

func TestGlyphFractionalOriginAndWarmCache(t *testing.T) {
	face := loadTestFace(t)
	scaled, err := NewFace(face, &FaceOptions{Size: 16, DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	a, _, _, advance, ok := scaled.Glyph(Point26_6{X: 16, Y: 32}, 'A')
	if !ok {
		t.Fatal("missing glyph")
	}
	b, _, _, nextAdvance, ok := scaled.Glyph(Point26_6{X: 16 - 3*64, Y: 32 + 2*64}, 'A')
	if !ok || b != a.Add(image.Pt(-3, 2)) || advance != nextAdvance {
		t.Fatal("integer translation changed glyph phase or advance")
	}
	if n := testing.AllocsPerRun(100, func() { scaled.Glyph(Point26_6{X: 16, Y: 32}, 'A') }); n != 0 {
		t.Fatalf("warm glyph allocations = %g", n)
	}
	for phase := uint8(0); phase < 4; phase++ {
		_, mask, lcdAdvance, ok := scaled.GlyphLCD('A', phase)
		if !ok || mask == nil || lcdAdvance != advance {
			t.Fatal("LCD metrics differ")
		}
		independent := false
		for i := 0; i < len(mask.Pix); i += 4 {
			if mask.Pix[i+3] != 255 {
				t.Fatal("LCD must preserve opaque destination alpha")
			}
			independent = independent || mask.Pix[i] != mask.Pix[i+2]
		}
		if !independent {
			t.Fatal("LCD channels must sample distinct horizontal areas")
		}
	}
}

func BenchmarkPaintCoverage(b *testing.B) {
	face, err := LoadDefault()
	if err != nil {
		b.Fatal(err)
	}
	outline, err := face.Outline(face.GlyphIndex('A'))
	if err != nil {
		b.Fatal(err)
	}
	const pixelsPerEm = 48
	scale := pixelsPerEm / float64(face.Head.UnitsPerEm)
	bounds := image.Rect(
		int(math.Floor(float64(outline.Bounds.XMin)*scale)),
		int(math.Floor(float64(outline.Bounds.YMin)*scale)),
		int(math.Ceil(float64(outline.Bounds.XMax)*scale)),
		int(math.Ceil(float64(outline.Bounds.YMax)*scale)),
	)
	var scratch glyphRasterScratch
	contours := scratch.scaleOutline(outline.Contours, scale, bounds, 0, 0, 1)
	img := image.NewAlpha(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	b.ReportAllocs()
	b.SetBytes(int64(len(img.Pix)))
	b.ResetTimer()
	for range b.N {
		paintCoverage(img, contours)
	}
}
