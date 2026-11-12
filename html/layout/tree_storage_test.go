package layout

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkLayoutTopologyRotation(b *testing.B) {
	for _, depth := range []int{1, 8} {
		b.Run(fmt.Sprintf("depth=%d", depth), func(b *testing.B) {
			subtree := strings.Repeat(`<div>`, depth) + strings.Repeat(`</div>`, depth)
			root, styles, face := retainedFixture(b, `<div id="list">`+strings.Repeat(subtree, 1024/depth)+`</div>`)
			b.Cleanup(func() { _ = face.Close() })
			list := retainedNode(root, "list")
			var cache Cache
			updateRetained(b, &cache, root, styles, face, nil, true)
			rotate := func() {
				first := list.Children[0]
				copy(list.Children, list.Children[1:])
				list.Children[len(list.Children)-1] = first
				updateRetained(b, &cache, root, styles, face, nil, true)
			}
			for range 4 {
				rotate()
			}
			b.ReportAllocs()
			for b.Loop() {
				rotate()
			}
		})
	}
}

func BenchmarkBuildBoxTree(b *testing.B) {
	for _, depth := range []int{1, 8} {
		b.Run(fmt.Sprintf("depth=%d", depth), func(b *testing.B) {
			subtree := strings.Repeat(`<div>`, depth) + strings.Repeat(`</div>`, depth)
			root, styles, face := retainedFixture(b, `<div>`+strings.Repeat(subtree, 1024/depth)+`</div>`)
			b.Cleanup(func() { _ = face.Close() })
			b.ReportAllocs()
			for b.Loop() {
				if buildTree(root, styles) == nil {
					b.Fatal("missing layout tree")
				}
			}
		})
	}
}
