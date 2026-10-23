package font

import (
	"runtime"
	"slices"
	"testing"
	"weak"
)

func cachedOutlineFixture(t *testing.T, face *Face, id uint16) (Outline, weak.Pointer[Outline]) {
	t.Helper()
	outline, err := face.Outline(id)
	if err != nil {
		t.Fatal(err)
	}
	expected := Outline{Bounds: outline.Bounds, Contours: slices.Clone(outline.Contours)}
	for index, contour := range expected.Contours {
		expected.Contours[index] = slices.Clone(contour)
	}
	if allocations := testing.AllocsPerRun(50, func() {
		cached, err := face.Outline(id)
		if err != nil || cached != outline {
			t.Fatal("live outline was not shared")
		}
	}); allocations != 0 {
		t.Fatalf("live outline lookup allocated %g times", allocations)
	}
	return expected, weak.Make(outline)
}

func TestOutlineCacheReclaimsContoursAndRebuilds(t *testing.T) {
	cff, err := Parse(buildTestOTFCFF())
	if err != nil {
		t.Fatal(err)
	}
	defer cff.Close()
	for _, face := range []*Face{loadTestFace(t), cff} {
		for _, character := range "AéÅ中" {
			id := face.GlyphIndex(character)
			expected, reference := cachedOutlineFixture(t, face, id)
			runtime.GC()
			if reference.Value() != nil {
				t.Fatal("face pinned an unused outline after GC")
			}
			actual, err := face.Outline(id)
			if err != nil || actual.Bounds != expected.Bounds || len(actual.Contours) != len(expected.Contours) {
				t.Fatalf("rebuilding %q changed outline bounds: %v", character, err)
			}
			for index, contour := range actual.Contours {
				if !slices.Equal(contour, expected.Contours[index]) {
					t.Fatalf("rebuilding %q changed contour %d", character, index)
				}
			}
		}
	}
}

func TestCollectedOutlineKeepsWarmGlyphBitmap(t *testing.T) {
	face := loadTestFace(t)
	id := face.GlyphIndex('A')
	bitmap, err := face.RasterizeGlyph(id, 18)
	if err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	if face.outlineCache[id].Value() != nil {
		t.Fatal("rasterized mask retained source contours")
	}
	if allocations := testing.AllocsPerRun(50, func() {
		cached, err := face.RasterizeGlyph(id, 18)
		if err != nil || cached != bitmap {
			t.Fatal("collecting outlines invalidated a warm bitmap")
		}
	}); allocations != 0 {
		t.Fatalf("warm bitmap lookup allocated %g times", allocations)
	}
}
