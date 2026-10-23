package input

import "strings"

type PointerDevice uint8

const (
	PointerDeviceNone PointerDevice = iota
	PointerDeviceMouse
	PointerDeviceTouch
)

type Pointer struct {
	Device         PointerDevice
	X, Y           float32
	PressX, PressY float32
	DeltaX, DeltaY float32
	WheelX, WheelY float32
	Down           bool
	Pressed        bool
	Released       bool
}

type State struct {
	downKeys        map[KeyCode]struct{}
	pressedKeys     map[KeyCode]struct{}
	releasedKeys    map[KeyCode]struct{}
	downMouse       map[MouseButton]struct{}
	pressedMouse    map[MouseButton]struct{}
	releasedMouse   map[MouseButton]struct{}
	pointer         Pointer
	textEdits       []TextEdit
	composition     string
	composing       bool
	compositionKeys map[KeyCode]struct{}
	textConfig      TextInputConfig
	textRevision    uint64
	clipboardWrite  func(string) bool
	clipboardRead   func() (string, bool)
	textSelection   TextSelection
	hasSelection    bool
	textBuffer      TextBuffer
	events          []Event
	nextEventSeq    uint64
	consumedEvents  map[uint64]struct{}
}

func (s *State) SetClipboard(write func(string) bool, read func() (string, bool)) {
	if s == nil {
		return
	}
	s.clipboardWrite, s.clipboardRead = write, read
}

func (s *State) SetClipboardText(text string) bool {
	return s != nil && s.clipboardWrite != nil && s.clipboardWrite(text)
}

func (s *State) ClipboardText() (string, bool) {
	if s == nil || s.clipboardRead == nil {
		return "", false
	}
	return s.clipboardRead()
}

func (s *State) ApplyTextSelection(selection TextSelection) {
	if s == nil {
		return
	}
	s.textSelection, s.hasSelection = selection, true
	s.textEdits = append(s.textEdits, TextEdit{SelectionStart: selection.Start, SelectionEnd: selection.End, HasSelection: true})
}

func (s *State) TextSelection() (TextSelection, bool) {
	if s == nil {
		return TextSelection{}, false
	}
	return s.textSelection, s.hasSelection
}

func NewState() *State {
	return &State{
		downKeys:        map[KeyCode]struct{}{},
		pressedKeys:     map[KeyCode]struct{}{},
		releasedKeys:    map[KeyCode]struct{}{},
		downMouse:       map[MouseButton]struct{}{},
		pressedMouse:    map[MouseButton]struct{}{},
		releasedMouse:   map[MouseButton]struct{}{},
		compositionKeys: map[KeyCode]struct{}{},
	}
}

// CopyFrom reuses storage for an isolated snapshot of keyboard and pointer state.
func (s *State) CopyFrom(source *State) {
	copyKeys := func(dst, src map[KeyCode]struct{}) {
		clear(dst)
		for key := range src {
			dst[key] = struct{}{}
		}
	}
	copyMouse := func(dst, src map[MouseButton]struct{}) {
		clear(dst)
		for key := range src {
			dst[key] = struct{}{}
		}
	}
	copyKeys(s.downKeys, source.downKeys)
	copyKeys(s.pressedKeys, source.pressedKeys)
	copyKeys(s.releasedKeys, source.releasedKeys)
	copyMouse(s.downMouse, source.downMouse)
	copyMouse(s.pressedMouse, source.pressedMouse)
	copyMouse(s.releasedMouse, source.releasedMouse)
	s.pointer = source.pointer
	s.clipboardWrite, s.clipboardRead = source.clipboardWrite, source.clipboardRead
}

// Enqueue records a normalized platform event for the next input stage.
func (s *State) Enqueue(event Event) {
	if s == nil {
		return
	}
	s.nextEventSeq++
	event.Seq = s.nextEventSeq
	s.events = append(s.events, event)
}

