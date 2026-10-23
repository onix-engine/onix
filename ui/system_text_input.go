package ui

import (
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/ui/html"
	"github.com/go-gl/mathgl/mgl32"
)

const inputDoubleClickInterval = 500 * time.Millisecond

func SyncTextInputSystem(name string) ecs.SystemConfig {
	if name == "" {
		name = "sync_ui_text_input"
	}
	return ecs.SystemConfig{Name: name, System: syncTextInput}
}

func syncTextInput(context *ecs.SystemContext) error {
	world := context.World
	if world == nil {
		return nil
	}
	state, ok := ecs.GetResource[*uiEventState](world)
	if !ok || state == nil || state.Focused == 0 {
		return nil
	}
	uiInputState, ok := ecs.GetResource[*input.State](world)
	if !ok || uiInputState == nil {
		return nil
	}
	field, ok := ecs.Get[TextField](world, state.Focused)
	if !ok || !field.Focused {
		return nil
	}
	found := false
	for entity, access := range ecs.Query3[Relation[SurfaceStyle], scene.UIGlobalTransform, scene.Size2D](world).All {
		visual := access.A.Ref()
		transform := access.B.Ref()
		size := access.C.Ref()

		if visual.Owner != state.Focused || !ecs.Has[Part[CaretPart]](world, entity) {
			continue
		}
		found = setTextInputCaret(uiInputState, transform, scene.AnchorTopLeft, size.Width, size.Height)
		if !(!found) {
			break
		}
	}
	if found {
		return nil
	}
	for entity, access := range ecs.Query3[Relation[Control], scene.UIGlobalTransform, scene.Sprite2D](world).All {
		visual := access.A.Ref()
		transform := access.B.Ref()
		sprite := access.C.Ref()

		if visual.Owner != state.Focused || !ecs.Has[Part[CaretPart]](world, entity) {
			continue
		}
		anchor, ok := ecs.Get[scene.Anchor](world, entity)
		if !ok {
			anchor = scene.AnchorTopLeft
		}
		if !(!setTextInputCaret(uiInputState, transform, anchor, sprite.Width, sprite.Height)) {
			break
		}
	}
	return nil
}

func setTextInputCaret(uiInputState *input.State, transform *scene.UIGlobalTransform, anchor scene.Anchor, width, height float32) bool {
	minX, minY, maxX, maxY, ok := scene.BoundsInSpace2D(transform.Matrix, anchor, width, height, true)
	if !ok {
		return false
	}
	uiInputState.SetTextInputCaret(input.TextInputRect{X: minX, Y: minY, Width: maxX - minX, Height: maxY - minY})
	return true
}

func selectOnInputDoubleClick(world *ecs.World, state *uiEventState, entity ecs.Entity, x, y float32, now time.Time) bool {
	if state == nil {
		return false
	}
	textarea, _ := ecs.Get[Textarea](world, entity)
	if !isSingleLineTextInput(world, entity) && !textarea.SelectTokens {
		resetInputClick(state)
		return false
	}
	double := state.LastInputClick == entity && !state.LastInputClickAt.IsZero() && now.Sub(state.LastInputClickAt) <= inputDoubleClickInterval
	if textarea.SelectTokens {
		double = double && abs32(x-state.LastInputClickX) <= 4 && abs32(y-state.LastInputClickY) <= 4
	}
	state.LastInputClick = entity
	state.LastInputClickAt = now
	state.LastInputClickX, state.LastInputClickY = x, y
	if !double {
		return false
	}
	if field := ecs.GetMut[TextField](world, entity); field != nil {
		if textarea.SelectTokens {
			index, found := textIndexFromInput(world, entity, x, y, true)
			if !found {
				resetInputClick(state)
				return false
			}
			token := CodeTokenAt(field.Value, index)
			field.Anchor, field.Caret = token.RuneStart, token.RuneEnd
		} else {
			field.Anchor = 0
			field.Caret = runeCount(field.Value)
		}
	}
	resetInputClick(state)
	return true
}

func isSingleLineTextInput(world *ecs.World, entity ecs.Entity) bool {
	return ecs.Has[Input](world, entity) || ecs.Has[NumberInput](world, entity)
}

