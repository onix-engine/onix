package input

import (
	"testing"

	"github.com/onix-engine/onix/internal/reuse"
)

func TestInputQueuesReleaseReferencesAfterBorrow(t *testing.T) {
	s := NewState()
	s.Enqueue(Event{Kind: EventTextInput, Text: "consumed"})
	s.Enqueue(Event{Kind: EventFilesDropped, File: "old-file"})
	s.Enqueue(Event{Kind: EventTextComposition, Composition: TextComposition{Text: "old-composition"}})
	borrowed := s.DrainEvents()
	if borrowed[0].Text != "consumed" || borrowed[2].Composition.Text != "old-composition" {
		t.Fatal("drain cleared events before their consumer finished")
	}
	// Enqueue invalidates the borrowed slice, but release must keep new input.
	s.Enqueue(Event{Kind: EventTextInput, Text: "pending"})
	s.ReleaseEvents()
	if len(s.events) != 1 || s.events[0].Text != "pending" {
		t.Fatal("releasing consumed input erased pending input")
	}
	for _, event := range borrowed[1:] {
		if event != (Event{}) {
			t.Fatal("consumed input retained references")
		}
	}
	s.DrainEvents()
	s.EndFrame()
	if borrowed[0] != (Event{}) {
		t.Fatal("end of frame retained a drained event")
	}
	for _, blur := range []bool{false, true} {
		s.CommitText("old-edit")
		edits := s.textEdits
		if blur {
			s.BlurTextInput()
		} else {
			s.EndFrame()
		}
		if edits[0].Text != "" || len(s.textEdits) != 0 {
			t.Fatal("text input reset retained an edit")
		}
	}
}

func TestInputQueuesReclaimPeaksAndStayWarm(t *testing.T) {
	s := NewState()
	for range 4096 {
		s.Enqueue(Event{Kind: EventTextInput, Text: "peak"})
		s.ApplyTextSelection(TextSelection{})
	}
	s.DrainEvents()
	s.ReleaseEvents()
	s.EndFrame()
	frame := func() {
		s.Enqueue(Event{Kind: EventTextInput, Text: "live"})
		events := s.DrainEvents()
		if len(events) != 1 || events[0].Text != "live" {
			t.Fatal("queue maintenance changed live input")
		}
		s.ReleaseEvents()
		s.ApplyTextSelection(TextSelection{})
		s.EndFrame()
	}
	for range reuse.RetentionFrames + 1 {
		frame()
	}
	if cap(s.events) > 32 || cap(s.textEdits) > 16 {
		t.Fatalf("input retained peak capacities: %d/%d", cap(s.events), cap(s.textEdits))
	}
	if allocations := testing.AllocsPerRun(100, frame); allocations != 0 {
		t.Fatalf("warm input maintenance allocated %g times", allocations)
	}
}
