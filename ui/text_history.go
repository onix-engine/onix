package ui

import "github.com/diiyw/onix/input"

const textFieldHistoryLimit = 128

type textFieldEdit struct {
	value         string
	caret, anchor int
}

type textFieldHistory struct {
	edits []textFieldEdit
	index int
}

// RecordEdit records a committed native edit. Document-backed editors own
// their history; ordinary controls retain immutable strings without copying.
func (f *TextField) RecordEdit(previous TextField) {
	if f.Buffer != nil || previous.Value == f.Value {
		return
	}
	if f.history == nil {
		f.history = &textFieldHistory{}
	}
	h := f.history
	if len(h.edits) == 0 || h.edits[h.index].value != previous.Value {
		clear(h.edits)
		h.edits = append(h.edits[:0], textFieldEdit{previous.Value, previous.Caret, previous.Anchor})
		h.index = 0
	}
	clear(h.edits[h.index+1:])
	h.edits = h.edits[:h.index+1]
	// Capture the selection immediately before the edit, including selections
	// made since the preceding edit.
	h.edits[h.index] = textFieldEdit{previous.Value, previous.Caret, previous.Anchor}
	if len(h.edits) == textFieldHistoryLimit {
		copy(h.edits, h.edits[1:])
		h.edits = h.edits[:len(h.edits)-1]
	}
	h.edits = append(h.edits, textFieldEdit{f.Value, f.Caret, f.Anchor})
	h.index = len(h.edits) - 1
}

func (f *TextField) restoreEdit(redo bool) {
	h := f.history
	if f.Buffer != nil || h == nil || len(h.edits) == 0 {
		return
	}
	if h.edits[h.index].value != f.Value {
		clear(h.edits)
		h.edits = h.edits[:0]
		return
	}
	index := h.index - 1
	if redo {
		index = h.index + 1
	}
	if index < 0 || index >= len(h.edits) {
		return
	}
	h.index = index
	edit := h.edits[index]
	f.Value, f.Caret, f.Anchor, f.Composition = edit.value, edit.caret, edit.anchor, ""
}

func textHistoryShortcut(state *input.State) (undo, redo bool) {
	if state.Composing() || state.KeyHeld(input.KeyLeftAlt) || state.KeyHeld(input.KeyRightAlt) {
		return false, false
	}
	primary := state.KeyHeld(input.KeyLeftControl) || state.KeyHeld(input.KeyRightControl) || state.KeyHeld(input.KeyLeftSuper) || state.KeyHeld(input.KeyRightSuper)
	shift := state.KeyHeld(input.KeyLeftShift) || state.KeyHeld(input.KeyRightShift)
	z := state.KeyPressed(input.KeyZ) && !state.KeyPressedDuringComposition(input.KeyZ)
	y := state.KeyPressed(input.KeyY) && !state.KeyPressedDuringComposition(input.KeyY)
	return primary && !shift && z, primary && (shift && z || !shift && y)
}