func resetInputClick(state *uiEventState) {
	if state != nil {
		state.LastInputClick = 0
		state.LastInputClickAt = time.Time{}
	}
}

type textEditsScratch struct{}

func editFocused(world *ecs.World, entity ecs.Entity, uiInputState *input.State) {
	if entity == 0 || uiInputState == nil || !scene.IsInteractive(world, entity) {
		return
	}
	if uiInputState.KeyPressed(input.KeyEscape) && !uiInputState.KeyPressedDuringComposition(input.KeyEscape) {
		if state, ok := ecs.GetResource[*uiEventState](world); ok && state != nil {
			focus(world, state, 0)
		}
		return
	}
	field := ecs.GetMut[TextField](world, entity)
	if field == nil {
		return
	}
	if target, ok := ecs.Get[scene.InputTarget2D](world, entity); ok && target.Disabled {
		// Disabled textareas may still be focused to scroll, but must remain
		// read-only.
		return
	}
	if control, ok := ecs.Get[html.Control](world, entity); ok && control.ReadOnly {
		if textarea := ecs.GetMut[Textarea](world, entity); textarea != nil && uiInputState.KeyPressed(input.KeyA) &&
			(uiInputState.KeyHeld(input.KeyLeftControl) || uiInputState.KeyHeld(input.KeyRightControl) || uiInputState.KeyHeld(input.KeyLeftSuper) || uiInputState.KeyHeld(input.KeyRightSuper)) {
			textarea.preserveScroll = true
		}
		editReadOnlyField(field, uiInputState)
		return
	}
	edits := ecs.FrameScratch[textEditsScratch, input.TextEdit](world)
	edits.Values = uiInputState.AppendTextEdits(edits.Values)
	defer edits.ReleaseUnused()
	field.Composition = uiInputState.CompositionText()
	shortcut := uiInputState.KeyHeld(input.KeyLeftControl) || uiInputState.KeyHeld(input.KeyRightControl) || uiInputState.KeyHeld(input.KeyLeftSuper) || uiInputState.KeyHeld(input.KeyRightSuper)
	if shortcut && uiInputState.KeyPressed(input.KeyA) {
		if textarea := ecs.GetMut[Textarea](world, entity); textarea != nil {
			textarea.preserveScroll = true
		}
	}
	pointer := uiInputState.Pointer()
	mouseSelecting := pointer.Device == input.PointerDeviceMouse && (pointer.Down || pointer.Released)
	if mouseSelecting {
		edits.Values = discardTextSelectionEdits(edits.Values)
	}
	_, editable := ecs.Get[html.Editable](world, entity)
	multiline := ecs.Has[Textarea](world, entity) || editable
	shift := uiInputState.KeyHeld(input.KeyLeftShift) || uiInputState.KeyHeld(input.KeyRightShift)
	textareaSubmits := 0
	if ecs.Has[Textarea](world, entity) && !ecs.Has[html.Control](world, entity) && !shift {
		edits.Values, textareaSubmits = textareaSubmitEdits(edits.Values)
	}
	previous := *field
	next := previous
	var submits int
	next.Value, next.Caret, next.Anchor, submits = editTextValue(next.Value, next.Caret, next.Anchor, next.MaxRunes, multiline, edits.Values, uiInputState, mouseSelecting, next.Buffer)
	submits += textareaSubmits
	if editable && next.Value != previous.Value {
		inputType, data := htmlInputEditMetadata(edits.Values)
		if dispatchHTMLEditableBeforeInput(world, entity, inputType, data) {
			if next.Buffer != nil {
				next.Buffer.Sync(previous.Value)
			}
			selectionStart, selectionEnd := orderedSelection(previous.Anchor, previous.Caret)
			uiInputState.SetTextInputSelection(selectionStart, selectionEnd)
			return
		}
		if state := ecs.GetMut[htmlEditableState](world, entity); state != nil {
			state.InputType, state.Data = inputType, data
		}
	}
	*field = next
	if field.Value != previous.Value {
		if number := ecs.GetMut[NumberInput](world, entity); number != nil {
			number.Changed = true
		}
		ecs.WriteMessage(world, InputEvent[string]{Entity: entity, Value: field.Value})
	}
	if submits > 0 {
		field.Submitted = true
		ecs.WriteMessage(world, SubmittedEvent[string]{Entity: entity, Value: field.Value})
		if ecs.Has[NumberInput](world, entity) {
			if state, ok := ecs.GetResource[*uiEventState](world); ok && state != nil {
				focus(world, state, 0)
			}
		}
	}
}

