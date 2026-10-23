package brotli

import "testing"

func TestHistogramReindexInPlace(t *testing.T) {
	out := []int{10, 20, 30, 40, 50}
	symbols := []uint32{2, 4, 2, 3, 4, 1}

	got := histogramReindexInPlace(out, symbols, uint(len(symbols)))
	if got != 4 {
		t.Fatalf("histogram count = %d, want 4", got)
	}
	wantOut := []int{30, 50, 40, 20, 50}
	for index, want := range wantOut[:got] {
		if out[index] != want {
			t.Fatalf("out[%d] = %d, want %d", index, out[index], want)
		}
	}
	wantSymbols := []uint32{0, 1, 0, 2, 1, 3}
	for index, want := range wantSymbols {
		if symbols[index] != want {
			t.Fatalf("symbols[%d] = %d, want %d", index, symbols[index], want)
		}
	}
}
