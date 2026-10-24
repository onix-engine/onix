package ui

import (
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
)

func TestTextareaRestoreScroll(t *testing.T) {
	world := newUIWorld(t)
	control := SpawnTextarea(world, TextOptions{Width: 100, Height: 80, Value: strings.Repeat(strings.Repeat("x", 80)+"\n", 30)})
	field := ecs.GetMut[TextField](world, control)
	field.Focused, field.Caret, field.Anchor = true, 0, 0
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	textarea := ecs.GetMut[Textarea](world, control)
	textarea.scrollDragging = true
	textarea.RestoreScroll(40, 120)
	for range 3 {
		if err := SyncSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
	}
	textarea = ecs.GetMut[Textarea](world, control)
	if textarea.ScrollX != 40 || textarea.ScrollY != 120 || textarea.scrollDragging {
		t.Fatalf("saved viewport was not restored: %+v", textarea)
	}
	// Ordinary caret movement must resume revealing the caret.
	ecs.GetMut[TextField](world, control).Caret = 1
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	textarea = ecs.GetMut[Textarea](world, control)
	if textarea.ScrollY != 0 || textarea.ScrollX >= 40 {
		t.Fatalf("caret movement did not reveal the caret: %+v", textarea)
	}
	// An inactive document may have shrunk since its viewport was saved.
	ecs.GetMut[TextField](world, control).Value = "short"
	textarea.RestoreScroll(1000, 1000)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	textarea = ecs.GetMut[Textarea](world, control)
	if textarea.ScrollX != 0 || textarea.ScrollY != 0 {
		t.Fatalf("restored viewport was not clamped to content: %+v", textarea)
	}
}
