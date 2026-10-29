package ecs

import "testing"

type scratchMapTestTag struct{}
type scratchSliceTestTag struct{}

func TestFrameScratchMapClearsAndReusesMap(t *testing.T) {
	world := NewWorld()
	first := world.FrameScratchMap[scratchMapTestTag, int, string]()
	first[1] = "old"
	second := world.FrameScratchMap[scratchMapTestTag, int, string]()
	if len(second) != 0 {
		t.Fatalf("scratch map retained %d entries", len(second))
	}
	second[2] = "reused"
	if first[2] != "reused" {
		t.Fatal("scratch map allocation was not reused")
	}
}

func TestFrameScratchSliceRetainsCapacity(t *testing.T) {
	world := NewWorld()
	first := world.FrameScratch[scratchSliceTestTag, int]()
	first.Values = append(first.Values, 1, 2, 3, 4)
	capacity := cap(first.Values)
	second := world.FrameScratch[scratchSliceTestTag, int]()
	if len(second.Values) != 0 || cap(second.Values) != capacity {
		t.Fatalf("scratch slice len/cap = %d/%d, want 0/%d", len(second.Values), cap(second.Values), capacity)
	}
}

func TestFrameScratchSliceReleasesUnusedTail(t *testing.T) {
	world := NewWorld()
	first := world.FrameScratch[scratchSliceTestTag, *int]()
	a, b := 1, 2
	first.Values = append(first.Values, &a, &b)
	second := world.FrameScratch[scratchSliceTestTag, *int]()
	second.Values = append(second.Values, &a)
	second.ReleaseUnused()
	retained := second.Values[:2]
	if retained[1] != nil {
		t.Fatal("scratch slice retained an unused pointer")
	}
}

func TestFrameScratchReleasesTailAcrossSkippedCleanup(t *testing.T) {
	world := NewWorld()
	scratch := world.FrameScratch[scratchSliceTestTag, *int]()
	value := 42
	scratch.Values = append(scratch.Values, &value, &value, &value)
	scratch = world.FrameScratch[scratchSliceTestTag, *int]()
	scratch.Values = append(scratch.Values, &value)
	// A consumer may skip ReleaseUnused on a frame that exits early.
	scratch = world.FrameScratch[scratchSliceTestTag, *int]()
	scratch.Values = append(scratch.Values, &value)
	scratch.ReleaseUnused()
	for _, pointer := range scratch.Values[1:3] {
		if pointer != nil {
			t.Fatal("scratch lost the previous high-water mark and retained a pointer")
		}
	}
}

// Reserve must survive a slowly rising wanted size: a per-frame +1 would
// otherwise reallocate the whole buffer on every frame.
func TestScratchReserveGrowsGeometrically(t *testing.T) {
	scratch := &Scratch[int]{}
	scratch.Values = append(scratch.Reserve(100), 1)
	for want := 101; want <= 200; want++ {
		if got := cap(scratch.Reserve(want)); got != 200 {
			t.Fatalf("capacity after reserve(%d) = %d, want 200", want, got)
		}
	}
	if got := cap(scratch.Reserve(201)); got != 400 {
		t.Fatalf("capacity after reserve(201) = %d, want 400", got)
	}
}
