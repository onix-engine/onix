package graphics

import "testing"

type scratchValueTestTag struct{}
type otherScratchValueTestTag struct{}

func TestScratchValueRetainsTypedStateWithoutAllocating(t *testing.T) {
	ctx := Canvas2DContext(323, 183)
	value := ScratchValue[scratchValueTestTag, int](ctx)
	*value = 42
	if got := ScratchValue[scratchValueTestTag, int](ctx); got != value || *got != 42 {
		t.Fatalf("scratch value = %p/%d, want %p/42", got, *got, value)
	}
	if other := ScratchValue[otherScratchValueTestTag, int](ctx); other == value {
		t.Fatal("distinct scratch tags shared storage")
	}

	if allocations := testing.AllocsPerRun(100, func() {
		ScratchValue[scratchValueTestTag, int](ctx)
	}); allocations != 0 {
		t.Fatalf("graphics scratch lookup allocated %.0f objects, want 0", allocations)
	}
}
