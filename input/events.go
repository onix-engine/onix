package input

// EventKind identifies one normalized platform input event.
type EventKind uint8

const (
	EventKeyDown EventKind = iota + 1
	EventKeyUp
	EventTextInput
	EventTextComposition
	EventTextCommand
	EventTextSelection
	EventMouseDown
	EventMouseUp
	EventMouseMove
	EventMouseWheel
	EventTouchStart
	EventTouchMove
	EventTouchEnd
	// EventFilesDropped reports one file dropped onto the window.
	EventFilesDropped
)

type Phase uint8

const (
	PhaseGlobal Phase = iota
	PhaseGUI
	PhaseShortcut
	PhaseUnhandled
)

type Dispatch struct {
	Event   Event
	Phase   Phase
	Handled bool
}

func (d *Dispatch) Consume() {
	if d != nil {
		d.Handled = true
	}
}

// Event is the normalized, ordered input stream produced by a window.
//
// The platform boundary uses a tagged value so keyboard, pointer, and text
// events retain their relative order. Gameplay and UI events should continue
// to use typed ECS messages.
type Event struct {
	Seq  uint64
	Kind EventKind

	Key    KeyCode
	Button MouseButton
	X      float64
	Y      float64
	DeltaX float64
	DeltaY float64
	WheelX float64
	WheelY float64
	// File is set for EventFilesDropped. One event is emitted per dropped file.
	File string

	Text        string
	Composition TextComposition
	Command     TextCommand
	Selection   TextSelection
}
