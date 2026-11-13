package ecs

import (
	"testing"

	"github.com/onix-engine/onix/internal/reuse"
)

func TestFrameScratchReclaimsPeakAndIdleReferences(t *testing.T) {
	w := NewWorld()
	scratch := w.FrameScratch[scratchSliceTestTag, *int]()
	value := 42
	scratch.Values = make([]*int, 4096)
	for i := range scratch.Values {
		scratch.Values[i] = &value
	}
	peak := scratch.Values
	w.Maintain()
	for range reuse.RetentionFrames + 1 {
		scratch = w.FrameScratch[scratchSliceTestTag, *int]()
		scratch.Values = append(scratch.Values, &value)
		// Maintenance also handles a consumer that skipped ReleaseUnused.
		w.Maintain()
	}
	if len(scratch.Values) != 1 || cap(scratch.Values) > 64 || *scratch.Values[0] != value || peak[1] != nil {
		t.Fatal("frame maintenance lost live values or retained its peak")
	}
	scratch.Values = make([]*int, 4096)
	scratch.Values[0] = &value
	peak = scratch.Values
	w.Maintain()
	if peak[0] != nil {
		t.Fatal("an unused workspace retained its last values")
	}
	for range reuse.RetentionFrames {
		w.Maintain()
	}
	if scratch.Values != nil {
		t.Fatal("idle workspace retained its large backing array")
	}
}

func TestFrameScratchRetainsLargestBatchInFrame(t *testing.T) {
	w := NewWorld()
	frame := func() {
		scratch := w.FrameScratch[scratchSliceTestTag, int]()
		scratch.Values = scratch.Reserve(512)[:512]
		scratch = w.FrameScratch[scratchSliceTestTag, int]()
		scratch.Values = append(scratch.Values, 1)
		w.Maintain()
	}
	frame()
	if allocations := testing.AllocsPerRun(100, frame); allocations != 0 {
		t.Fatalf("a small final batch evicted the larger batch: %g allocations", allocations)
	}
}

func TestMessageBuffersReclaimOnlyExpiredStorage(t *testing.T) {
	w := NewWorld()
	value := 42
	var expired []*int
	for range 2 {
		for range 4096 {
			w.WriteMessage(&value)
		}
		messages := w.messageBufferFor[*int]()
		expired = messages.newer
		w.AdvanceMessages()
		if expired[0] != &value {
			t.Fatal("message rotation released a still-live message")
		}
	}
	frame := func() {
		w.WriteMessage(&value)
		w.AdvanceMessages()
	}
	for range reuse.RetentionFrames*2 + 4 {
		frame()
	}
	messages := w.messageBufferFor[*int]()
	if cap(messages.older) > 64 || cap(messages.newer) > 64 || len(messages.older) != 1 || messages.older[0] != &value || expired[1] != nil {
		t.Fatal("message storage retained its peak or lost live messages")
	}
	if allocations := testing.AllocsPerRun(100, frame); allocations != 0 {
		t.Fatalf("warm message rotation allocated %g times", allocations)
	}
}

func TestMessageScratchReclaimsExpiredArenas(t *testing.T) {
	w := NewWorld()
	value := 42
	for range 2 {
		scratch := w.MessageScratch[scratchSliceTestTag, *int]()
		scratch.Values = make([]*int, 4096)
		scratch.Values[0] = &value
		w.AdvanceMessages()
	}
	for range reuse.RetentionFrames*2 + 4 {
		w.AdvanceMessages()
	}
	for _, store := range w.messages {
		arena := store.(*messageScratch[*int])
		if arena.older != nil || arena.scratch.Values != nil {
			t.Fatal("idle message arena retained peak storage")
		}
	}
}
