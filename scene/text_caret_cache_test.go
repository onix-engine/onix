package scene

import (
	"testing"
	"unicode/utf8"

	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
)

func TestTextCaretCacheInvalidationAndUnicode(t *testing.T) {
	text := NewText2D("")
	text.Font = notosanssc.NotoSansSCRegular
	for _, value := range []string{"first\r\n中文🙂\nlast", "é\tchanged\n🙂", "", "a\x80z"} {
		text.Value = value
		count := utf8.RuneCountInString(value)
		for _, caret := range []int{count, count - 1, 0, count / 2, count + 10, -10, count} {
			reference := NewText2D(value)
			reference.Font = text.Font
			x, y, w, h, ok := TextCaretRect2D(&text, value, caret)
			rx, ry, rw, rh, rok := TextCaretRect2D(&reference, value, caret)
			if x != rx || y != ry || w != rw || h != rh || ok != rok {
				t.Fatalf("cached caret differs after content change: %q, %d", value, caret)
			}
		}
	}
	if allocations := testing.AllocsPerRun(100, func() {
		TextCaretRect2D(&text, text.Value, 1)
		TextCaretRect2D(&text, text.Value, 2)
	}); allocations != 0 {
		t.Fatalf("warm caret navigation allocated %g times", allocations)
	}
}
