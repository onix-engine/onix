package input

import (
	"math/rand/v2"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestStateCopyFromSharesClipboardAndIsolatesKeys(t *testing.T) {
	source, snapshot := NewState(), NewState()
	clipboard := ""
	source.SetClipboard(func(value string) bool { clipboard = value; return true }, func() (string, bool) { return clipboard, true })
	source.PressKey(KeyC)
	snapshot.CopyFrom(source)
	if !snapshot.SetClipboardText("选区🙂") {
		t.Fatal("input snapshot lost clipboard access")
	}
	if value, ok := source.ClipboardText(); !ok || value != "选区🙂" {
		t.Fatalf("clipboard write was not shared: %q, %v", value, ok)
	}
	snapshot.ReleaseKey(KeyC)
	if !source.KeyHeld(KeyC) {
		t.Fatal("snapshot changed the source keyboard state")
	}
	if count := testing.AllocsPerRun(100, func() { snapshot.CopyFrom(source) }); count != 0 {
		t.Fatalf("input snapshot allocated %g times", count)
	}
	source.SetClipboard(nil, nil)
	snapshot.CopyFrom(source)
	if snapshot.SetClipboardText("stale") {
		t.Fatal("input snapshot retained an unavailable clipboard")
	}
}

func TestEventQueuePreservesOrderAndDerivesState(t *testing.T) {
	state := NewState()
	state.Enqueue(Event{Kind: EventKeyDown, Key: KeyA})
	state.Enqueue(Event{Kind: EventMouseDown, Button: MouseLeft, X: 12, Y: 18})
	state.Enqueue(Event{Kind: EventMouseMove, X: 20, Y: 24})
	state.Enqueue(Event{Kind: EventMouseUp, Button: MouseLeft, X: 20, Y: 24})

	events := state.DrainEvents()
	if len(events) != 4 {
		t.Fatalf("unexpected event count: %d", len(events))
	}
	for index, event := range events {
		if event.Seq != uint64(index+1) {
			t.Fatalf("event %d sequence = %d, want %d", index, event.Seq, index+1)
		}
	}
	for _, event := range events {
		state.ApplyEvent(event)
	}

	if !state.KeyHeld(KeyA) || !state.KeyPressed(KeyA) {
		t.Fatal("key event did not derive key state")
	}
	pointer := state.Pointer()
	if pointer.Down || !pointer.Pressed || !pointer.Released || pointer.X != 20 || pointer.Y != 24 {
		t.Fatalf("unexpected derived input state: %+v", pointer)
	}
	if len(state.DrainEvents()) != 0 {
		t.Fatal("drained events were not removed from queue")
	}
}

func TestStatePreservesFileDropEvents(t *testing.T) {
	state := NewState()
	state.Enqueue(Event{Kind: EventFilesDropped, X: 12, Y: 18, File: "/tmp/example.txt"})
	events := state.DrainEvents()
	if len(events) != 1 || events[0].Kind != EventFilesDropped || events[0].File != "/tmp/example.txt" {
		t.Fatalf("unexpected file drop event: %#v", events)
	}
}

func TestEventConsumptionIsScopedToFrame(t *testing.T) {
	state := NewState()
	state.Enqueue(Event{Kind: EventKeyDown, Key: KeyA})
	event := state.DrainEvents()[0]
	state.ConsumeEvent(event.Seq)
	if !state.EventConsumed(event.Seq) {
		t.Fatal("event was not marked consumed")
	}
	state.EndFrame()
	if state.EventConsumed(event.Seq) {
		t.Fatal("event consumption leaked into the next frame")
	}
}

func TestKeyStateTracksPressedDownReleased(t *testing.T) {
	state := NewState()

	state.PressKey(KeySpace)
	if !state.KeyPressed(KeySpace) || !state.KeyHeld(KeySpace) {
		t.Fatal("expected key to be pressed and down on first frame")
	}

	state.EndFrame()
	if state.KeyPressed(KeySpace) || !state.KeyHeld(KeySpace) {
		t.Fatal("expected pressed to clear while key remains down")
	}

	state.ReleaseKey(KeySpace)
	if !state.KeyReleased(KeySpace) || state.KeyHeld(KeySpace) {
		t.Fatal("expected released to be set and key down to clear")
	}
}

func TestInputStateTracksPrimaryInputAcrossFrame(t *testing.T) {
	state := NewState()

	state.PressMouse(MouseLeft, 10, 20)
	pointer := state.Pointer()
	if !pointer.Pressed || !pointer.Down || pointer.X != 10 || pointer.Y != 20 {
		t.Fatalf("unexpected pointer after press: %+v", pointer)
	}

	state.MoveMouse(14, 28)
	pointer = state.Pointer()
	if pointer.DeltaX != 4 || pointer.DeltaY != 8 {
		t.Fatalf("unexpected pointer delta after move: %+v", pointer)
	}

	state.ScrollMouse(0, -2)
	pointer = state.Pointer()
	if pointer.WheelY != -2 {
		t.Fatalf("unexpected wheel delta: %+v", pointer)
	}

	state.EndFrame()
	pointer = state.Pointer()
	if pointer.Pressed || pointer.DeltaX != 0 || pointer.WheelY != 0 {
		t.Fatalf("expected frame-local input state to clear: %+v", pointer)
	}

	state.ReleaseMouse(MouseLeft, 14, 28)
	pointer = state.Pointer()
	if !pointer.Released || pointer.Down {
		t.Fatalf("unexpected pointer after release: %+v", pointer)
	}
}

func TestInputReleasePreservesPendingMovement(t *testing.T) {
	state := NewState()

	state.PressMouse(MouseLeft, 10, 20)
	state.MoveMouse(14, 28)
	state.ReleaseMouse(MouseLeft, 18, 30)

	pointer := state.Pointer()
	if !pointer.Released || pointer.Down || pointer.PressX != 10 || pointer.PressY != 20 || pointer.DeltaX != 8 || pointer.DeltaY != 10 {
		t.Fatalf("unexpected pointer after same-frame drag: %+v", pointer)
	}
}

func TestTextInputClearsPerFrameAndCompositionPersists(t *testing.T) {
	state := NewState()

	state.ApplyTextComposition(TextComposition{Text: "ni", Active: true})
	state.CommitText("你")
	state.CommitText("好")

	if state.Composing() || state.CompositionText() != "" {
		t.Fatalf("expected commit to end composition, got active=%t text=%q", state.Composing(), state.CompositionText())
	}
	if got, _, _ := ApplyTextEditsInRange("", 0, 0, 0, false, state.TextEdits()); got != "你好" {
		t.Fatalf("unexpected text input: %q", got)
	}

	state.ApplyTextComposition(TextComposition{Text: "ma", Active: true})
	state.EndFrame()

	if len(state.TextEdits()) != 0 {
		t.Fatalf("expected text edits to clear at frame end: %#v", state.TextEdits())
	}
	if state.CompositionText() != "ma" {
		t.Fatalf("expected composition to persist, got %q", state.CompositionText())
	}
	if !state.Composing() {
		t.Fatal("expected composition activity to persist")
	}
}

func TestTextInputSanitizesCommittedCompositionAndInitialValues(t *testing.T) {
	state := NewState()
	state.FocusTextInput(TextInputText, false, "a\r\nb\t\x00")
	if got := state.TextInputConfig().InitialValue; got != "ab" {
		t.Fatalf("unexpected single-line initial value: %q", got)
	}

	state.CommitText("c\r\nd\t\x08\x7f")
	state.ApplyTextComposition(TextComposition{Text: "e\nf\t\u0085", Active: true})
	if got, _, _ := ApplyTextEditsInRange("", 0, 0, 0, false, state.TextEdits()); got != "cd" {
		t.Fatalf("unexpected single-line commit: %q", got)
	}
	if got := state.CompositionText(); got != "ef" {
		t.Fatalf("unexpected single-line composition: %q", got)
	}

	state.FocusTextInput(TextInputText, true, "a\r\nb\t\x00")
	state.CommitText("c\r\nd\t\x08")
	if got := state.TextInputConfig().InitialValue; got != "a\nb\t" {
		t.Fatalf("unexpected multiline initial value: %q", got)
	}
	if got, _, _ := ApplyTextEditsInRange("", 0, 0, 0, true, state.TextEdits()); got != "c\nd\t" {
		t.Fatalf("unexpected accumulated multiline commit: %q", got)
	}
}

func TestTextInputFocusChangeClearsSessionEvents(t *testing.T) {
	state := NewState()
	state.FocusTextInput(TextInputText, false, "first")
	state.CommitText("x")
	state.PushTextCommand(TextCommandBackspace)

	state.FocusTextInput(TextInputText, false, "second")
	if len(state.TextEdits()) != 0 {
		t.Fatal("new text input session inherited pending events")
	}

	state.CommitText("y")
	state.PushTextCommand(TextCommandEnter)
	state.BlurTextInput()
	if len(state.TextEdits()) != 0 {
		t.Fatal("blurred text input session retained pending events")
	}
}

func TestTextInputSessionAndCommandsPersistAcrossFrame(t *testing.T) {
	state := NewState()
	state.FocusTextInput(TextInputDecimal, false, "12.5")
	first := state.TextInputConfig()
	if !first.Active || first.Kind != TextInputDecimal || first.InitialValue != "12.5" || first.Revision == 0 {
		t.Fatalf("unexpected focused config: %#v", first)
	}

	state.PushTextCommand(TextCommandBackspace)
	state.PushTextCommand(TextCommandBackspace)
	if got := len(state.TextEdits()); got != 2 {
		t.Fatalf("unexpected edit count: %d", got)
	}
	state.EndFrame()
	if len(state.TextEdits()) != 0 {
		t.Fatal("expected commands to clear at frame end")
	}
	if state.TextInputConfig() != first {
		t.Fatalf("expected text input config to persist: %#v", state.TextInputConfig())
	}

	state.ApplyTextComposition(TextComposition{Active: true})
	state.FocusTextInput(TextInputInteger, false, "7")
	second := state.TextInputConfig()
	if second.Revision <= first.Revision || state.Composing() {
		t.Fatalf("expected a new clean session: %#v", second)
	}
	state.BlurTextInput()
	if state.TextInputConfig().Active || state.TextInputConfig().Revision <= second.Revision {
		t.Fatalf("unexpected blurred config: %#v", state.TextInputConfig())
	}
}

func TestTextInputCaretUpdatesWithoutRestartingSession(t *testing.T) {
	state := NewState()
	state.FocusTextInput(TextInputText, false, "text")
	before := state.TextInputConfig()
	caret := TextInputRect{X: 12, Y: 34, Width: 1, Height: 20}
	state.SetTextInputCaret(caret)
	after := state.TextInputConfig()
	if after.Caret != caret || after.Revision != before.Revision {
		t.Fatalf("unexpected caret update: before=%#v after=%#v", before, after)
	}
}

func TestTextEditsPreserveEventOrderAndCompositionState(t *testing.T) {
	state := NewState()
	state.FocusTextInput(TextInputText, false, "a")
	state.CommitText("b")
	state.PushTextCommand(TextCommandBackspace)

	value, _, submits := ApplyTextEditsInRange("a", 1, 1, 0, false, state.TextEdits())
	if value != "a" || submits != 0 {
		t.Fatalf("text edit order changed: value=%q submits=%d", value, submits)
	}

	state.EndFrame()
	state.ApplyTextComposition(TextComposition{Text: "ni", Active: true})
	state.PressKey(KeyEnter)
	state.PushTextCommand(TextCommandBackspace)
	state.CommitText("你")
	value, _, submits = ApplyTextEditsInRange(value, len([]rune(value)), len([]rune(value)), 0, false, state.TextEdits())
	if value != "a你" || submits != 0 {
		t.Fatalf("composition command leaked into committed edits: value=%q submits=%d", value, submits)
	}
}

func TestTextInputConfigValueEncodesPlatformFlags(t *testing.T) {
	config := TextInputConfig{Revision: 7, Kind: TextInputInteger, Active: true, Multiline: true}
	value := config.Value()
	if value>>8 != 7 || value&1 == 0 || value&(1<<1) == 0 || value>>2&0x3 != uint64(TextInputInteger) {
		t.Fatalf("unexpected encoded config: %#x", value)
	}
}

func TestNativeSelectionEditReplacesRangeBeforeCollapsing(t *testing.T) {
	state := NewState()
	state.FocusTextInput(TextInputText, false, "hello")
	state.ApplyTextSelection(TextSelection{Start: 1, End: 4})
	state.CommitText("X")
	state.ApplyTextSelection(TextSelection{Start: 2, End: 2})
	value, caret, _ := ApplyTextEditsInRange("hello", 2, 2, 0, false, state.TextEdits())
	if value != "hXo" || caret != 2 {
		t.Fatalf("unexpected native replacement: value=%q caret=%d", value, caret)
	}
}

func TestApplyTextEditsInRangeEditsUnicodeAtCaret(t *testing.T) {
	value, caret, submits := ApplyTextEditsInRange("a界c", 2, 2, 0, false, []TextEdit{
		{Command: TextCommandBackspace},
		{Text: "你好"},
	})
	if value != "a你好c" || caret != 3 || submits != 0 {
		t.Fatalf("unexpected caret edit: value=%q caret=%d submits=%d", value, caret, submits)
	}
}

func TestApplyTextEditsInRangeInsertsMultilineWithinLimit(t *testing.T) {
	value, caret, _ := ApplyTextEditsInRange("ab", 1, 1, 4, true, []TextEdit{
		{Command: TextCommandEnter},
		{Text: "xy"},
	})
	if value != "a\nxb" || caret != 3 {
		t.Fatalf("unexpected limited caret edit: value=%q caret=%d", value, caret)
	}
}

func TestApplyTextEditsInRangeDeletesAndReplacesSelection(t *testing.T) {
	value, caret, _ := ApplyTextEditsInRange("a界bc", 1, 3, 0, false, []TextEdit{{Command: TextCommandBackspace}})
	if value != "ac" || caret != 1 {
		t.Fatalf("unexpected selection deletion: value=%q caret=%d", value, caret)
	}

	value, caret, _ = ApplyTextEditsInRange("a界bc", 3, 1, 0, false, []TextEdit{{Text: "你"}})
	if value != "a你c" || caret != 2 {
		t.Fatalf("unexpected selection replacement: value=%q caret=%d", value, caret)
	}
}

func TestApplyTextEditsInRangeDoesNotAllocateWithoutEdits(t *testing.T) {
	const value = "a界c"
	if got, caret, submits := ApplyTextEditsInRange(value, 3, 1, 0, false, nil); got != value || caret != 3 || submits != 0 {
		t.Fatalf("empty edits changed state: value=%q caret=%d submits=%d", got, caret, submits)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		ApplyTextEditsInRange(value, 3, 1, 0, false, nil)
	}); allocations != 0 {
		t.Fatalf("empty text edits allocated %.0f objects, want 0", allocations)
	}
}

