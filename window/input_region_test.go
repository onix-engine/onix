package window

import "testing"

func TestNativeInputRegionUsesViewportCoordinates(t *testing.T) {
	viewport := newViewportState(300, 300)
	if _, ok := viewport.Update(600, 300, 600, 300); !ok {
		t.Fatal("expected viewport layout")
	}

	got := viewport.nativeInputRegion([]InputRegionRect{{X: 10, Y: 20, Width: 30, Height: 40}})
	want := InputRegionRect{X: 160, Y: 20, Width: 30, Height: 40}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("native input region = %+v, want %+v", got, want)
	}
}

func TestNativeInputRegionPreservesDisabledAndEmptyStates(t *testing.T) {
	viewport := newViewportState(100, 100)
	viewport.Update(100, 100, 100, 100)
	if got := viewport.nativeInputRegion(nil); got != nil {
		t.Fatalf("nil region must disable custom input shaping: %+v", got)
	}
	if got := viewport.nativeInputRegion([]InputRegionRect{}); got == nil || len(got) != 0 {
		t.Fatalf("empty region must keep input shaping enabled: %+v", got)
	}
}
