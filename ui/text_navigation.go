package ui

import (
	"runtime"
	"unicode"
	"unicode/utf8"

	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/text/piecetree"
)

func moveTextCaret(text piecetree.Snapshot, caret, anchor int, state *input.State) (int, int) {
	left, right := state.KeyPressed(input.KeyLeft), state.KeyPressed(input.KeyRight)
	up, down := state.KeyPressed(input.KeyUp), state.KeyPressed(input.KeyDown)
	home, end := state.KeyPressed(input.KeyHome), state.KeyPressed(input.KeyEnd)
	if !left && !right && !up && !down && !home && !end {
		return caret, anchor
	}
	shift := state.KeyHeld(input.KeyLeftShift) || state.KeyHeld(input.KeyRightShift)
	ctrl := state.KeyHeld(input.KeyLeftControl) || state.KeyHeld(input.KeyRightControl)
	super := state.KeyHeld(input.KeyLeftSuper) || state.KeyHeld(input.KeyRightSuper)
	alt := state.KeyHeld(input.KeyLeftAlt) || state.KeyHeld(input.KeyRightAlt)
	mac := runtime.GOOS == "darwin"
	word := ctrl && !mac || alt && mac
	length := text.Runes()
	switch {
	case (home || end) && ctrl || mac && super && (up || down):
		caret = 0
		if end || down {
			caret = length
		}
	case home || end || up || down || mac && super && (left || right):
		line, _ := text.Position(text.ByteOffset(caret))
		start := text.RuneOffset(text.LineStart(line))
		finish := length
		if line+1 < text.Lines() {
			finish = text.RuneOffset(text.LineStart(line+1)) - 1
		}
		if home || mac && super && left {
			caret = start
		} else if end || mac && super && right {
			caret = finish
		} else {
			column := caret - start
			if up && line > 0 {
				caret = min(text.RuneOffset(text.LineStart(line-1))+column, start-1)
			} else if down && line+1 < text.Lines() {
				nextEnd := length
				if line+2 < text.Lines() {
					nextEnd = text.RuneOffset(text.LineStart(line+2)) - 1
				}
				caret = min(finish+1+column, nextEnd)
			} else if up {
				caret = 0
			} else {
				caret = length
			}
		}
	case caret != anchor && !shift:
		if left {
			caret = min(caret, anchor)
		} else {
			caret = max(caret, anchor)
		}
	case word:
		caret = textWordBoundary(text, caret, right)
	case left:
		caret = max(0, caret-1)
	case right:
		caret = min(length, caret+1)
	}
	if !shift {
		anchor = caret
	}
	return caret, anchor
}

func textWordClass(r rune) int {
	if unicode.IsSpace(r) {
		return 0
	}
	if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' {
		return 1
	}
	return 2
}

func textWordBoundary(text piecetree.Snapshot, caret int, forward bool) int {
	offset := text.ByteOffset(caret)
	class := -1
	for offset > 0 && !forward || offset < text.Len() && forward {
		pos := offset
		if !forward {
			pos--
			for pos > 0 && !utf8.RuneStart(text.ByteAt(pos)) {
				pos--
			}
		}
		r, size := text.DecodeRune(pos)
		next := textWordClass(r)
		if class >= 0 && next != class {
			if forward && next != 0 || !forward && class != 0 {
				break
			}
		}
		class = next
		if forward {
			offset += size
			caret++
		} else {
			offset = pos
			caret--
		}
	}
	return caret
}
