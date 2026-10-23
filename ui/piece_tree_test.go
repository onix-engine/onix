package ui

import (
	"testing"

	"github.com/diiyw/onix/input"
)

func TestEditTextValueDocumentBuffer(t *testing.T) {
	state := input.NewState()
	var buffer input.TextBuffer
	value, caret, anchor, _ := editTextValue("a界\r\nb", 2, 1, 0, true, []input.TextEdit{{Text: "😀"}}, state, false, &buffer)
	if value != "a😀\r\nb" || caret != 2 || anchor != 2 || buffer.String() != value {
		t.Fatalf("bound native input=%q caret=%d anchor=%d", value, caret, anchor)
	}
	value, caret, anchor, _ = editTextValue(value, caret, anchor, 0, true, []input.TextEdit{{Command: input.TextCommandBackspace}}, state, false, &buffer)
	if value != "a\r\nb" || caret != 1 || anchor != 1 || buffer.String() != value {
		t.Fatal("bound backspace did not edit document")
	}
	if buffer.RetainedEditBytes() != len("😀") {
		t.Fatal("document binding reset the tree")
	}
}
