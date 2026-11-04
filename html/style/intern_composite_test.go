package style

import (
	"fmt"
	"testing"
)

func TestStyleInternCompositeValues(t *testing.T) {
	var index styleIndex
	first := ComputedStyle{BoxShadow: []Shadow{{BlurRadius: LengthPx(4)}}}
	second := ComputedStyle{BoxShadow: []Shadow{{BlurRadius: LengthPx(8)}}}
	a, b := index.commit(&first, nil), index.commit(&second, nil)
	if a == b || !styleEqual(a, &first) || !styleEqual(b, &second) {
		t.Fatal("different shadow contents shared an immutable style")
	}
	copyOfFirst := ComputedStyle{BoxShadow: []Shadow{{BlurRadius: LengthPx(4)}}}
	if index.commit(&copyOfFirst, nil) != a {
		t.Fatal("equal contents in a separate slice did not share a style")
	}
	if index.commit(&ComputedStyle{}, nil) == index.commit(&ComputedStyle{BoxShadow: []Shadow{}}, nil) {
		t.Fatal("nil and empty slices lost their distinct style values")
	}
}

// Same-length composite properties must not collapse the intern table into
// one probe chain as the number of distinct styles grows.
func BenchmarkStyleInternCompositeValues(b *testing.B) {
	for _, count := range []int{128, 1024} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			values := make([]ComputedStyle, count)
			var index styleIndex
			for i := range values {
				values[i].BoxShadow = []Shadow{{BlurRadius: LengthPx(float64(i + 1))}}
				index.commit(&values[i], nil)
			}
			b.ReportAllocs()
			for b.Loop() {
				for i := range values {
					index.commit(&values[i], nil)
				}
			}
		})
	}
}