// DrainEvents returns events in platform arrival order. The returned slice is
// owned by the caller until the next Enqueue call.
func (s *State) DrainEvents() []Event {
	if s == nil || len(s.events) == 0 {
		return nil
	}
	events := s.events
	s.events = s.events[:0]
	return events
}

func (s *State) HasPendingEvents() bool {
	return s != nil && len(s.events) != 0
}

// ConsumeEvent prevents later input stages from handling the same event.
func (s *State) ConsumeEvent(sequence uint64) {
	if s == nil || sequence == 0 {
		return
	}
	if s.consumedEvents == nil {
		s.consumedEvents = map[uint64]struct{}{}
	}
	s.consumedEvents[sequence] = struct{}{}
}

func (s *State) EventConsumed(sequence uint64) bool {
	if s == nil || sequence == 0 {
		return false
	}
	_, ok := s.consumedEvents[sequence]
	return ok
}

// ApplyEvent derives the queryable input state from one normalized event.
func (s *State) ApplyEvent(event Event) {
	if s == nil {
		return
	}
	switch event.Kind {
	case EventKeyDown:
		s.PressKey(event.Key)
	case EventKeyUp:
		s.ReleaseKey(event.Key)
	case EventTextInput:
		s.CommitText(event.Text)
	case EventTextComposition:
		s.ApplyTextComposition(event.Composition)
	case EventTextCommand:
		s.PushTextCommand(event.Command)
	case EventTextSelection:
		s.ApplyTextSelection(event.Selection)
	case EventMouseDown:
		s.PressMouse(MouseButton(event.Button), event.X, event.Y)
	case EventMouseUp:
		s.ReleaseMouse(MouseButton(event.Button), event.X, event.Y)
	case EventMouseMove:
		s.MoveMouse(event.X, event.Y)
	case EventMouseWheel:
		s.ScrollMouse(event.WheelX, event.WheelY)
	case EventTouchStart:
		s.TouchStart(event.X, event.Y)
	case EventTouchMove:
		s.TouchMove(event.X, event.Y)
	case EventTouchEnd:
		s.TouchEnd(event.X, event.Y)
	}
}

func (s *State) EndFrame() {
	if s == nil {
		return
	}
	clear(s.pressedKeys)
	clear(s.releasedKeys)
	clear(s.pressedMouse)
	clear(s.releasedMouse)
	clear(s.compositionKeys)
	clear(s.consumedEvents)
	s.textEdits = s.textEdits[:0]
	s.hasSelection = false
	s.pointer.DeltaX = 0
	s.pointer.DeltaY = 0
	s.pointer.WheelX = 0
	s.pointer.WheelY = 0
	s.pointer.Pressed = false
	s.pointer.Released = false
}

func (s *State) PressKey(key KeyCode) {
	if s == nil {
		return
	}
	pressButton(s.downKeys, s.pressedKeys, key)
	if s.composing {
		s.compositionKeys[key] = struct{}{}
	}
}

func (s *State) ReleaseKey(key KeyCode) {
	if s == nil {
		return
	}
	releaseButton(s.downKeys, s.releasedKeys, key)
}

func (s *State) KeyHeld(key KeyCode) bool {
	return hasButton(s.downKeys, key)
}

func (s *State) KeyPressed(key KeyCode) bool {
	return hasButton(s.pressedKeys, key)
}

func (s *State) KeyPressedDuringComposition(key KeyCode) bool {
	return s != nil && hasButton(s.compositionKeys, key)
}

func (s *State) CompositionHandledKey() bool {
	return s != nil && len(s.compositionKeys) > 0
}

func (s *State) KeyReleased(key KeyCode) bool {
	return hasButton(s.releasedKeys, key)
}

func (s *State) PressMouse(button MouseButton, x, y float64) {
	if s == nil {
		return
	}
	pressButton(s.downMouse, s.pressedMouse, button)
	if button == MouseLeft {
		s.pressPointer(PointerDeviceMouse, float32(x), float32(y))
		return
	}
	s.snapPointer(PointerDeviceMouse, float32(x), float32(y))
}

