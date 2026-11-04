package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/text/piecetree"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestTextEditingNavigation(t *testing.T) {
	const value = "甲乙🙂\nhello world\n尾"
	for _, test := range []struct {
		name                  string
		keys                  []input.KeyCode
		caret, anchor         int
		wantCaret, wantAnchor int
	}{
		{name: "home", caret: 9, anchor: 9, keys: []input.KeyCode{input.KeyHome}, wantCaret: 4, wantAnchor: 4},
		{name: "end", caret: 9, anchor: 9, keys: []input.KeyCode{input.KeyEnd}, wantCaret: 15, wantAnchor: 15},
		{name: "document start", caret: 9, anchor: 9, keys: []input.KeyCode{input.KeyLeftControl, input.KeyHome}, wantCaret: 0, wantAnchor: 0},
		{name: "document end", caret: 9, anchor: 9, keys: []input.KeyCode{input.KeyLeftControl, input.KeyEnd}, wantCaret: 17, wantAnchor: 17},
		{name: "select to start", caret: 9, anchor: 9, keys: []input.KeyCode{input.KeyLeftShift, input.KeyHome}, wantCaret: 4, wantAnchor: 9},
		{name: "select to end", caret: 9, anchor: 9, keys: []input.KeyCode{input.KeyLeftShift, input.KeyLeftControl, input.KeyEnd}, wantCaret: 17, wantAnchor: 9},
		{name: "up unicode", caret: 6, anchor: 6, keys: []input.KeyCode{input.KeyUp}, wantCaret: 2, wantAnchor: 2},
		{name: "down short line", caret: 9, anchor: 9, keys: []input.KeyCode{input.KeyDown}, wantCaret: 17, wantAnchor: 17},
		{name: "collapse left", caret: 8, anchor: 5, keys: []input.KeyCode{input.KeyLeft}, wantCaret: 5, wantAnchor: 5},
		{name: "collapse right", caret: 5, anchor: 8, keys: []input.KeyCode{input.KeyRight}, wantCaret: 8, wantAnchor: 8},
		{name: "extend left", caret: 3, anchor: 3, keys: []input.KeyCode{input.KeyLeftShift, input.KeyLeft}, wantCaret: 2, wantAnchor: 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := input.NewState()
			for _, key := range test.keys {
				state.PressKey(key)
			}
			check := func() {
				t.Helper()
				caret, anchor := moveTextCaret(piecetree.FromString(value), test.caret, test.anchor, state)
				if caret != test.wantCaret || anchor != test.wantAnchor {
					t.Fatalf("selection = %d:%d, want %d:%d", caret, anchor, test.wantCaret, test.wantAnchor)
				}
			}
			check()
			if allocs := testing.AllocsPerRun(100, check); allocs != 0 {
				t.Fatalf("navigation allocated %g", allocs)
			}
		})
	}
	text := piecetree.FromString("甲乙 hello, world 🙂")
	for _, test := range []struct {
		caret int
		right bool
		want  int
	}{{0, true, 3}, {3, true, 8}, {8, true, 10}, {10, false, 8}, {8, false, 3}, {3, false, 0}} {
		if got := textWordBoundary(text, test.caret, test.right); got != test.want {
			t.Fatalf("word boundary (%d, %v) = %d, want %d", test.caret, test.right, got, test.want)
		}
	}
}

func TestTextEditingHistoryAndDelete(t *testing.T) {
	world := newUIWorld(t)
	entity := SpawnTextarea(world, TextOptions{Width: 200, Height: 80, Value: "甲乙🙂尾"})
	_ = world.Insert(entity, htmlui.Control{Tag: "textarea"})
	field := world.GetMut[TextField](entity)
	field.Caret, field.Anchor = 3, 1
	edit := func(text string, keys ...input.KeyCode) {
		t.Helper()
		state := input.NewState()
		state.FocusTextInput(input.TextInputText, true, "")
		if text != "" {
			state.CommitText(text)
		}
		for _, key := range keys {
			state.PressKey(key)
		}
		editFocused(world, entity, state)
	}
	check := func(value string, caret, anchor int) {
		t.Helper()
		field = world.GetMut[TextField](entity)
		if field.Value != value || field.Caret != caret || field.Anchor != anchor {
			t.Fatalf("got %q %d:%d, want %q %d:%d", field.Value, field.Caret, field.Anchor, value, caret, anchor)
		}
	}
	edit("中\n文")
	check("甲中\n文尾", 4, 4)
	edit("", input.KeyLeftControl, input.KeyZ)
	check("甲乙🙂尾", 3, 1)
	edit("", input.KeyLeftControl, input.KeyLeftShift, input.KeyZ)
	check("甲中\n文尾", 4, 4)
	edit("", input.KeyDelete)
	check("甲中\n文", 4, 4)
	edit("", input.KeyLeftSuper, input.KeyZ)
	check("甲中\n文尾", 4, 4)
	edit("", input.KeyLeftControl, input.KeyY)
	check("甲中\n文", 4, 4)
	edit("", input.KeyLeftControl, input.KeyZ)
	edit("新")
	edit("", input.KeyLeftControl, input.KeyY)
	check("甲中\n文新尾", 5, 5)
	field.Value, field.Caret, field.Anchor = "external", 0, 0
	edit("", input.KeyLeftControl, input.KeyZ)
	check("external", 0, 0)
	edit("x")
	_ = world.Insert(entity, htmlui.Control{ReadOnly: true})
	edit("", input.KeyLeftControl, input.KeyZ)
	check("xexternal", 1, 1)
}

func TestTextEditingHistoryCancellationAndComposition(t *testing.T) {
	world := newUIWorld(t)
	html := htmlui.New(`<div id="editor" contenteditable style="width:160px;height:48px">ab</div>`)
	html.Width, html.Height = 220, 80
	bundle := ecs.NewBundle(1)
	bundle.Put(html)
	owner := world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	entity := htmlFocusableByID(t, world, owner, "editor")
	field := world.GetMut[TextField](entity)
	field.Caret, field.Anchor = 2, 2
	state := input.NewState()
	state.CommitText("X")
	editFocused(world, entity, state)
	cancel := true
	_ = world.Insert(owner, htmlui.ActionHandler{Handle: func(c *htmlui.ActionContext) {
		if cancel && c.Event.InputType == "historyUndo" {
			c.PreventDefault()
		}
	}})
	state = input.NewState()
	state.PressKey(input.KeyLeftControl)
	state.PressKey(input.KeyZ)
	editFocused(world, entity, state)
	field = world.GetMut[TextField](entity)
	if field.Value != "abX" {
		t.Fatal("cancelled undo changed the value", field.Value)
	}
	cancel = false
	editFocused(world, entity, state)
	if field.Value != "ab" {
		t.Fatal("cancelled undo consumed the history entry", field.Value)
	}
	state = input.NewState()
	state.CommitText("文")
	editFocused(world, entity, state)
	state = input.NewState()
	state.ApplyTextComposition(input.TextComposition{Text: "zhong", Active: true})
	state.PressKey(input.KeyLeftControl)
	state.PressKey(input.KeyZ)
	editFocused(world, entity, state)
	if field.Value != "ab文" {
		t.Fatal("composition shortcut changed history", field.Value)
	}
	state.CommitText("中")
	editFocused(world, entity, state)
	if field.Value != "ab文中" {
		t.Fatal("composition commit was replaced by undo", field.Value)
	}
}
