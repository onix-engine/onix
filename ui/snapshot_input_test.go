package ui

import (
	"strings"
	"testing"

	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/text/piecetree"
)

func TestSnapshotFieldIMESelectionAndGeometry(t *testing.T) {
	world := newUIWorld(t)
	initial := strings.Repeat("abc中文🙂\r\n", 300)
	owner := SpawnTextarea(world, TextOptions{Width: 300, Height: 120, Value: initial})
	if !FocusControl(world, owner) {
		t.Fatal("focus failed")
	}
	var buffer input.TextBuffer
	base := piecetree.FromString(initial)
	middle := base.Runes() / 2
	before, _, _ := buffer.ApplyEdits(base, middle, middle+1, 0, true, []input.TextEdit{{Text: "x"}})
	field := world.GetMut[TextField](owner)
	field.Value, field.Source, field.Buffer = "", before, &buffer
	field.Anchor, field.Caret = middle, middle+1
	state, _ := world.GetResource[*input.State]()
	state.EndFrame()
	state.ApplyTextComposition(input.TextComposition{Text: "拼音", Active: true})
	if err := runUIUpdate(world); err != nil {
		t.Fatal(err)
	}
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	selected := world.GetRef[TextField](owner)
	if selected.Source != before || selected.Anchor != middle || selected.Caret != middle+1 {
		t.Fatal("composition changed the committed version or collapsed selection")
	}
	entity, ok := FindPart[ValuePart, scene.Text2D](world, owner)
	if !ok {
		t.Fatal("missing text visual")
	}
	text := world.GetRef[scene.Text2D](entity)
	visual := text.Text()
	start, end := before.ByteOffset(middle), before.ByteOffset(middle+1)
	if visual.Len() != before.Len()-(end-start)+len("拼音") || visual.Slice(start, start+len("拼音")) != "拼音" {
		t.Fatal("composition did not replace the selected range visually")
	}
	if _, _, _, _, ok := scene.TextCaretRect2D(text, visual, middle+2); !ok {
		t.Fatal("segmented composition has no caret geometry")
	}
	if visual.Materialized() || before.Materialized() {
		t.Fatal("composition or caret materialized a document")
	}
	state.EndFrame()
	state.CommitText("界")
	if err := runUIUpdate(world); err != nil {
		t.Fatal(err)
	}
	after := world.GetRef[TextField](owner)
	if after.Source.Slice(start, start+len("界")) != "界" || after.Anchor != middle+1 || after.Caret != middle+1 || after.Composition != "" {
		t.Fatal("IME commit did not replace the selected range")
	}
	if after.Source.Materialized() || before.Materialized() || visual.Materialized() || before.Slice(start, end) != "x" {
		t.Fatal("IME commit materialized or mutated a published version")
	}
}
