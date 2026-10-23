package scene

import (
	"math"
	"testing"
	"unicode/utf8"

	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	"github.com/go-gl/mathgl/mgl32"
)

func TestText2DInkOffsetAffectsPaintOnly(t *testing.T) {
	text := NewText2D("123")
	text.Font = notosanssc.NotoSansSCRegular
	text.LineHeight = 25
	if !text.ensureLayout() {
		t.Fatal("text layout failed")
	}
	_, caretY, _, caretHeight, ok := TextCaretRect2D(&text, text.Value, 1)
	if !ok {
		t.Fatal("caret layout failed")
	}
	baseline := text.glyphs[0].y
	text.InkOffsetY = -2
	if !text.ensureLayout() || text.glyphs[0].y != baseline {
		t.Fatal("ink offset changed glyph layout")
	}
	_, nextY, _, nextHeight, ok := TextCaretRect2D(&text, text.Value, 1)
	if !ok || nextY != caretY || nextHeight != caretHeight {
		t.Fatal("ink offset changed caret geometry")
	}
	items := text.appendRenderItems(nil, 1, mgl32.Ident4(), clipRect2D{}, true, 0)
	if len(items) == 0 || items[0].transform[13] != -2 {
		t.Fatalf("paint offset=%v, want -2", items)
	}
}

func TestText2DTabStopsShareRenderingMeasurementAndCaret(t *testing.T) {
	text := NewText2D("A\t界\tB\n\tC")
	text.Font = notosanssc.NotoSansSCRegular
	text.FontSize, text.DPI, text.LineHeight, text.TabWidth = 13, 72, 21.45, 30
	if !text.ensureLayout() {
		t.Fatal("text layout failed")
	}
	for caret := 0; caret <= utf8.RuneCountInString(text.Value); caret++ {
		x, y, _, height, ok := TextCaretRect2D(&text, text.Value, caret)
		if !ok || TextCaretIndex2D(&text, text.Value, x, y+height/2) != caret {
			t.Fatalf("tab caret %d did not round-trip at (%v,%v)", caret, x, y)
		}
	}
	for _, point := range []struct {
		caret int
		x     float32
	}{
		{caret: 2, x: 30},
		{caret: 4, x: 60},
		{caret: 7, x: 30},
	} {
		x, _, _, _, _ := TextCaretRect2D(&text, text.Value, point.caret)
		if math.Abs(float64(x-point.x)) > .001 {
			t.Fatalf("caret %d x=%v, want %v", point.caret, x, point.x)
		}
	}
	for _, glyph := range text.glyphs {
		want := float32(30)
		if glyph.raster.Rune == 'B' {
			want = 60
		} else if glyph.raster.Rune != '界' && glyph.raster.Rune != 'C' {
			continue
		}
		if x := glyph.x - glyph.raster.OffsetX; math.Abs(float64(x-want)) > .001 {
			t.Fatalf("glyph %q origin=%v, want %v", glyph.raster.Rune, x, want)
		}
	}
	if width, _, _ := MeasureTextValue2D(&text, "A\t界\t"); width != 60 {
		t.Fatalf("tab prefix width=%v, want 60", width)
	}
	text.TabWidth = 40
	x, _, _, _, _ := TextCaretRect2D(&text, text.Value, 4)
	if x != 80 {
		t.Fatalf("changed tab interval did not invalidate layout: x=%v", x)
	}
}

func TestText2DMeasuresWhitespaceOnlyLines(t *testing.T) {
	text := NewText2D("\n\n\n")
	text.Font = notosanssc.NotoSansSCRegular
	text.LineHeight = 20
	width, height, ok := MeasureText2D(&text)
	if !ok || width != 0 || height != 80 {
		t.Fatalf("blank lines measured as (%v,%v), ok=%v", width, height, ok)
	}
}

func TestText2DWidthUsesAdvancesAcrossRasterScales(t *testing.T) {
	for _, value := range []string{"i", "f", "代码", "app_ecs.go"} {
		t.Run(value, func(t *testing.T) {
			text := NewText2D(value)
			text.Font = notosanssc.NotoSansSCRegular
			text.FontSize, text.DPI = 13, 72
			want, _, ok := MeasureTextValue2D(&text, value)
			if !ok {
				t.Fatal("text measurement failed")
			}
			for _, scale := range []float64{1, 1.5, 2} {
				text.rasterScale = scale
				for _, paint := range []bool{false, true} {
					if !text.buildLayout(paint) {
						t.Fatal("text layout failed")
					}
					if width := text.layoutWidth; math.Abs(float64(width-want)) > 1.0/32 {
						t.Errorf("scale %v, paint %v: width = %v, advance width = %v", scale, paint, width, want)
					}
				}
			}
		})
	}
}