func (s *State) ReleaseMouse(button MouseButton, x, y float64) {
	if s == nil {
		return
	}
	releaseButton(s.downMouse, s.releasedMouse, button)
	if button == MouseLeft {
		s.releasePointer(PointerDeviceMouse, float32(x), float32(y))
		return
	}
	s.snapPointer(PointerDeviceMouse, float32(x), float32(y))
}

func (s *State) MoveMouse(x, y float64) {
	if s == nil {
		return
	}
	s.movePointer(PointerDeviceMouse, float32(x), float32(y))
}

func (s *State) ScrollMouse(xoff, yoff float64) {
	if s == nil {
		return
	}
	if s.pointer.Device == PointerDeviceNone {
		s.pointer.Device = PointerDeviceMouse
	}
	s.pointer.WheelX += float32(xoff)
	s.pointer.WheelY += float32(yoff)
}

func (s *State) MouseHeld(button MouseButton) bool {
	return hasButton(s.downMouse, button)
}

func (s *State) MousePressed(button MouseButton) bool {
	return hasButton(s.pressedMouse, button)
}

func (s *State) MouseReleased(button MouseButton) bool {
	return hasButton(s.releasedMouse, button)
}

func (s *State) TouchStart(x, y float64) {
	if s == nil {
		return
	}
	s.pressPointer(PointerDeviceTouch, float32(x), float32(y))
}

func (s *State) TouchMove(x, y float64) {
	if s == nil {
		return
	}
	s.movePointer(PointerDeviceTouch, float32(x), float32(y))
}

func (s *State) TouchEnd(x, y float64) {
	if s == nil {
		return
	}
	s.releasePointer(PointerDeviceTouch, float32(x), float32(y))
}

func (s *State) Pointer() Pointer {
	if s == nil {
		return Pointer{}
	}
	return s.pointer
}

func (s *State) ConsumePointerWheel() {
	if s == nil {
		return
	}
	s.pointer.WheelX = 0
	s.pointer.WheelY = 0
}

func (s *State) PointerHeld() bool {
	return s.Pointer().Down
}

func (s *State) PointerPressed() bool {
	return s.Pointer().Pressed
}

func (s *State) PointerReleased() bool {
	return s.Pointer().Released
}

func (s *State) CommitText(text string) {
	if s == nil {
		return
	}
	s.composing = false
	s.composition = ""
	if text = sanitizeText(text, s.textConfig.Multiline); text != "" {
		s.textEdits = append(s.textEdits, TextEdit{Text: text})
	}
}

func (s *State) TextEdits() []TextEdit {
	return s.AppendTextEdits(nil)
}

// AppendTextEdits appends this frame's effective text edits to dst.
func (s *State) AppendTextEdits(dst []TextEdit) []TextEdit {
	if s == nil {
		return dst
	}
	start := len(dst)
	edits := append(dst, s.textEdits...)
	hasBackspace := false
	hasEnter := false
	for i := start; i < len(edits); i++ {
		if edits[i].Command != 0 && s.composing {
			edits[i].DuringComposition = true
		}
		edit := edits[i]
		hasBackspace = hasBackspace || edit.Command == TextCommandBackspace
		hasEnter = hasEnter || edit.Command == TextCommandEnter || strings.ContainsRune(edit.Text, '\n')
	}
	if !hasBackspace && s.KeyPressed(KeyBackspace) {
		edits = append(edits, TextEdit{Command: TextCommandBackspace, DuringComposition: s.composing || s.KeyPressedDuringComposition(KeyBackspace)})
	}
	if !hasEnter && s.KeyPressed(KeyEnter) {
		edits = append(edits, TextEdit{Command: TextCommandEnter, DuringComposition: s.composing || s.KeyPressedDuringComposition(KeyEnter)})
	}
	return edits
}