// Readonly controls still allow selection, keyboard navigation and copying.
// Mutation commands never change the value.
func editReadOnlyField(field *TextField, state *input.State) {
	shortcut := state.KeyHeld(input.KeyLeftControl) || state.KeyHeld(input.KeyRightControl) || state.KeyHeld(input.KeyLeftSuper) || state.KeyHeld(input.KeyRightSuper)
	if shortcut && state.KeyPressed(input.KeyA) {
		field.Anchor, field.Caret = 0, runeCount(field.Value)
		return
	}
	if shortcut && (state.KeyPressed(input.KeyC) || state.KeyPressed(input.KeyX)) {
		start, end := orderedSelection(field.Anchor, field.Caret)
		if start != end {
			state.SetClipboardText(runeSlice(field.Value, start, end))
		}
		return
	}
	extend := state.KeyHeld(input.KeyLeftShift) || state.KeyHeld(input.KeyRightShift)
	start, end := orderedSelection(field.Anchor, field.Caret)
	if state.KeyPressed(input.KeyLeft) {
		if start != end && !extend {
			field.Caret = start
		} else {
			field.Caret = max(0, field.Caret-1)
		}
	} else if state.KeyPressed(input.KeyRight) {
		if start != end && !extend {
			field.Caret = end
		} else {
			field.Caret = min(runeCount(field.Value), field.Caret+1)
		}
	} else {
		return
	}
	if !extend {
		field.Anchor = field.Caret
	}
}

func htmlInputEditMetadata(edits []input.TextEdit) (inputType, data string) {
	for _, edit := range edits {
		if edit.HasSelection {
			continue
		}
		if edit.Text != "" {
			if edit.DuringComposition {
				return "insertCompositionText", edit.Text
			}
			return "insertText", edit.Text
		}
		switch edit.Command {
		case input.TextCommandBackspace:
			return "deleteContentBackward", ""
		case input.TextCommandEnter:
			return "insertParagraph", ""
		}
	}
	return "insertText", ""
}

func textareaSubmitEdits(edits []input.TextEdit) ([]input.TextEdit, int) {
	kept := edits[:0]
	submits := 0
	for _, edit := range edits {
		if edit.DuringComposition {
			kept = append(kept, edit)
			continue
		}
		if edit.Command == input.TextCommandEnter {
			submits++
			continue
		}
		if edit.Text != "" {
			count := strings.Count(edit.Text, "\n")
			if count > 0 {
				submits += count
				edit.Text = strings.ReplaceAll(edit.Text, "\n", "")
				if edit.Text == "" && !edit.HasSelection {
					continue
				}
			}
		}
		kept = append(kept, edit)
	}
	return kept, submits
}

