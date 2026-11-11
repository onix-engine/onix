package ecs

import (
	"testing"

	"github.com/onix-engine/onix/internal/reuse"
)

func TestFrameScratchMapReclaimsPeakAndIdleReferences(t *testing.T) {
	w := NewWorld()
	old := w.FrameScratchMap[scratchMapTestTag, int, string]()
	for i := range 4096 {
		old[i] = "peak"
	}
	w.Maintain()
	var current map[int]string
	for range reuse.RetentionFrames + 1 {
		current = w.FrameScratchMap[scratchMapTestTag, int, string]()
		current[0], current[1] = "live", "live"
		w.Maintain()
	}
	old[10000] = "retired"
	if len(current) != 2 || current[0] != "live" {
		t.Fatal("peak reclamation retained the original map or lost live values")
	}
	w.Maintain()
	if len(current) != 0 {
		t.Fatal("idle workspace retains its last values")
	}
	for range reuse.RetentionFrames {
		w.Maintain()
	}
	next := w.FrameScratchMap[scratchMapTestTag, int, string]()
	current[20000] = "old idle map"
	if len(next) != 0 {
		t.Fatal("idle workspace retains its old buckets")
	}
	next[1] = "writable"
}

func TestFrameScratchMapMultipleUsesRetainFramePeak(t *testing.T) {
	w := NewWorld()
	frame := func() {
		values := w.FrameScratchMap[scratchMapTestTag, int, int]()
		for i := range 256 {
			values[i] = i
		}
		values = w.FrameScratchMap[scratchMapTestTag, int, int]()
		values[0] = 1
		w.Maintain()
	}
	frame()
	if allocations := testing.AllocsPerRun(100, frame); allocations != 0 {
		t.Fatalf("a large earlier batch was forgotten: %g allocations", allocations)
	}
}

func TestArchetypeEdgeSparseIDsAndUpdates(t *testing.T) {
	var a archetype
	for i := 31; i >= 0; i-- {
		a.edges = setArchetypeEdge(a.edges, componentID(i*97+11), uint32(i))
	}
	for i := range 32 {
		if target, ok := a.edge(componentID(i*97 + 11)); !ok || target != uint32(i) {
			t.Fatalf("sparse edge %d selected %d, %v", i, target, ok)
		}
	}
	for _, id := range []componentID{0, 12, 10000, ^componentID(0)} {
		if _, ok := a.edge(id); ok {
			t.Fatalf("missing edge %d was found", id)
		}
	}
	a.edges = setArchetypeEdge(a.edges, 11, 99)
	if target, ok := a.edge(11); !ok || target != 99 {
		t.Fatal("edge replacement failed")
	}
}
