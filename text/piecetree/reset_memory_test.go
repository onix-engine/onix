package piecetree

import (
	"strings"
	"testing"
	"unsafe"
)

func TestResetReclaimsPreviousDocumentCapacity(t *testing.T) {
	var tree Tree
	large := strings.Repeat("a", 1<<20)
	tree.Replace(0, 0, large)
	snapshot := tree.String()
	peak := uintptr(cap(tree.added)) + uintptr(cap(tree.nodes))*unsafe.Sizeof(node{})
	tree.Reset("small")
	if tree.String() != "small" || snapshot != large || cap(tree.added) > 64<<10 || cap(tree.nodes) > 128 {
		t.Fatal("reset retained large buffers or changed a published snapshot")
	}
	tree.Reserve(256, 64)
	if tree.RetainedBytes() != len("small")+cap(tree.added) {
		t.Fatal("retained text storage omitted unused edit capacity")
	}
	if allocations := testing.AllocsPerRun(100, func() { tree.Reset("small") }); allocations != 0 {
		t.Fatalf("reset of a small warm document allocated %g times", allocations)
	}
	t.Logf("edit/node capacity before reset: %d bytes; after: %d/%d entries", peak, cap(tree.added), cap(tree.nodes))
}