func editTextValue(value string, caret, anchor, maxRunes int, multiline bool, edits []input.TextEdit, state *input.State, mouseSelecting bool, buffer *input.TextBuffer) (string, int, int, int) {
	apply := state.ApplyTextEditsInRange
	if buffer != nil {
		apply = buffer.ApplyTextEditsInRange
	}
	length := 0
	if buffer != nil {
		buffer.Sync(value)
		length = buffer.Runes()
	} else {
		length = runeCount(value)
	}
	if !mouseSelecting {
		if selection, ok := state.TextSelection(); ok {
			anchor, caret = selection.Start, selection.End
		}
	}
	caret = max(0, min(caret, length))
	anchor = max(0, min(anchor, length))
	shortcut := state.KeyHeld(input.KeyLeftControl) || state.KeyHeld(input.KeyRightControl) || state.KeyHeld(input.KeyLeftSuper) || state.KeyHeld(input.KeyRightSuper)
	start, end := orderedSelection(anchor, caret)
	if shortcut && state.KeyPressed(input.KeyA) {
		return value, length, 0, 0
	}
	if shortcut && state.KeyPressed(input.KeyC) {
		if start != end {
			state.SetClipboardText(runeSlice(value, start, end))
		}
		return value, caret, anchor, 0
	}
	if shortcut && state.KeyPressed(input.KeyX) {
		if start != end && state.SetClipboardText(runeSlice(value, start, end)) {
			edits = prependTextEdit(edits, input.TextEdit{Command: input.TextCommandBackspace})
		}
	}
	if shortcut && state.KeyPressed(input.KeyV) {
		if pasted, ok := state.ClipboardText(); ok {
			edits = prependTextEdit(edits, input.TextEdit{Text: pasted})
		}
	}
	if state.Composing() {
		compositionCaret, compositionAnchor := caret, anchor
		value, caret, submits := apply(value, start, end, maxRunes, multiline, edits)
		// Composition updates do not commit text. Keep the selected range intact
		// so the following IME commit replaces it instead of appending after it.
		committed := false
		for _, edit := range edits {
			if edit.Text != "" || (edit.Command != 0 && !edit.DuringComposition) {
				committed = true
				break
			}
		}
		if !committed {
			return value, compositionCaret, compositionAnchor, submits
		}
		return value, caret, caret, submits
	}
	extend := state.KeyHeld(input.KeyLeftShift) || state.KeyHeld(input.KeyRightShift)
	if state.KeyPressed(input.KeyLeft) {
		if start != end && !extend {
			caret = start
		} else {
			caret = max(caret-1, 0)
		}
		if !extend {
			anchor = caret
		}
	}
	if state.KeyPressed(input.KeyRight) {
		if start != end && !extend {
			caret = end
		} else {
			caret = min(caret+1, length)
		}
		if !extend {
			anchor = caret
		}
	}
	start, end = orderedSelection(anchor, caret)
	next, editedCaret, submits := apply(value, start, end, maxRunes, multiline, edits)
	value = next
	if textEditsChangeValue(edits) {
		caret = editedCaret
		anchor = caret
	}
	selectionStart, selectionEnd := orderedSelection(anchor, caret)
	state.SetTextInputSelection(selectionStart, selectionEnd)
	return value, caret, anchor, submits
}

func prependTextEdit(edits []input.TextEdit, edit input.TextEdit) []input.TextEdit {
	if len(edits) == cap(edits) {
		result := make([]input.TextEdit, len(edits)+1)
		result[0] = edit
		copy(result[1:], edits)
		return result
	}
	edits = edits[:len(edits)+1]
	copy(edits[1:], edits[:len(edits)-1])
	edits[0] = edit
	return edits
}

func textEditsChangeValue(edits []input.TextEdit) bool {
	for _, edit := range edits {
		if !edit.HasSelection && (edit.Text != "" || edit.Command != 0) {
			return true
		}
	}
	return false
}

func discardTextSelectionEdits(edits []input.TextEdit) []input.TextEdit {
	kept := edits[:0]
	for _, edit := range edits {
		if edit.HasSelection && edit.Text == "" && edit.Command == 0 {
			continue
		}
		kept = append(kept, edit)
	}
	return kept
}

func orderedSelection(anchor, caret int) (int, int) {
	if anchor > caret {
		return caret, anchor
	}
	return anchor, caret
}

func setCaretFromInput(world *ecs.World, owner ecs.Entity, screenX, screenY float32, extend bool) {
	if caret, found := textIndexFromInput(world, owner, screenX, screenY, false); found {
		setCaret(world, owner, caret, extend)
	}
}

