package scene

import (
	"math"
	"testing"
	"unicode/utf8"

	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
)

func TestTextCaretUsesInlineObjectWidths(t *testing.T) {
	for _, mode := range []TextWrapMode{TextWrapNone, TextWrapCharacters, TextWrapWords} {
		text := NewText2D("前缀 [目录/](very/long/encoded/path/) ")
		text.Font = notosanssc.NotoSansSCRegular
		text.WrapMode, text.WrapWidth = mode, 100
		text.InlineObjects = []TextInlineObject2D{
			{Start: len("前缀 "), End: len("前缀 ["), Width: 1, Height: 1},
			{Start: len("前缀 [目录/"), End: len(text.Value) - 1, Width: 1, Height: 1},
		}
		if !text.ensureMeasuredLayout() {
			t.Fatal("layout")
		}
		check := func() {
			last := text.lines[len(text.lines)-1]
			x, y, _, _, ok := TextCaretRect2D(&text, text.Value, utf8.RuneCountInString(text.Value))
			if !ok || math.Abs(float64(x-last.x-last.advance)) > .001 || y != last.y {
				panic("caret does not follow rendered link and space")
			}
		}
		check()
		if n := testing.AllocsPerRun(100, check); n != 0 {
			t.Fatalf("cached inline caret allocated %g", n)
		}
	}
}