func (s *State) ApplyTextComposition(event TextComposition) {
	if s == nil {
		return
	}
	s.composing = event.Active
	if event.Active {
		s.composition = sanitizeText(event.Text, s.textConfig.Multiline)
		return
	}
	s.composition = ""
}

func (s *State) CompositionText() string {
	if s == nil {
		return ""
	}
	return s.composition
}

func (s *State) Composing() bool {
	return s != nil && s.composing
}

func (s *State) PushTextCommand(command TextCommand) {
	if s == nil || command == 0 {
		return
	}
	s.textEdits = append(s.textEdits, TextEdit{Command: command, DuringComposition: s.composing})
}

func (s *State) FocusTextInput(kind TextInputKind, multiline bool, initialValue string) {
	if s == nil {
		return
	}
	s.clearTextInputEvents()
	s.textRevision++
	s.textConfig = TextInputConfig{
		Revision:     s.textRevision,
		Kind:         kind,
		InitialValue: sanitizeText(initialValue, multiline),
		Active:       true,
		Multiline:    multiline,
	}
	s.ApplyTextComposition(TextComposition{})
}

func (s *State) SetTextInputCaret(caret TextInputRect) {
	if s == nil || !s.textConfig.Active {
		return
	}
	s.textConfig.Caret = caret
}

func (s *State) SetTextInputSelection(start, end int) {
	if s == nil || !s.textConfig.Active {
		return
	}
	s.textConfig.SelectionStart = max(0, start)
	s.textConfig.SelectionEnd = max(0, end)
}

func (s *State) BlurTextInput() {
	if s == nil {
		return
	}
	s.clearTextInputEvents()
	if s.textConfig.Active {
		s.textRevision++
	}
	s.textConfig = TextInputConfig{Revision: s.textRevision}
	s.ApplyTextComposition(TextComposition{})
}

func (s *State) TextInputConfig() TextInputConfig {
	if s == nil {
		return TextInputConfig{}
	}
	return s.textConfig
}

func (s *State) clearTextInputEvents() {
	s.textEdits = s.textEdits[:0]
	clear(s.compositionKeys)
}

func (s *State) pressPointer(device PointerDevice, x, y float32) {
	s.pointer.Device = device
	s.pointer.X = x
	s.pointer.Y = y
	s.pointer.PressX = x
	s.pointer.PressY = y
	s.pointer.DeltaX = 0
	s.pointer.DeltaY = 0
	if !s.pointer.Down {
		s.pointer.Pressed = true
	}
	s.pointer.Down = true
}

func (s *State) releasePointer(device PointerDevice, x, y float32) {
	if s.pointer.Device == device {
		s.pointer.DeltaX += x - s.pointer.X
		s.pointer.DeltaY += y - s.pointer.Y
	}
	s.pointer.Device = device
	s.pointer.X = x
	s.pointer.Y = y
	if s.pointer.Down {
		s.pointer.Released = true
	}
	s.pointer.Down = false
}

func (s *State) movePointer(device PointerDevice, x, y float32) {
	if s.pointer.Device != device {
		s.pointer.Device = device
		s.pointer.X = x
		s.pointer.Y = y
		return
	}
	s.pointer.DeltaX += x - s.pointer.X
	s.pointer.DeltaY += y - s.pointer.Y
	s.pointer.X = x
	s.pointer.Y = y
}

func (s *State) snapPointer(device PointerDevice, x, y float32) {
	s.pointer.Device = device
	s.pointer.X = x
	s.pointer.Y = y
	s.pointer.DeltaX = 0
	s.pointer.DeltaY = 0
}

func pressButton[T comparable](down, pressed map[T]struct{}, value T) {
	if _, ok := down[value]; !ok {
		pressed[value] = struct{}{}
	}
	down[value] = struct{}{}
}

func releaseButton[T comparable](down, released map[T]struct{}, value T) {
	if _, ok := down[value]; ok {
		delete(down, value)
		released[value] = struct{}{}
	}
}

func hasButton[T comparable](set map[T]struct{}, value T) bool {
	_, ok := set[value]
	return ok
}