// Character hit testing uses the glyph under the pointer instead of rounding
// its right half to the next caret (which may belong to a different token).
func textIndexFromInput(world *ecs.World, owner ecs.Entity, screenX, screenY float32, character bool) (int, bool) {
	for entity, access := range ecs.Query3[Relation[scene.Text2D], scene.Text2D, scene.UIGlobalTransform](world).All {
		visualAccess := access.A
		textAccess := access.B
		transformAccess := access.C

		visual, transform := visualAccess.Ref(), transformAccess.Ref()
		if visual.Owner != owner || !ecs.Has[Part[ValuePart]](world, entity) {
			continue
		}
		x, y := inputCoordsForUI(screenX, screenY)
		local := transform.Matrix.Inv().Mul4x1(mgl32.Vec4{x, y, 0, 1})
		value := textFieldValue(world, owner)
		if field, ok := ecs.Get[TextField](world, owner); ok && field.Password {
			value = maskedTextFieldValue(world, owner, field)
		}
		text := textAccess.Mut()
		caret := TextCaretIndex(text, value, local.X(), local.Y())
		if character && caret > 0 {
			cx, cy, _, height, ok := scene.TextCaretRect2D(text, value, caret)
			if ok && (local.Y() < cy || local.Y() < cy+height && local.X() < cx) {
				caret--
			}
		}
		return caret, true
	}
	return 0, false
}

// CodeToken describes a code token in byte and rune offsets. Offset is the byte
// position of the requested character; Quoted identifies string contents.
type CodeToken struct {
	Start, End, Offset int
	RuneStart, RuneEnd int
	Quoted             bool
}

// CodeTokenAt finds the token at a rune index in one allocation-free scan. Words
// include Unicode letters, digits and combining marks. Quoted contents stay
// together, excluding their delimiters; other delimiters stay separate.
func CodeTokenAt(value string, index int) (token CodeToken) {
	index = max(0, index)
	var previous uint8
	var quote rune
	var comment uint8 // 1: line, 2: block, 3: closing block slash
	var escaped bool
	quoteStart, quoteByteStart := 0, 0
	for next := 0; next < len(value); {
		offset := next
		r, size := utf8.DecodeRuneInString(value[offset:])
		next += size
		if token.RuneEnd == index {
			token.Offset = offset
		}
		if quote != 0 {
			switch {
			case r == quote && !escaped:
				if index < token.RuneEnd {
					token.RuneStart, token.Start, token.Quoted = quoteStart, quoteByteStart, true
					return
				}
				if index == token.RuneEnd {
					token.RuneStart, token.Start = token.RuneEnd, offset
					token.RuneEnd++
					token.End = next
					return
				}
				quote, previous = 0, 0
				token.RuneEnd++
				token.RuneStart, token.Start, token.End = token.RuneEnd, next, next
				continue
			case quote != '`' && (r == '\n' || r == '\r') && !escaped:
				if index < token.RuneEnd {
					token.RuneStart, token.Start, token.Quoted = quoteStart, quoteByteStart, true
					return
				}
				quote, previous = 0, 0
				token.RuneStart, token.Start = token.RuneEnd, offset
			default:
				escaped = quote != '`' && r == '\\' && !escaped
				token.RuneEnd++
				token.End = next
				continue
			}
		}
		// Quotes inside comments do not open strings. Keep the usual word
		// boundaries within comments so their text remains selectable.
		switch comment {
		case 1:
			if r == '\n' || r == '\r' {
				comment = 0
			}
		case 2:
			if r == '*' && offset+1 < len(value) && value[offset+1] == '/' {
				comment = 3
			}
		case 3:
			comment = 0
		default:
			if r == '/' && offset+1 < len(value) {
				switch value[offset+1] {
				case '/':
					comment = 1
				case '*':
					comment = 2
				}
			}
		}
		class := codeTokenClass(r)
		if token.RuneEnd == 0 || class == 0 || class != previous {
			if token.RuneEnd > index {
				return
			}
			token.RuneStart, token.Start = token.RuneEnd, offset
		}
		if comment == 0 && (r == '"' || r == '\'' || r == '`') {
			quote, quoteStart, quoteByteStart, escaped = r, token.RuneEnd+1, next, false
		}
		token.RuneEnd++
		token.End = next
		if class == 0 && token.RuneEnd > index {
			return
		}
		previous = class
	}
	if quote != 0 && index < token.RuneEnd {
		token.RuneStart, token.Start, token.Quoted = quoteStart, quoteByteStart, true
	} else if index >= token.RuneEnd {
		token.RuneStart, token.Start, token.Offset = token.RuneEnd, token.End, token.End
	}
	return
}

