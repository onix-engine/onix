package input

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/diiyw/onix/text/piecetree"
)

type TextInputKind uint8

const (
	TextInputText TextInputKind = iota
	TextInputDecimal
	TextInputInteger
)

type TextInputConfig struct {
	Revision       uint64
	Kind           TextInputKind
	InitialValue   string
	SelectionStart int
	SelectionEnd   int
	Active         bool
	Multiline      bool
	Caret          TextInputRect
}

type TextInputRect struct {
	X      float32
	Y      float32
	Width  float32
	Height float32
}

func (c TextInputConfig) Value() uint64 {
	value := c.Revision << 8
	if c.Active {
		value |= 1
	}
	if c.Multiline {
		value |= 1 << 1
	}
	value |= uint64(c.Kind) << 2
	return value
}

type TextComposition struct {
	Text   string
	Active bool
}

type TextSelection struct {
	Start int
	End   int
}

type TextCommand uint8

const (
	TextCommandBackspace TextCommand = iota + 1
	TextCommandEnter
)

type TextEdit struct {
	Text              string
	Command           TextCommand
	DuringComposition bool
	SelectionStart    int
	SelectionEnd      int
	HasSelection      bool
}

func ApplyTextEditsInRange(value string, selectionStart, selectionEnd, maxRunes int, multiline bool, edits []TextEdit) (string, int, int) {
	var buffer TextBuffer
	return buffer.apply(value, selectionStart, selectionEnd, maxRunes, multiline, edits, 0)
}

// ApplyTextEditsInRange applies edits through the State's reusable piece tree.
// Keeping the buffer on the active input session avoids rebuilding a rune
// slice for every key press.
func (s *State) ApplyTextEditsInRange(value string, selectionStart, selectionEnd, maxRunes int, multiline bool, edits []TextEdit) (string, int, int) {
	if s == nil {
		return ApplyTextEditsInRange(value, selectionStart, selectionEnd, maxRunes, multiline, edits)
	}
	return s.textBuffer.apply(value, selectionStart, selectionEnd, maxRunes, multiline, edits, 64)
}

// TextBuffer owns a document's piece tree and reusable input scratch space.
// Keep one per editor document; String snapshots are immutable. Do not copy it.
type TextBuffer struct {
	piecetree.Tree
	value                                   string
	scratch                                 []byte
	cursor                                  int
	initialized, valid                      bool
	before                                  string
	changeStart, changeOldEnd, changeNewEnd int
	changed                                 bool
}

// Sync installs an external snapshot only when the document has changed.
func (b *TextBuffer) Sync(value string) { b.sync(value, 64) }

// ApplyTextEditsInRange uses rune selections and returns one cached snapshot
// after applying the entire batch to the tree.
func (b *TextBuffer) ApplyTextEditsInRange(value string, start, end, maxRunes int, multiline bool, edits []TextEdit) (string, int, int) {
	return b.apply(value, start, end, maxRunes, multiline, edits, 64)
}

func (b *TextBuffer) apply(value string, selectionStart, selectionEnd, maxRunes int, multiline bool, edits []TextEdit, reserve int) (string, int, int) {
	if len(edits) == 0 {
		length := 0
		valid := false
		if b.initialized && b.valid && b.value == value {
			length, valid = b.Runes(), true
		} else if utf8.ValidString(value) {
			length, valid = utf8.RuneCountInString(value), true
		}
		if valid {
			selectionStart = max(0, min(selectionStart, length))
			selectionEnd = max(0, min(selectionEnd, length))
			if selectionStart > selectionEnd {
				selectionStart, selectionEnd = selectionEnd, selectionStart
			}
			return value, selectionEnd, 0
		}
	}
	b.sync(value, reserve)
	b.before, b.changed = value, false
	if !b.valid {
		b.scratch = b.scratch[:0]
		for _, r := range value {
			b.scratch = utf8.AppendRune(b.scratch, r)
		}
		b.Tree.Reset(string(b.scratch))
		b.changed = true
		b.changeStart, b.changeOldEnd, b.changeNewEnd = 0, len(value), b.Len()
	}
	length := b.Runes()
	selectionStart = max(0, min(selectionStart, length))
	selectionEnd = max(0, min(selectionEnd, length))
	if selectionStart > selectionEnd {
		selectionStart, selectionEnd = selectionEnd, selectionStart
	}
	caret := selectionEnd
	submits := 0
	changed := !b.valid
	for _, edit := range edits {
		if edit.HasSelection {
			selectionStart = max(0, min(edit.SelectionStart, b.Runes()))
			selectionEnd = max(0, min(edit.SelectionEnd, b.Runes()))
			if selectionStart > selectionEnd {
				selectionStart, selectionEnd = selectionEnd, selectionStart
			}
			caret = selectionEnd
			continue
		}
		if edit.Text != "" {
			limit := -1
			if maxRunes > 0 {
				limit = max(maxRunes-b.Runes()+(selectionEnd-selectionStart), 0)
			}
			removed := selectionEnd - selectionStart
			b.delete(selectionStart, selectionEnd)
			inserted := b.insertSanitized(edit.Text, multiline, limit)
			if inserted > 0 || removed > 0 {
				changed = true
				caret = selectionStart + inserted
				selectionStart, selectionEnd = caret, caret
			}
			continue
		}
		if edit.DuringComposition {
			continue
		}
		switch edit.Command {
		case TextCommandBackspace:
			if selectionStart != selectionEnd {
				b.delete(selectionStart, selectionEnd)
				changed = true
				caret = selectionStart
				selectionEnd = selectionStart
			} else if caret > 0 {
				b.delete(caret-1, caret)
				changed = true
				caret--
				selectionStart, selectionEnd = caret, caret
			}
		case TextCommandEnter:
			if multiline {
				if maxRunes <= 0 || b.Runes()-(selectionEnd-selectionStart) < maxRunes {
					b.delete(selectionStart, selectionEnd)
					b.insertRune('\n')
					changed = true
					caret = selectionStart + 1
					selectionStart, selectionEnd = caret, caret
				}
			} else {
				submits++
			}
		}
	}
	if !changed {
		b.value = value
		return value, caret, submits
	}
	return b.string(), caret, submits
}

