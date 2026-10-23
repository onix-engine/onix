package scene

import (
	"slices"
	"strings"
	"testing"

	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
)

func TestTextReusesUnchangedPlainLineMetrics(t *testing.T) {
	for _, mode := range []TextWrapMode{TextWrapNone, TextWrapTokens, TextWrapCharacters} {
		base := strings.Repeat("plain\t中文🙂\r\n", 120)
		text := NewText2D(base)
		text.Font, text.LineHeight, text.TabWidth = notosanssc.NotoSansSCRegular, 20, 32
		text.WrapMode, text.WrapWidth = mode, 160
		text.glyphBand = textGlyphBand{top: 100, bottom: 180, valid: true}
		for _, value := range []string{base, "longer first line\n" + base, base + "tail", strings.Replace(base, "中文", "a very long replacement", 1), base[:len(base)/2], "", base} {
			text.Value = value
			for _, spacing := range []float32{0, 1, 0} {
				text.LetterSpacing = spacing
				if !text.ensureLayout() {
					t.Fatal("incremental layout")
				}
				reference := NewText2D(value)
				reference.Font, reference.LineHeight, reference.TabWidth = text.Font, text.LineHeight, text.TabWidth
				reference.LetterSpacing, reference.glyphBand = spacing, text.glyphBand
				reference.WrapMode, reference.WrapWidth = text.WrapMode, text.WrapWidth
				if !reference.ensureLayout() || !slices.Equal(text.lines, reference.lines) || text.layoutWidth != reference.layoutWidth || text.layoutHeight != reference.layoutHeight || !slices.Equal(text.glyphs, reference.glyphs) || !slices.Equal(text.wrapRangeCache, reference.wrapRangeCache) {
					t.Fatal("reused line metrics differ from a fresh layout")
				}
			}
		}
	}
}
