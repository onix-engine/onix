package piecetree

import (
	"strings"
	"testing"
)

func TestSnapshotsRetainOldVersionsAcrossTailEdits(t *testing.T) {
	var tree Tree
	value := strings.Repeat("中🙂\n", 10000)
	tree.Reset(value)
	snapshots, expected := []string{tree.String()}, []string{value}
	for _, edit := range []struct {
		start, end int
		value      string
	}{
		{len(value), len(value), "tail"},
		{len(value) + 4, len(value) + 4, "more"},
		{len(value) + 2, len(value) + 8, ""},
		{len(value) + 2, len(value) + 2, "replace"},
		{0, 3, "首"},
	} {
		tree.Replace(edit.start, edit.end, edit.value)
		value = value[:edit.start] + edit.value + value[edit.end:]
		snapshots = append(snapshots, tree.String())
		expected = append(expected, value)
	}
	for _, tail := range []string{"start", strings.Repeat("界", 100000)} {
		tree.Replace(tree.Len(), tree.Len(), tail)
		value += tail
		snapshots = append(snapshots, tree.String())
		expected = append(expected, value)
	}
	// External snapshots, batched mutations and compaction also release builder
	// ownership without modifying strings retained by renderers or workers.
	tree.ApplySnapshot("external", 0, tree.Len(), len("external"))
	tree.Replace(tree.Len(), tree.Len(), "1")
	tree.Replace(tree.Len(), tree.Len(), "2")
	if tree.String() != "external12" {
		t.Fatal(tree.String())
	}
	tree.Compact()
	for i, snapshot := range snapshots {
		if snapshot != expected[i] {
			t.Fatalf("snapshot %d changed", i)
		}
	}
}

func TestWarmTailSnapshotsDoNotAllocate(t *testing.T) {
	var tree Tree
	tree.Reset(strings.Repeat("a", 2<<20))
	tree.Reserve(4096, 4096)
	tree.Replace(tree.Len(), tree.Len(), "x")
	tree.String() // Establish append storage, without touching the original.
	if n := testing.AllocsPerRun(100, func() {
		tree.Replace(tree.Len(), tree.Len(), "x")
		tree.String()
	}); n != 0 {
		t.Fatalf("tail snapshot allocated %g times", n)
	}
	if n := testing.AllocsPerRun(100, func() {
		tree.Replace(tree.Len()-1, tree.Len(), "")
		tree.String()
	}); n != 0 {
		t.Fatalf("suffix deletion allocated %g times", n)
	}
}

func BenchmarkTailSnapshot(b *testing.B) {
	var tree Tree
	tree.Reset(strings.Repeat("a", 2<<20))
	tree.Reserve(4096, 4096)
	b.ReportAllocs()
	for b.Loop() {
		tree.Replace(tree.Len(), tree.Len(), "x")
		tree.String()
	}
}
