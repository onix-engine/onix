package scene

import (
	"testing"

	notosanssc "github.com/onix-engine/onix/testdata/fonts/Noto_Sans_SC"
)

func TestTextLineLayoutKeepsNativeTextAndCaretGeometry(t *testing.T) {
	text := NewText2D("first\n中文\nlast")
	text.Font, text.LineHeight = notosanssc.NotoSansSCRegular, 20
	text.LineLayout = &TextLineLayout2D{Offsets: []float32{0, 60, 80}, Width: 400, Height: 160}
	width, height, ok := MeasureText2D(&text)
	if !ok || width != 400 || height != 160 {
		t.Fatalf("extent=%g,%g ok=%v", width, height, ok)
	}
	x, y, _, _, ok := TextCaretRect2D(&text, text.Value, 7)
	if !ok || y != 60 || TextCaretIndex2D(&text, text.Value, x, y+10) != 7 {
		t.Fatalf("caret at (%g,%g), ok=%v", x, y, ok)
	}
	if !text.ensureLayout() {
		t.Fatal("glyph layout failed")
	}
	reference := NewText2D("中文")
	reference.Font, reference.LineHeight = text.Font, text.LineHeight
	if !reference.ensureLayout() {
		t.Fatal("reference glyph layout failed")
	}
	index := 0
	for _, glyph := range text.glyphs {
		if glyph.line == 1 {
			if glyph.y != reference.glyphs[index].y+60 {
				t.Fatalf("glyph y=%g, want %g", glyph.y, reference.glyphs[index].y+60)
			}
			index++
		}
	}
	text.LineLayout.Offsets[1] = 40
	text.LineLayout.Revision++
	_, y, _, _, _ = TextCaretRect2D(&text, text.Value, 7)
	if y != 40 {
		t.Fatal("line layout revision was ignored")
	}
	if allocations := testing.AllocsPerRun(100, func() { MeasureText2D(&text) }); allocations != 0 {
		t.Fatalf("unchanged layout allocated %g times", allocations)
	}
	text.Value = ""
	if _, height, ok := MeasureText2D(&text); !ok || height != 160 {
		t.Fatal("empty side lost its scrollable extent")
	}
}

func TestHiddenLinesKeepWrappedSourceCoordinates(t *testing.T) {
	for _, mode := range []TextWrapMode{TextWrapNone, TextWrapWords, TextWrapTokens, TextWrapCharacters} {
		text := NewText2D("abc def ghi\r\n隐藏行\r\nafter\n")
		text.Font, text.LineHeight, text.WrapMode, text.WrapWidth = notosanssc.NotoSansSCRegular, 20, mode, 35
		value := text.Text()
		start, end := value.LineStart(1), value.LineStart(2)
		text.LineLayout = &TextLineLayout2D{Hidden: []TextHiddenLines2D{{start, end}, {value.Len(), value.Len() + 1}}}
		rows := TextLineCount2D(&text)
		if rows < 2 || !text.ensureLayout() {
			t.Fatal("missing visible lines")
		}
		for _, line := range text.lines {
			if line.start >= start && line.start < end || line.start == value.Len() {
				t.Fatalf("mode %v laid out hidden text at %d", mode, line.start)
			}
		}
		for _, g := range text.glyphs {
			if int(g.byteIndex) >= start && int(g.byteIndex) < end {
				t.Fatalf("mode %v painted hidden glyph", mode)
			}
		}
		caret := value.RuneOffset(end) + 1
		x, y, _, h, ok := TextCaretRect2D(&text, value, caret)
		if !ok || TextCaretIndex2D(&text, value, x, y+h/2) != caret {
			t.Fatalf("mode %v lost source hit", mode)
		}
		for range TextSelectionRects2D(&text, value, value.RuneOffset(start), value.RuneOffset(end), 0, 0) {
			t.Fatalf("mode %v painted hidden selection", mode)
		}
		text.LineLayout = nil
		if TextLineCount2D(&text) <= rows {
			t.Fatalf("mode %v did not expand", mode)
		}
	}
}
