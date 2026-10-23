package scene

import (
	"slices"
	"strings"
	"testing"

	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
)

func windowRelayoutFixture(t testing.TB, mode TextWrapMode) (Text2D, string, string) {
	t.Helper()
	short := strings.Repeat("x\n", 400)
	dense := strings.Repeat("x", 6000) + strings.Repeat("\nx", 399)
	value := NewText2D(short)
	value.Font = notosanssc.NotoSansSCRegular
	value.WrapMode, value.WrapWidth = mode, 80
	if !value.ensureMeasuredLayout() {
		t.Fatal("measure failed")
	}
	value.glyphBand = textGlyphBand{top: 0, bottom: value.lines[0].height / 2, valid: true}
	return value, short, dense
}

func TestTextWindowRelayoutSizesCurrentDocument(t *testing.T) {
	t.Run("unwrapped", func(t *testing.T) { testTextWindowRelayout(t, TextWrapNone) })
	t.Run("tokens", func(t *testing.T) { testTextWindowRelayout(t, TextWrapTokens) })
}

func testTextWindowRelayout(t *testing.T, mode TextWrapMode) {
	value, short, dense := windowRelayoutFixture(t, mode)
	cycle := func() {
		value.Value = short
		value.ensureLayout()
		value.Value = dense
		value.ensureLayout()
	}
	cycle()
	if len(value.glyphs) != 6000 || len(value.lines) != 400 {
		t.Fatalf("glyphs=%d lines=%d", len(value.glyphs), len(value.lines))
	}
	full := NewText2D(dense)
	full.Font = value.Font
	full.WrapMode, full.WrapWidth = value.WrapMode, value.WrapWidth
	if !full.ensureLayout() {
		t.Fatal("full layout failed")
	}
	if !slices.Equal(value.lines, full.lines) || !slices.Equal(value.glyphs, full.glyphs[:6000]) {
		t.Fatal("windowed geometry differs from full layout")
	}
	// Shrinking the window releases its peak buffer, then growing it needs one
	// correctly sized allocation rather than repeated growth from stale offsets.
	if allocs := testing.AllocsPerRun(20, cycle); allocs > 2 {
		t.Fatalf("switching windowed documents allocated %g times; want at most 2", allocs)
	}
	if allocs := testing.AllocsPerRun(20, func() {
		value.changed = true
		value.ensureLayout()
	}); allocs != 0 {
		t.Fatalf("rebuilding a warm window allocated %g times", allocs)
	}
}

func BenchmarkTextWindowRelayout(b *testing.B) {
	value, short, dense := windowRelayoutFixture(b, TextWrapNone)
	cycle := func() { value.Value = short; value.ensureLayout(); value.Value = dense; value.ensureLayout() }
	cycle()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		cycle()
	}
}

func TestTextWindowRelayoutWrappedGeometry(t *testing.T) {
	for _, mode := range []TextWrapMode{TextWrapWords, TextWrapCharacters, TextWrapTokens} {
		for _, height := range []float32{0, 8, 27.3} {
			for _, withObject := range []bool{false, true} {
				value := NewText2D("old")
				value.Font = notosanssc.NotoSansSCRegular
				value.WrapMode, value.WrapWidth, value.LineHeight = mode, 85, height
				value.OutlineWidth = 1
				value.glyphBand = textGlyphBand{top: 100, bottom: 140, valid: true}
				if !value.ensureLayout() {
					t.Fatal("initial layout failed")
				}
				value.Value = strings.Repeat("AV界\tand_more words\n", 30)
				value.TabWidth = 30
				if withObject {
					value.InlineObjects = []TextInlineObject2D{{Start: 0, End: 1, Width: 20, Height: 50, Baseline: 40}}
				}
				full := value
				full.glyphBand = textGlyphBand{}
				full.glyphs, full.outlineGlyphs, full.lines, full.wrapRangeCache, full.inlineObjectLayouts = nil, nil, nil, nil, nil
				if !full.ensureLayout() || !value.ensureLayout() {
					t.Fatal("replacement layout failed")
				}
				if !slices.Equal(value.lines, full.lines) {
					t.Fatalf("line geometry changed: mode=%v height=%v object=%v", mode, height, withObject)
				}
				count := 0
				for index, glyph := range full.glyphs {
					if !value.inGlyphBand(full.lines[glyph.line].y) {
						continue
					}
					if count >= len(value.glyphs) || value.glyphs[count] != glyph || value.outlineGlyphs[count] != full.outlineGlyphs[index] {
						t.Fatalf("window glyph mismatch: mode=%v height=%v object=%v", mode, height, withObject)
					}
					count++
				}
				if count != len(value.glyphs) {
					t.Fatal("glyphs escaped the window")
				}
			}
		}
	}
}
