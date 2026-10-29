package text

import (
	"testing"

	"github.com/diiyw/onix/font"
)

type noGlyphCmap struct{}

func (noGlyphCmap) GlyphIndex(rune) uint16 { return 0 }

func TestSystemFallbackGlyphCachesStayConsistentAndAllocationFree(t *testing.T) {
	face, err := font.Load("../testdata/fonts/Noto_Sans_SC/static/NotoSansSC-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	face.Cmap = noGlyphCmap{}
	face.Head.UnitsPerEm *= 2
	value := NewFont(face)
	t.Cleanup(func() { _ = value.Dispose() })
	owner, id := value.ResolveGlyph('中')
	if owner == face || id == 0 {
		t.Skip("no installed Chinese fallback")
	}
	cache := value.GlyphCache(18, 72)
	metrics := cache.GlyphMetrics('中')
	if cache.AtlasCount() != 0 {
		t.Fatal("measuring fallback glyphs populated an atlas")
	}
	gray := cache.Glyph('中')
	lcd := cache.GlyphLCDAt('中', 0)
	outline := cache.OutlineGlyph('中', 1)
	if !gray.Visible() || !lcd.Visible() || !outline.Visible() || gray.Width != metrics.Width || gray.Height != metrics.Height || gray.OffsetX != metrics.OffsetX || gray.OffsetY != metrics.OffsetY || gray.Advance != metrics.Advance || gray.Advance != lcd.Advance || gray.Advance != outline.Advance {
		t.Fatal("fallback measurement, gray, LCD, and outline glyphs disagree")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		cache.GlyphMetrics('中')
		cache.Glyph('中')
		cache.GlyphLCDAt('中', 0)
		cache.OutlineGlyph('中', 1)
		cache.Kern('中', '中')
	}); allocations != 0 {
		t.Fatalf("warmed fallback glyph cache allocated %g times", allocations)
	}
	if owner.GlyphIndex('中') != id {
		t.Fatal("glyph cache modified the fallback face")
	}
}