func codeTokenClass(r rune) uint8 {
	switch {
	case r == '_' || r == '$' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r):
		return 1
	case r != '\n' && r != '\r' && unicode.IsSpace(r):
		return 2
	case strings.ContainsRune("+-*/%=!<>:&|^~?", r):
		return 3
	default:
		return 0
	}
}

func textFieldValue(world *ecs.World, entity ecs.Entity) string {
	if field, ok := ecs.Get[TextField](world, entity); ok {
		return field.Value
	}
	return ""
}

func setCaret(world *ecs.World, entity ecs.Entity, caret int, extend bool) {
	if field := ecs.GetMut[TextField](world, entity); field != nil {
		field.Caret = max(0, min(caret, runeCount(field.Value)))
		if !extend {
			field.Anchor = field.Caret
		}
	}
}

func setSelecting(world *ecs.World, entity ecs.Entity, selecting bool) {
	if field := ecs.GetMut[TextField](world, entity); field != nil {
		field.Selecting = selecting
	}
}

func textFieldSelecting(world *ecs.World, entity ecs.Entity) bool {
	if field, ok := ecs.Get[TextField](world, entity); ok {
		return field.Selecting
	}
	return false
}

const (
	textareaScrollAxisHorizontal uint8 = 1
	textareaScrollAxisVertical   uint8 = 2
)

type textareaScrollGeometry struct {
	trackX         float32
	trackY         float32
	viewportWidth  float32
	viewportHeight float32
	contentWidth   float32
	contentHeight  float32
	trackWidth     float32
	trackHeight    float32
	horizontalY    float32
	verticalX      float32
	barSize        float32
	horizontal     bool
	vertical       bool
}

func beginTextareaScrollDrag(world *ecs.World, owner ecs.Entity, screenX, screenY float32) bool {
	geometry, ok := textareaScrollGeometryFor(world, owner)
	if !ok {
		return false
	}
	localX, localY, ok := textareaLocalPoint(world, owner, screenX, screenY)
	if !ok {
		return false
	}
	axis := textareaScrollAxisAt(geometry, localX, localY)
	if axis == 0 {
		return false
	}
	textarea := ecs.GetMut[Textarea](world, owner)
	if textarea == nil {
		return false
	}
	textarea.scrollDragging = true
	textarea.scrollDragAxis = axis
	if axis == textareaScrollAxisHorizontal {
		textarea.scrollDragPointer = localX
		textarea.scrollDragOffset = textarea.ScrollX
		if !textareaScrollThumbContains(geometry.trackWidth, geometry.viewportWidth, geometry.contentWidth, textarea.ScrollX, localX-geometry.trackX) {
			textarea.ScrollX = textareaScrollAtPointer(geometry.trackWidth, geometry.viewportWidth, geometry.contentWidth, localX-geometry.trackX)
			textarea.scrollDragOffset = textarea.ScrollX
		}
	} else {
		textarea.scrollDragPointer = localY
		textarea.scrollDragOffset = textarea.ScrollY
		if !textareaScrollThumbContains(geometry.trackHeight, geometry.viewportHeight, geometry.contentHeight, textarea.ScrollY, localY-geometry.trackY) {
			textarea.ScrollY = textareaScrollAtPointer(geometry.trackHeight, geometry.viewportHeight, geometry.contentHeight, localY-geometry.trackY)
			textarea.scrollDragOffset = textarea.ScrollY
		}
	}
	return true
}

// TextareaScrollBarAt reports whether a screen point is inside a visible textarea scrollbar.
func TextareaScrollBarAt(world *ecs.World, screenX, screenY float32) bool {
	if world == nil {
		return false
	}
	found := false
	for entity, access := range ecs.Query[Textarea](world).All {
		textarea := access.Ref()

		if textarea == nil || !hierarchyVisible(world, entity) {
			continue
		}
		control, ok := controlViewFor(world, entity)
		target, targetOK := ecs.Get[scene.InputTarget2D](world, entity)
		if !ok || (control.Disabled && (!targetOK || !target.AllowDisabledInput)) {
			continue
		}
		geometry, ok := textareaScrollGeometryFor(world, entity)
		if !ok {
			continue
		}
		localX, localY, ok := textareaLocalPoint(world, entity, screenX, screenY)
		if ok && textareaScrollAxisAt(geometry, localX, localY) != 0 {
			found = true
			break
		}
	}
	return found
}

