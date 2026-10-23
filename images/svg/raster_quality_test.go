package svg

import "testing"

func TestFillPreservesNarrowFractionalCoverage(t *testing.T) {
	img, err := RenderString(`<svg width="2" height="2"><rect x="0.01" width="0.02" height="2" fill="white"/></svg>`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// A strip narrower than the old 1/8-pixel horizontal grid still covers 2%.
	assertPixel(t, img, 0, 0, 255, 255, 255, 5)
	assertPixel(t, img, 1, 0, 0, 0, 0, 0)
}

func TestFillFractionalCoverageAcrossPixels(t *testing.T) {
	img, err := RenderString(`<svg width="3" height="1"><rect x="0.2" width="1.5" height="1" fill="white"/></svg>`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, img, 0, 0, 255, 255, 255, 204)
	assertPixel(t, img, 1, 0, 255, 255, 255, 179)
	assertPixel(t, img, 2, 0, 0, 0, 0, 0)
}

func TestOffscreenShapeDoesNotAllocateNegativeRaster(t *testing.T) {
	img, err := RenderString(`<svg width="2" height="2"><path d="M-10 -10h2v2h-2z M10 10h2v2h-2z" fill="white" stroke="white"/><rect x="10" y="10" width="2" height="2" fill="white" stroke="white"/></svg>`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for offset := 3; offset < len(img.Pix); offset += 4 {
		if img.Pix[offset] != 0 {
			t.Fatal("offscreen geometry covered a pixel")
		}
	}
}