func (b *TextBuffer) sync(value string, reserve int) {
	if b.initialized && b.value == value {
		return
	}
	b.valid = utf8.ValidString(value)
	b.Tree.Reset(value)
	b.before, b.changed = "", false
	if reserve > 0 {
		b.Tree.Reserve(reserve*utf8.UTFMax, 64)
	}
	b.value, b.initialized = value, true
}

func (b *TextBuffer) delete(start, end int) {
	b.cursor = b.ByteOffset(start)
	b.replace(b.cursor, b.ByteOffset(end), "")
}

func (b *TextBuffer) insertRune(value rune) {
	var encoded [utf8.UTFMax]byte
	size := utf8.EncodeRune(encoded[:], value)
	b.replace(b.cursor, b.cursor, string(encoded[:size]))
	b.cursor += size
}

func (b *TextBuffer) insertSanitized(text string, multiline bool, limit int) int {
	b.scratch = b.scratch[:0]
	inserted := 0
	for offset := 0; offset < len(text) && (limit < 0 || inserted < limit); {
		current, size := utf8.DecodeRuneInString(text[offset:])
		offset += size
		if current == utf8.RuneError && size == 1 {
			for offset < len(text) {
				next, width := utf8.DecodeRuneInString(text[offset:])
				if next != utf8.RuneError || width != 1 {
					break
				}
				offset += width
			}
		}
		if current == '\r' {
			if offset < len(text) && text[offset] == '\n' {
				offset++
			}
			current = '\n'
		}
		if current == '\n' || current == '\t' {
			if !multiline {
				continue
			}
		} else if unicode.IsControl(current) {
			continue
		}
		b.scratch = utf8.AppendRune(b.scratch, current)
		inserted++
	}
	b.replace(b.cursor, b.cursor, string(b.scratch))
	b.cursor += len(b.scratch)
	return inserted
}

func (b *TextBuffer) string() string {
	b.value = b.Tree.String()
	b.valid = true
	// Deltas in history own their bytes, so compaction can reclaim deleted text.
	if b.RetainedBytes() > max(64<<10, b.Len()*3) {
		b.Tree.Compact()
		b.Tree.Reserve(256, 64)
	}
	return b.value
}

// ReplaceBytes applies an external byte-range edit and caches its snapshot.
func (b *TextBuffer) ReplaceBytes(start, end int, text string) {
	valid := b.valid && utf8.ValidString(text)
	b.before, b.changed = b.value, false
	b.replace(start, end, text)
	b.value = b.string()
	b.initialized = true
	if !valid {
		b.valid = utf8.ValidString(b.value)
	}
}

// ApplySnapshot commits an external edit without allocating another full copy.
// The range must describe all differences from the current document.
func (b *TextBuffer) ApplySnapshot(value string, start, oldEnd, newEnd int) {
	b.before, b.changed = b.value, true
	b.changeStart, b.changeOldEnd, b.changeNewEnd = start, oldEnd, newEnd
	b.Tree.ApplySnapshot(value, start, oldEnd, newEnd)
	b.value, b.initialized, b.valid = value, true, utf8.ValidString(value)
	b.string()
}

// ChangedRange reports the batch's byte delta without comparing document-sized
// snapshots. It is valid only for the exact before/after pair of the last edit.
func (b *TextBuffer) ChangedRange(before, after string) (start, oldEnd, newEnd, prefixRunes int, ok bool) {
	if !b.changed || b.before != before || b.value != after || !b.Matches(after) {
		return
	}
	start, oldEnd, newEnd = b.changeStart, b.changeOldEnd, b.changeNewEnd
	prefixRunes = b.RuneOffset(start)
	ok = true
	return
}

func (b *TextBuffer) replace(start, end int, text string) {
	if start == end && text == "" {
		return
	}
	if !b.changed {
		b.changeStart, b.changeOldEnd, b.changeNewEnd = start, end, start+len(text)
		b.changed = true
	} else {
		b.changeStart = min(b.changeStart, start)
		b.changeOldEnd += max(0, end-b.changeNewEnd)
		b.changeNewEnd = max(b.changeNewEnd, end) + len(text) - (end - start)
	}
	b.Tree.Replace(start, end, text)
}

func normalizeText(text string) string {
	return strings.ToValidUTF8(text, "\uFFFD")
}

func sanitizeText(text string, multiline bool) string {
	text = normalizeText(text)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			if multiline {
				return r
			}
			return -1
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
}