func updateTextareaScrollDrags(world *ecs.World, uiInputState *input.State) {
	if uiInputState == nil {
		return
	}
	pointer := uiInputState.Pointer()
	for entity, row := range ecs.Query[Textarea](world).All {
		if !func(entity ecs.Entity, access ecs.ComponentAccess[Textarea]) bool {
			if !access.Ref().scrollDragging {
				return true
			}
			next := access.Get()
			textarea := &next
			defer func() { ecs.CommitComponent(access, next) }()
			if pointer.Released || !pointer.Down {
				textarea.scrollDragging = false
				textarea.scrollDragAxis = 0
				return true
			}
			geometry, ok := textareaScrollGeometryFor(world, entity)
			if !ok {
				return true
			}
			localX, localY, ok := textareaLocalPoint(world, entity, pointer.X, pointer.Y)
			if !ok {
				return true
			}
			if textarea.scrollDragAxis == textareaScrollAxisHorizontal {
				available := geometry.trackWidth - scrollThumbSize(geometry.trackWidth, geometry.viewportWidth, geometry.contentWidth, 20)
				if available > 0 {
					textarea.ScrollX = min(max(textarea.scrollDragOffset+(localX-textarea.scrollDragPointer)*max(geometry.contentWidth-geometry.viewportWidth, 0)/available, 0), max(geometry.contentWidth-geometry.viewportWidth, 0))
				}
			} else if textarea.scrollDragAxis == textareaScrollAxisVertical {
				available := geometry.trackHeight - scrollThumbSize(geometry.trackHeight, geometry.viewportHeight, geometry.contentHeight, 20)
				if available > 0 {
					textarea.ScrollY = min(max(textarea.scrollDragOffset+(localY-textarea.scrollDragPointer)*max(geometry.contentHeight-geometry.viewportHeight, 0)/available, 0), max(geometry.contentHeight-geometry.viewportHeight, 0))
				}
			}
			return true
		}(entity, row) {
			break
		}
	}
}

func textareaScrollGeometryFor(world *ecs.World, owner ecs.Entity) (textareaScrollGeometry, bool) {
	control, ok := controlViewFor(world, owner)
	if !ok {
		return textareaScrollGeometry{}, false
	}
	textarea, ok := ecs.Get[Textarea](world, owner)
	if !ok {
		return textareaScrollGeometry{}, false
	}
	textEntity, ok := FindPart[ValuePart, scene.Text2D](world, owner)
	if !ok {
		return textareaScrollGeometry{}, false
	}
	text := ecs.GetRef[scene.Text2D](world, textEntity)
	if text == nil {
		return textareaScrollGeometry{}, false
	}
	contentWidth, contentHeight, measured := scene.MeasureText2D(text)
	if !measured {
		return textareaScrollGeometry{}, false
	}
	_, _, viewportWidth, viewportHeight := textareaContentBox(world, textEntity, &textarea, control)
	barSize := textareaScrollBarSize(&textarea)
	box := scene.TextRangeRect2D{Width: control.Width, Height: control.Height}
	if authored, ok := ecs.Get[html.Control](world, owner); ok {
		box = authored.PaddingBox
	}
	horizontal, vertical := !textarea.DisableHorizontalScroll && contentWidth > viewportWidth, contentHeight > viewportHeight
	trackWidth, trackHeight := box.Width, box.Height
	if vertical {
		trackWidth = max(trackWidth-barSize, 0)
	}
	if horizontal {
		trackHeight = max(trackHeight-barSize, 0)
	}
	return textareaScrollGeometry{
		trackX: box.X, trackY: box.Y,
		viewportWidth: viewportWidth, viewportHeight: viewportHeight,
		contentWidth: contentWidth, contentHeight: contentHeight,
		trackWidth: trackWidth, trackHeight: trackHeight,
		horizontalY: box.Y + box.Height - barSize, verticalX: box.X + box.Width - barSize,
		barSize: barSize, horizontal: horizontal, vertical: vertical,
	}, true
}