func TestAppendTextEditsDoesNotAllocateAfterWarmup(t *testing.T) {
	state := NewState()
	state.CommitText("text")
	edits := state.AppendTextEdits(nil)
	if len(edits) != 1 || edits[0].Text != "text" {
		t.Fatalf("unexpected appended edits: %#v", edits)
	}

	if allocations := testing.AllocsPerRun(100, func() {
		edits = state.AppendTextEdits(edits[:0])
	}); allocations != 0 {
		t.Fatalf("appending text edits allocated %.0f objects, want 0", allocations)
	}
}

func TestStateTextEditsReuseTreeAcrossUnicodeAndExternalValues(t *testing.T) {
	state := NewState()
	value, caret, submits := state.ApplyTextEditsInRange("abcd", 2, 2, 0, true, []TextEdit{{Text: "界"}})
	if value != "ab界cd" || caret != 3 || submits != 0 {
		t.Fatalf("initial tree edit = %q, caret %d, submits %d", value, caret, submits)
	}
	value, caret, submits = state.ApplyTextEditsInRange(value, 1, 4, 5, true, []TextEdit{{Text: "X\r\nY\x00"}})
	if value != "aX\nYd" || caret != 4 || submits != 0 {
		t.Fatalf("distant tree edit = %q, caret %d, submits %d", value, caret, submits)
	}
	value, caret, submits = state.ApplyTextEditsInRange(value, caret, caret, 5, true, []TextEdit{{Command: TextCommandBackspace}})
	if value != "aX\nd" || caret != 3 || submits != 0 {
		t.Fatalf("tree backspace = %q, caret %d, submits %d", value, caret, submits)
	}
	value, caret, submits = state.ApplyTextEditsInRange("外部", 1, 1, 0, false, []TextEdit{{Text: "部"}})
	if value != "外部部" || caret != 2 || submits != 0 {
		t.Fatalf("external value reset = %q, caret %d, submits %d", value, caret, submits)
	}
	inserted := strings.Repeat("界", 200)
	value, caret, submits = state.ApplyTextEditsInRange("left-right", 4, 5, 0, false, []TextEdit{{Text: inserted}})
	if value != "left"+inserted+"right" || caret != 204 || submits != 0 {
		t.Fatalf("middle tree insertion length = %d, caret %d, submits %d", len([]rune(value)), caret, submits)
	}
}

