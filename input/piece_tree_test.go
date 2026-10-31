package input

import (
	"math/rand/v2"
	"strings"
	"testing"
	"unicode/utf8"
	"unsafe"
)

func TestTextBufferTailCompactionPublishesDetachedSnapshot(t *testing.T) {
	var buffer TextBuffer
	initial := strings.Repeat("a", 1<<20)
	value, _, _ := buffer.ApplyTextEditsInRange(initial, 1024, len(initial), 0, true, []TextEdit{{Command: TextCommandBackspace}})
	if value != initial[:1024] || unsafe.StringData(value) == unsafe.StringData(initial) || unsafe.StringData(buffer.value.String()) != unsafe.StringData(value) {
		t.Fatal("input published the oversized snapshot after compaction")
	}
	start, end, next, _, ok := buffer.ChangedRange(initial, value)
	if !ok || start != 1024 || end != len(initial) || next != 1024 {
		t.Fatal("compaction lost the edit delta")
	}
	previous := value
	value, _, _ = buffer.ApplyTextEditsInRange(value, len(value), len(value), 0, true, []TextEdit{{Text: "x"}})
	if value != previous+"x" || buffer.before.String() != previous || initial != strings.Repeat("a", 1<<20) {
		t.Fatal("subsequent edit lost or mutated a snapshot")
	}
}

func TestTextBufferLargePasteReleasesScratch(t *testing.T) {
	var buffer TextBuffer
	paste := strings.Repeat("界\r\n", 30000)
	value, _, _ := buffer.ApplyTextEditsInRange("", 0, 0, 0, true, []TextEdit{{Text: paste}})
	if value != strings.Repeat("界\n", 30000) || cap(buffer.scratch) > 64<<10 {
		t.Fatal("large paste lost sanitization or retained its scratch buffer")
	}
	value, _, _ = buffer.ApplyTextEditsInRange(value, 0, buffer.Runes(), 0, true, []TextEdit{{Text: "small"}})
	buffer.Reserve(4096, 128)
	// Establish snapshot capacity after the replacement before checking typing.
	value, _, _ = buffer.ApplyTextEditsInRange(value, buffer.Runes(), buffer.Runes(), 0, true, []TextEdit{{Text: "x"}})
	if n := testing.AllocsPerRun(100, func() {
		value, _, _ = buffer.ApplyTextEditsInRange(value, buffer.Runes(), buffer.Runes(), 0, true, []TextEdit{{Text: "x"}})
	}); n != 0 {
		t.Fatalf("warm typing after paste allocated %g times", n)
	}
}

func TestTextBufferBatchChangedRange(t *testing.T) {
	for _, fixture := range []struct {
		value string
		edits []TextEdit
	}{
		{"a界😀\r\nz", []TextEdit{{HasSelection: true, SelectionStart: 1, SelectionEnd: 3}, {Text: "好"}, {HasSelection: true, SelectionStart: 0, SelectionEnd: 0}, {Text: "head\n"}, {HasSelection: true, SelectionStart: 7, SelectionEnd: 9}, {Text: "tail"}}},
		{"abcdef", []TextEdit{{HasSelection: true, SelectionStart: 5, SelectionEnd: 6}, {Text: "end"}, {HasSelection: true, SelectionStart: 1, SelectionEnd: 2}, {Text: "start"}}},
		{"abc", []TextEdit{{Text: "界"}, {Command: TextCommandBackspace}, {Text: "😀"}}},
		{"\xff\xfe界", []TextEdit{{Text: "a"}}},
	} {
		var buffer TextBuffer
		got, _, _ := buffer.ApplyTextEditsInRange(fixture.value, 0, 0, 0, true, fixture.edits)
		start, oldEnd, newEnd, runes, ok := buffer.ChangedRange(fixture.value, got)
		if !ok || start < 0 || oldEnd > len(fixture.value) || newEnd > len(got) || fixture.value[:start]+got[start:newEnd]+fixture.value[oldEnd:] != got {
			t.Fatalf("invalid delta for %q -> %q: %d/%d/%d, ok=%t", fixture.value, got, start, oldEnd, newEnd, ok)
		}
		if runes != utf8.RuneCountInString(got[:start]) {
			t.Fatal("prefix rune count mismatch")
		}
		if _, _, _, _, ok := buffer.ChangedRange("stale", got); ok {
			t.Fatal("accepted stale base")
		}
	}
}

func TestTextBufferCompactionAndSnapshotOwnership(t *testing.T) {
	var buffer TextBuffer
	initial := strings.Repeat("大\n", 100000)
	buffer.Sync(initial)
	snapshot := buffer.String()
	buffer.ReplaceBytes(0, buffer.Len(), "small")
	if snapshot != initial || buffer.String() != "small" || buffer.RetainedBytes() > 64<<10 {
		t.Fatal("replaced document retained deleted original or mutated snapshot")
	}
	next := "s外部mall"
	buffer.ApplySnapshot(next, 1, 1, 7)
	if buffer.String() != next {
		t.Fatalf("external snapshot = %q", buffer.String())
	}
	if allocations := testing.AllocsPerRun(100, func() { buffer.Sync(next); buffer.String() }); allocations != 0 {
		t.Fatalf("snapshot synchronization allocated %g times", allocations)
	}
}

func TestTextBufferDocumentsRemainIndependent(t *testing.T) {
	var first, second TextBuffer
	a, _, _ := first.ApplyTextEditsInRange("abc", 1, 1, 0, true, []TextEdit{{Text: "界"}})
	b, _, _ := second.ApplyTextEditsInRange("other", 0, 5, 0, true, []TextEdit{{Text: "😀"}})
	if a != "a界bc" || b != "😀" || first.String() != a || second.String() != b {
		t.Fatal("document switching changed another buffer")
	}
}

func TestTextBufferRandomBatchDelta(t *testing.T) {
	random := rand.New(rand.NewPCG(15, 29))
	var buffer TextBuffer
	value := "a界😀\r\n" + strings.Repeat("line\n", 50)
	for step := range 2000 {
		before := value
		count := utf8.RuneCountInString(before)
		edits := make([]TextEdit, 0, 8)
		for range 4 {
			edits = append(edits, TextEdit{HasSelection: true, SelectionStart: random.IntN(count + 10), SelectionEnd: random.IntN(count + 10)})
			if random.IntN(3) == 0 {
				edits = append(edits, TextEdit{Command: TextCommandBackspace})
			} else {
				edits = append(edits, TextEdit{Text: [...]string{"界", "😀\n", "abc", "\r\n", "\x01"}[random.IntN(5)]})
			}
		}
		value, _, _ = buffer.ApplyTextEditsInRange(before, 0, 0, 0, true, edits)
		start, oldEnd, newEnd, _, ok := buffer.ChangedRange(before, value)
		if before != value && !ok {
			t.Fatalf("step %d lost changed range", step)
		}
		if ok && (start < 0 || oldEnd < start || newEnd < start || oldEnd > len(before) || newEnd > len(value) || before[:start]+value[start:newEnd]+before[oldEnd:] != value) {
			t.Fatalf("step %d: invalid batch delta %d/%d/%d", step, start, oldEnd, newEnd)
		}
	}
}
