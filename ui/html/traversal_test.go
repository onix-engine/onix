package html

import (
	"strings"
	"testing"

	"github.com/diiyw/onix/internal/dom"
	"github.com/diiyw/onix/internal/layout"
	"github.com/diiyw/onix/internal/style"
)

func TestHTMLMetadataPassPreservesCSSSourceOrder(t *testing.T) {
	root, css, err := parseHTMLDocument(`<html><head><title>discard</title><style>.x{color:red}</style><style>.x{color:green}</style></head><body><div id="keep">visible</div><style>.x{color:blue}</style><script>discard</script><p>end</p></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	if css != ".x{color:red}.x{color:green}.x{color:blue}" {
		t.Fatalf("CSS order = %q", css)
	}
	var text strings.Builder
	var visit func(*dom.Node)
	visit = func(n *dom.Node) {
		if n.Type == dom.ElementNode && isHTMLMetadataElement(n.Data) {
			t.Fatalf("metadata retained: %s", n.Data)
		}
		if n.Type == dom.TextNode {
			text.WriteString(n.Data)
		}
		for _, child := range n.Children {
			if child.Parent != n {
				t.Fatal("parent changed")
			}
			visit(child)
		}
	}
	visit(root)
	if text.String() != "visibleend" || findHTMLNodeByID(root, "keep") == nil {
		t.Fatalf("retained DOM text=%q", text.String())
	}
}

func TestHTMLOverflowExtentIncludesNestedDescendants(t *testing.T) {
	grandchild := &layout.Box{Content: layout.Rect{X: 160, Y: 90, Width: 40, Height: 30}}
	child := &layout.Box{Content: layout.Rect{X: 30, Y: 40, Width: 20, Height: 10}, Children: []*layout.Box{grandchild}}
	root := &layout.Box{Content: layout.Rect{X: 10, Y: 20, Width: 100, Height: 50}, Children: []*layout.Box{child, &layout.Box{Content: layout.Rect{X: -10, Y: -10, Width: 5, Height: 5}}}}
	extents := make(map[*layout.Box]htmlOverflowExtent)
	htmlOverflowContentExtents(root, extents)
	if extents[root] != (htmlOverflowExtent{width: 190, height: 100}) || extents[child] != (htmlOverflowExtent{width: 170, height: 80}) {
		t.Fatalf("root=%+v child=%+v", extents[root], extents[child])
	}
	if n := testing.AllocsPerRun(100, func() { htmlOverflowContentExtents(root, extents) }); n != 0 {
		t.Fatalf("allocs=%v", n)
	}
}

func TestHTMLOverflowExtentClipsEachAxis(t *testing.T) {
	for _, axes := range []struct {
		x, y string
		want htmlOverflowExtent
	}{
		{"clip", "visible", htmlOverflowExtent{100, 180}},
		{"visible", "clip", htmlOverflowExtent{280, 100}},
		{"hidden", "auto", htmlOverflowExtent{100, 100}},
	} {
		computed := &style.ComputedStyle{}
		computed.OverflowX = style.Value{Kind: style.ValueKeyword, Keyword: axes.x}
		computed.OverflowY = style.Value{Kind: style.ValueKeyword, Keyword: axes.y}
		child := &layout.Box{
			Content:  layout.Rect{Width: 100, Height: 50},
			Style:    computed,
			Children: []*layout.Box{{Content: layout.Rect{X: 100, Y: 50, Width: 180, Height: 130}}},
		}
		root := &layout.Box{Content: layout.Rect{Width: 100, Height: 100}, Children: []*layout.Box{child}}
		extents := make(map[*layout.Box]htmlOverflowExtent)
		htmlOverflowContentExtents(root, extents)
		if extents[root] != axes.want || extents[child] != (htmlOverflowExtent{280, 180}) {
			t.Fatalf("overflow %s/%s: parent=%+v child=%+v", axes.x, axes.y, extents[root], extents[child])
		}
		if allocations := testing.AllocsPerRun(20, func() { htmlOverflowContentExtents(root, extents) }); allocations != 0 {
			t.Fatalf("clipped extent traversal allocated %g", allocations)
		}
	}
}

func BenchmarkHTMLOverflowExtents(b *testing.B) {
	root := &layout.Box{Content: layout.Rect{Width: 100, Height: 100}}
	for i := range 100 {
		child := &layout.Box{Content: layout.Rect{X: float64(i), Width: 100, Height: 100}}
		root.Children = append(root.Children, child)
		for j := range 100 {
			child.Children = append(child.Children, &layout.Box{Content: layout.Rect{X: float64(j), Width: 10, Height: 10}})
		}
	}
	extents := make(map[*layout.Box]htmlOverflowExtent, 10101)
	htmlOverflowContentExtents(root, extents)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		htmlOverflowContentExtents(root, extents)
	}
}