func TestStateContinuousTextEditAllocatesOnlyResultString(t *testing.T) {
	state := NewState()
	value := strings.Repeat("x", 256)
	edit := []TextEdit{{Text: "a"}}
	value, _, _ = state.ApplyTextEditsInRange(value, 128, 129, 0, false, edit)

	iteration := 0
	allocations := testing.AllocsPerRun(100, func() {
		if iteration&1 == 0 {
			edit[0].Text = "b"
		} else {
			edit[0].Text = "a"
		}
		value, _, _ = state.ApplyTextEditsInRange(value, 128, 129, 0, false, edit)
		iteration++
	})
	if allocations > 1 {
		t.Fatalf("stable continuous text edit allocated %.0f objects, want at most the result string", allocations)
	}
}

var continuousTextEditResult string

// TestStateTextBufferMatchesStateless replays edits through the cached UTF-8 piece tree
// buffer and the stateless path, requiring identical values and carets.
func TestStateTextBufferMatchesStateless(t *testing.T) {
	steps := []struct {
		edits     []TextEdit
		multiline bool
		maxRunes  int
	}{
		{edits: []TextEdit{{Text: "你好"}}},
		{edits: []TextEdit{{Command: TextCommandBackspace}}},
		{edits: []TextEdit{{HasSelection: true, SelectionStart: 1, SelectionEnd: 4}}},
		{edits: []TextEdit{{Text: "界x"}}},
		{edits: []TextEdit{{Command: TextCommandBackspace}, {Command: TextCommandBackspace}}},
		{edits: []TextEdit{{HasSelection: true, SelectionStart: 6, SelectionEnd: 0}}},
		{edits: []TextEdit{{Text: "\xff界"}}},
		{edits: []TextEdit{{Command: TextCommandEnter}}, multiline: true},
		{edits: []TextEdit{{Text: "界界界界界界界界"}}, maxRunes: 8},
		{edits: []TextEdit{{HasSelection: true, SelectionStart: 3, SelectionEnd: 3}, {Command: TextCommandBackspace}}},
		{edits: []TextEdit{{Text: "a界"}, {Command: TextCommandEnter}, {Text: "b"}}},
	}
	state := NewState()
	cached, cachedCaret := "a界b界c界", 0
	plain, plainCaret := cached, cachedCaret
	for index, step := range steps {
		cached, cachedCaret, _ = state.ApplyTextEditsInRange(cached, cachedCaret, cachedCaret, step.maxRunes, step.multiline, step.edits)
		plain, plainCaret, _ = ApplyTextEditsInRange(plain, plainCaret, plainCaret, step.maxRunes, step.multiline, step.edits)
		if cached != plain || cachedCaret != plainCaret {
			t.Fatalf("step %d diverged: cached=%q caret=%d, stateless=%q caret=%d", index, cached, cachedCaret, plain, plainCaret)
		}
	}
	cached, cachedCaret = "a界b界c界", 0
	plain, plainCaret = cached, cachedCaret
	random := rand.New(rand.NewPCG(1, 2))
	for step := 0; step < 500; step++ {
		length := utf8.RuneCountInString(cached)
		edits := make([]TextEdit, 0, 2)
		switch random.IntN(4) {
		case 0:
			edits = append(edits, TextEdit{Command: TextCommandBackspace})
		case 1:
			edits = append(edits, TextEdit{Text: [...]string{"x", "界", "\U0001F600", "\r\n", "\x01"}[random.IntN(5)]})
		case 2:
			edits = append(edits, TextEdit{Command: TextCommandEnter})
		default:
			first, second := random.IntN(length+1), random.IntN(length+1)
			edits = append(edits, TextEdit{HasSelection: true, SelectionStart: first, SelectionEnd: second})
			if random.IntN(2) == 0 {
				edits = append(edits, TextEdit{Command: TextCommandBackspace})
			} else {
				edits = append(edits, TextEdit{Text: "界a"})
			}
		}
		cached, cachedCaret, _ = state.ApplyTextEditsInRange(cached, cachedCaret, cachedCaret, 0, true, edits)
		plain, plainCaret, _ = ApplyTextEditsInRange(plain, plainCaret, plainCaret, 0, true, edits)
		if cached != plain || cachedCaret != plainCaret {
			t.Fatalf("random step %d diverged: cached=%q caret=%d, stateless=%q caret=%d", step, cached, cachedCaret, plain, plainCaret)
		}
	}
}

func BenchmarkContinuousTextEdit(b *testing.B) {
	const caret = 2048
	initial := strings.Repeat("x", caret*2)
	edits := []TextEdit{{Text: "a"}}
	b.Run("stateless", func(b *testing.B) {
		value := initial
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			edits[0].Text = [...]string{"a", "b"}[index&1]
			value, _, _ = ApplyTextEditsInRange(value, caret, caret+1, 0, false, edits)
		}
		continuousTextEditResult = value
	})
	b.Run("piece_tree", func(b *testing.B) {
		state := NewState()
		value, _, _ := state.ApplyTextEditsInRange(initial, caret, caret+1, 0, false, edits)
		b.ResetTimer()
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			edits[0].Text = [...]string{"a", "b"}[index&1]
			value, _, _ = state.ApplyTextEditsInRange(value, caret, caret+1, 0, false, edits)
		}
		continuousTextEditResult = value
	})
}
