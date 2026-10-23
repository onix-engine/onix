package layout

import (
	"math"
	"testing"

	"github.com/diiyw/onix/font"
)

type latinOnlyCmap struct {
	original interface{ GlyphIndex(rune) uint16 }
}

func (c latinOnlyCmap) GlyphIndex(r rune) uint16 {
	if r > 127 {
		return 0
	}
	return c.original.GlyphIndex(r)
}

func TestHTMLShapingUsesFallbackMetricsAndFaceBoundaries(t *testing.T) {
	face, err := font.Load("../../testdata/fonts/Noto_Sans_SC/static/NotoSansSC-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = face.Close() })
	face.Cmap = latinOnlyCmap{face.Cmap}
	face.Head.UnitsPerEm *= 2
	face.UseSystemFallback()
	if owner, id := face.ResolveGlyph('中'); owner == face || id == 0 {
		t.Skip("no installed Chinese fallback")
	}
	const size, spacing = 19.25, 1.5
	token := textToken{Text: "A中文B"}
	glyphs, width, err := shapeToken(face, token, size, spacing, nil)
	if err != nil || len(glyphs) != 4 {
		t.Fatalf("shape mixed text: %v", err)
	}
	var previousFace *font.Face
	var previousID uint16
	pen := 0.0
	for _, r := range token.Text {
		owner, id := face.ResolveGlyph(r)
		metrics, err := owner.HorizontalMetrics(id)
		if err != nil {
			t.Fatal(err)
		}
		scale := size / float64(owner.Head.UnitsPerEm)
		if owner == previousFace {
			pen += float64(owner.GlyphKerning(previousID, id)) * scale
		}
		glyph := glyphs[0]
		glyphs = glyphs[1:]
		if math.Abs(glyph.X-(pen+float64(metrics.LeftSideBearing)*scale)) > 1e-9 {
			t.Fatal("HTML glyph position used the primary font's design units")
		}
		pen += float64(metrics.AdvanceWidth)*scale + spacing
		previousFace, previousID = owner, id
	}
	if math.Abs(width-pen) > 1e-9 {
		t.Fatalf("mixed width = %v, want %v", width, pen)
	}
	if allocations := testing.AllocsPerRun(100, func() { shapeToken(face, token, size, spacing, nil) }); allocations != 0 {
		t.Fatalf("cached HTML shaping allocated %g times", allocations)
	}
}
