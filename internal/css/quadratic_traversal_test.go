package css

import (
	"fmt"
	"testing"

	"github.com/diiyw/onix/internal/dom"
)

func TestCachedTypeIndicesMatchFreshScansAfterReorder(t *testing.T) {
	parent := dom.NewElement("div", nil)
	for _, tag := range []string{"x-one", "span", "x-two", "x-one", "span", "x-three", "x-two"} {
		parent.AppendChild(dom.NewText("ignored"))
		parent.AppendChild(dom.NewElement(tag, nil))
	}
	ctx := NewMatchContext()
	check := func() {
		t.Helper()
		for _, fromEnd := range []bool{true, false} {
			for index := len(parent.Children) - 1; index >= 0; index-- {
				node := parent.Children[index]
				if node.Type != dom.ElementNode {
					continue
				}
				want := childIndex(node, true, fromEnd, nil)
				if got := childIndex(node, true, fromEnd, ctx); got != want {
					t.Fatalf("%s reverse=%v: index %d, want %d", node.Data, fromEnd, got, want)
				}
			}
		}
	}
	check()
	parent.Children[1], parent.Children[11] = parent.Children[11], parent.Children[1]
	ctx.Reset()
	check()
	// A second parent must start its type counters at one.
	other := dom.NewElement("div", nil)
	for _, tag := range []string{"span", "x-two", "span"} {
		other.AppendChild(dom.NewElement(tag, nil))
	}
	for _, node := range other.Children {
		if got, want := childIndex(node, true, false, ctx), childIndex(node, true, false, nil); got != want {
			t.Fatalf("second parent index = %d, want %d", got, want)
		}
	}
	if allocations := testing.AllocsPerRun(100, func() { ctx.Reset(); check() }); allocations != 0 {
		t.Fatalf("warmed type indexing allocated %v times", allocations)
	}
}

func BenchmarkTypeIndexTraversal(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			parent := dom.NewElement("div", nil)
			for index := range count {
				parent.AppendChild(dom.NewElement(fmt.Sprintf("x-tag-%d", index), nil))
			}
			ctx := NewMatchContext()
			run := func() {
				ctx.Reset()
				for _, node := range parent.Children {
					if childIndex(node, true, false, ctx) != 1 {
						b.Fatal("incorrect type index")
					}
				}
			}
			run()
			b.ReportAllocs()
			for b.Loop() {
				run()
			}
		})
	}
}
