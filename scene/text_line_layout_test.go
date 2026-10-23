package scene

import (
	"testing"

	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
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
