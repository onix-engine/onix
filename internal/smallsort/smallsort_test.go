package smallsort

import (
	"cmp"
	"slices"
	"testing"
)

func TestInsertionSortFunc(t *testing.T) {
	values := []int{7, 1, 4, 1, 9, 2}
	InsertionSortFunc(values, cmp.Compare[int])
	if want := []int{1, 1, 2, 4, 7, 9}; !slices.Equal(values, want) {
		t.Fatalf("sorted values = %v, want %v", values, want)
	}
}

func TestInsertionSortFuncDoesNotAllocate(t *testing.T) {
	values := [8]int{7, 1, 4, 1, 9, 2, 8, 3}
	if allocations := testing.AllocsPerRun(100, func() {
		current := values
		InsertionSortFunc(current[:], cmp.Compare[int])
	}); allocations != 0 {
		t.Fatalf("small insertion sort allocated %.1f times", allocations)
	}
}

func TestSortFuncOrdersLargeSliceWithoutAllocating(t *testing.T) {
	values := [64]int{}
	for index := range values {
		values[index] = len(values) - index
	}
	if allocations := testing.AllocsPerRun(100, func() {
		current := values
		SortFunc(current[:], cmp.Compare[int])
		for index := 1; index < len(current); index++ {
			if current[index-1] > current[index] {
				t.Fatal("large sort did not order values")
			}
		}
	}); allocations != 0 {
		t.Fatalf("large smallsort allocated %.1f times", allocations)
	}
}

func BenchmarkInsertionSortFunc16(b *testing.B) {
	values := [16]int{13, 2, 9, 1, 14, 6, 3, 11, 8, 0, 12, 5, 10, 4, 15, 7}
	b.ReportAllocs()
	for b.Loop() {
		current := values
		InsertionSortFunc(current[:], cmp.Compare[int])
	}
}

func BenchmarkSortFunc64(b *testing.B) {
	values := [64]int{}
	for index := range values {
		values[index] = len(values) - index
	}
	b.ReportAllocs()
	for b.Loop() {
		current := values
		SortFunc(current[:], cmp.Compare[int])
	}
}
