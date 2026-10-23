package html

import (
	"bytes"
	"image"
	"testing"

	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/internal/layout"
	"github.com/diiyw/onix/internal/style"
)

type noGlyphCmap struct{}

func (noGlyphCmap) GlyphIndex(rune) uint16 { return 0 }

func TestHTMLCompositeTextRasterUsesFallbackFace(t *testing.T) {
	face, err := font.Load("../../testdata/fonts/Noto_Sans_SC/static/NotoSansSC-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = face.Close() })
	face.Cmap = noGlyphCmap{}
	face.Head.UnitsPerEm *= 2
	face.UseSystemFallback()
	owner, id := face.ResolveGlyph('中')
	if owner == face || id == 0 {
		t.Skip("no installed Chinese fallback")
	}
	computed := &style.ComputedStyle{}
	computed.FontSize = style.LengthPx(24)
	computed.Color = style.Value{Kind: style.ValueColor, Color: style.Color{R: 255, A: 255}}
	box := &layout.Box{TextFragments: []layout.TextFragment{{Style: computed, Text: "中", Glyphs: []layout.GlyphFragment{{Rune: '中', X: 8.5, Y: 32.25}}}}}
	got, want := image.NewRGBA(image.Rect(0, 0, 64, 64)), image.NewRGBA(image.Rect(0, 0, 64, 64))
	rasterHTMLText(got, box, face, nil, 0, 0)
	rasterHTMLText(want, box, owner, nil, 0, 0)
	if !bytes.Equal(got.Pix, want.Pix) || bytes.Equal(got.Pix, make([]byte, len(got.Pix))) {
		t.Fatal("composite text did not rasterize the owning fallback glyph")
	}
}
