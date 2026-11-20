package ui

import "github.com/onix-engine/onix/input"

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