func textareaLocalPoint(world *ecs.World, owner ecs.Entity, screenX, screenY float32) (float32, float32, bool) {
	transform, ok := ecs.Get[scene.UIGlobalTransform](world, owner)
	if !ok {
		return 0, 0, false
	}
	local := transform.Matrix.Inv().Mul4x1(mgl32.Vec4{screenX, screenY, 0, 1})
	if local.W() == 0 {
		return 0, 0, false
	}
	return local.X() / local.W(), local.Y() / local.W(), true
}

func textareaScrollAxisAt(geometry textareaScrollGeometry, x, y float32) uint8 {
	if geometry.barSize <= 0 {
		return 0
	}
	if geometry.horizontal && x >= geometry.trackX && x <= geometry.trackX+geometry.trackWidth && y >= geometry.horizontalY && y <= geometry.horizontalY+geometry.barSize {
		return textareaScrollAxisHorizontal
	}
	if geometry.vertical && x >= geometry.verticalX && x <= geometry.verticalX+geometry.barSize && y >= geometry.trackY && y <= geometry.trackY+geometry.trackHeight {
		return textareaScrollAxisVertical
	}
	return 0
}

func textareaScrollThumbContains(trackSize, viewportSize, contentSize, scroll, pointer float32) bool {
	thumbSize := scrollThumbSize(trackSize, viewportSize, contentSize, 20)
	return pointer >= scrollThumbOffset(trackSize, thumbSize, scroll, max(contentSize-viewportSize, 0)) && pointer <= scrollThumbOffset(trackSize, thumbSize, scroll, max(contentSize-viewportSize, 0))+thumbSize
}

func textareaScrollAtPointer(trackSize, viewportSize, contentSize, pointer float32) float32 {
	thumbSize := scrollThumbSize(trackSize, viewportSize, contentSize, 20)
	maximum := max(contentSize-viewportSize, 0)
	available := trackSize - thumbSize
	if maximum <= 0 || available <= 0 {
		return 0
	}
	return min(max((pointer-thumbSize*.5)*maximum/available, 0), maximum)
}

func TextCaretIndex(text *scene.Text2D, value string, x, y float32) int {
	return scene.TextCaretIndex2D(text, value, x, y)
}

func stepNumberInputValue(value string, input NumberInput, direction int) (string, bool) {
	return StepNumberInputValue(value, input, direction)
}

func StepNumberInputValue(value string, input NumberInput, direction int) (string, bool) {
	if direction == 0 {
		return value, false
	}
	step := input.Step
	if step == 0 {
		step = 1
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		trimmed = "0"
	}
	current, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return value, false
	}
	next := current + step*float64(direction)
	if input.Integer {
		return strconv.FormatInt(int64(math.Round(next)), 10), true
	}
	return formatNumberInputFloat(next, step), true
}

func NormalizeNumberInputValue(value string, input NumberInput) (string, bool) {
	current, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return value, false
	}
	if input.Integer {
		return strconv.FormatInt(int64(math.Round(current)), 10), true
	}
	step := input.Step
	if step == 0 {
		step = 1
	}
	return formatNumberInputFloat(current, step), true
}

func formatNumberInputFloat(value, step float64) string {
	precision := numberInputFractionDigits(step)
	formatted := strconv.FormatFloat(value, 'f', precision, 64)
	if precision == 0 {
		if formatted == "-0" {
			return "0"
		}
		return formatted
	}
	formatted = strings.TrimRight(formatted, "0")
	formatted = strings.TrimRight(formatted, ".")
	if formatted == "-0" {
		return "0"
	}
	return formatted
}

func numberInputFractionDigits(step float64) int {
	fraction := strconv.FormatFloat(math.Abs(step), 'f', -1, 64)
	dot := strings.IndexByte(fraction, '.')
	if dot < 0 {
		return 0
	}
	fraction = strings.TrimRight(fraction[dot+1:], "0")
	leadingZeros := 0
	for leadingZeros < len(fraction) && fraction[leadingZeros] == '0' {
		leadingZeros++
	}
	return min(len(fraction), leadingZeros+6)
}
