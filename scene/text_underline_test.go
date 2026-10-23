package scene

import (
	"testing"

	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
)

func TestTextUnderlineFollowsBaselineAndSkipsDescenders(t *testing.T) {
	text := NewText2D("\tBagy_End\nSecond")
	text.Font = notosanssc.NotoSansSCRegular
	text.FontSize, text.LineHeight, text.InkOffsetY, text.TabWidth = 18, 36, -2, 32
	text.LetterSpacing = 1
	var rects []TextRangeRect2D
	var bounds TextRangeRect2D
	var ok bool
	measure := func() { rects, bounds, ok = AppendTextUnderlineRects2D(rects[:0], &text, 0, 1, 9, 1) }
	measure()
	if !ok || len(rects) < 2 || bounds.Y != text.lines[0].baseline+text.InkOffsetY+1 || bounds.X != 33 {
		t.Fatalf("underline geometry: bounds=%+v segments=%+v", bounds, rects)
	}
	pen, previous := float32(0), rune(-1)
	for index, character := range text.Value[:9] {
		if index > 0 {
			pen += text.LetterSpacing
		}
		if character == '\t' {
			pen += textTabAdvance(pen, text.TabWidth)
			previous = -1
			continue
		}
		if previous >= 0 {
			pen += fixed26_6ToFloat32(text.face.Kern(previous, character))
		}
		ink, advance, _ := text.face.GlyphBounds(character)
		if character == 'g' || character == 'y' {
			left, right := pen+float32(ink.Min.X), pen+float32(ink.Max.X)
			for _, rect := range rects {
				if rect.X < right && rect.X+rect.Width > left {
					t.Fatalf("underline crosses %c: %+v", character, rect)
				}
			}
		}
		pen += fixed26_6ToFloat32(advance)
		previous = character
	}
	if allocations := testing.AllocsPerRun(100, measure); allocations != 0 {
		t.Fatalf("cached underline allocated %g times", allocations)
	}
	text.InkOffsetY -= 3
	measure()
	if bounds.Y != text.lines[0].baseline+text.InkOffsetY+1 {
		t.Fatal("underline ignored changed ink offset")
	}
	text.WrapMode, text.WrapWidth = TextWrapTokens, 60
	rects, bounds, ok = AppendTextUnderlineRects2D(rects[:0], &text, 1, 1, 9, 1)
	if !ok || bounds.X+bounds.Width > text.lines[1].x+text.lines[1].width+.01 {
		t.Fatal("underline crossed a wrapped line")
	}
}
