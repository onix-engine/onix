package ui

import (
	"testing"

	"github.com/diiyw/onix/input"
)

func TestClipboardShortcutsPreservePressModifiers(t *testing.T) {
	for _, primary := range []input.KeyCode{input.KeyLeftControl, input.KeyLeftSuper} {
		for _, test := range []struct {
			name, value, copied string
			key                 input.KeyCode
			caret, anchor       int
		}{
			{"select all", "甲乙🙂尾", "", input.KeyA, 4, 0},
			{"copy", "甲乙🙂尾", "乙🙂", input.KeyC, 3, 1},
			{"cut", "甲尾", "乙🙂", input.KeyX, 1, 1},
			{"paste", "甲中\ncode()尾", "", input.KeyV, 9, 9},
		} {
			t.Run(test.name, func(t *testing.T) {
				state := input.NewState()
				copied := ""
				state.SetClipboard(func(value string) bool { copied = value; return true }, func() (string, bool) { return "中\r\ncode()", true })
				state.PressKey(primary)
				state.PressKey(test.key)
				state.ReleaseKey(test.key)
				state.ReleaseKey(primary)
				value, caret, anchor, _ := editTextValue("甲乙🙂尾", 3, 1, 0, true, nil, state, false, nil)
				if value != test.value || caret != test.caret || anchor != test.anchor || copied != test.copied {
					t.Fatalf("shortcut lost press modifiers: text=%q selection=%d:%d clipboard=%q", value, anchor, caret, copied)
				}
				state.EndFrame()
				state.PressKey(test.key)
				state.PressKey(primary)
				value, caret, anchor, _ = editTextValue("甲乙🙂尾", 3, 1, 0, true, nil, state, false, nil)
				if value != "甲乙🙂尾" || caret != 3 || anchor != 1 {
					t.Fatal("modifier pressed after the key changed the text or selection")
				}
			})
		}
	}
}
