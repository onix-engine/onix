package text

import "testing"

func TestDefaultFontIsSharedAndDoesNotAllocateAfterWarmup(t *testing.T) {
	first := DefaultFont()
	if first == nil {
		t.Skip("system font unavailable")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if DefaultFont() != first {
			panic("default font was reloaded")
		}
	}); allocations != 0 {
		t.Fatalf("default font allocated %v times", allocations)
	}
}
