package textfield

import (
	"github.com/onix-engine/onix/input"
	"github.com/onix-engine/onix/text/piecetree"
)

type Field struct {
	Source piecetree.Snapshot

	LastScrollValue piecetree.Snapshot
	// Buffer optionally binds native input to a document-owned piece tree.
	Buffer                *input.TextBuffer
	history               *textFieldHistory
	Composition           string
	Placeholder           string
	Value                 string
	CommittedValue        string
	LastScrollComposition string
	LastScrollCaret       int
	Caret                 int
	MaxRunes              int
	FontSize              float64
	DPI                   float64
	Anchor                int
	// ScrollX is the signed horizontal text offset. Positive values move the
	// text right; negative values reveal the caret in overflowing values.
	ScrollX            float32
	LastScrollViewport float32
	Submitted          bool
	Password           bool
	LastScrollFocused  bool
	Selecting          bool
	ScrollInitialized  bool
	Focused            bool
}

func (f Field) Text() piecetree.Snapshot {
	if f.Buffer != nil {
		return f.Source
	}
	return piecetree.FromString(f.Value)
}

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
func (f *Field) RecordEdit(previous Field) {
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

func (f *Field) RestoreEdit(redo bool) {
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

// HistoryPosition and RestoreHistoryPosition support canceling a tentative undo or redo.
func (f Field) HistoryPosition() int {
	if f.history == nil {
		return 0
	}
	return f.history.index
}

func (f *Field) RestoreHistoryPosition(index int) {
	if f.history != nil {
		f.history.index = index
	}
}

// SetValue updates the live value and clamps the selection to its rune length.
func (f *Field) SetValue(value string) {
	if f.Buffer != nil {
		f.Buffer.Sync(value)
		f.Source = f.Buffer.Snapshot()
	} else {
		f.Value = value
	}
	f.Composition = ""
	length := f.Text().Runes()
	f.Anchor, f.Caret = min(max(f.Anchor, 0), length), min(max(f.Caret, 0), length)
}
