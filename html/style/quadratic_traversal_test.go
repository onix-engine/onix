package style

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/diiyw/onix/html/css"
	"github.com/diiyw/onix/html/dom"
)

func TestIndexedKeyframesKeepLastMatchingDefinition(t *testing.T) {
	root := dom.NewDocument()
	node := dom.NewElement("div", nil)
	root.AppendChild(node)
	sheet, err := css.Parse(`
div { animation: fade 1s linear forwards; color: red !important; }
@keyframes fade { to { opacity: .2; color: blue; } }
@media (min-width: 200px) { @keyframes fade { to { opacity: .8; background-color: green; } } }
@media (min-width: 400px) { @keyframes fade { to { opacity: .9; } } }
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ width, opacity float64 }{{100, .2}, {300, .8}, {500, .9}, {100, .2}} {
		viewport := Viewport{Width: test.width, Height: 100}
		styles, err := ComputeWithViewport(root, sheet, viewport)
		if err != nil {
			t.Fatal(err)
		}
		value := styles[node]
		tracks := CompileAnimationTracks(node, value, sheet, &QueryEnvironment{Viewport: viewport, Styles: styles})
		if len(tracks) != 1 || math.Abs(tracks[0].Sample(1, BaseAnimationValue(value), false).Opacity-test.opacity) > 1e-10 {
			t.Fatalf("width %g chose wrong keyframes: %+v", test.width, tracks)
		}
		if !value.AnimatedOpacity || value.AnimatedPaint&AnimatedColor != 0 || value.AnimatedPaint&AnimatedBackgroundColor == 0 {
			t.Fatalf("animation topology or important priority changed: %+v", value)
		}
	}
}

func BenchmarkAnimationCascadeTraversal(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			root := dom.NewDocument()
			node := dom.NewElement("div", nil)
			root.AppendChild(node)
			var source strings.Builder
			names := make([]string, count)
			for index := range names {
				names[index] = fmt.Sprintf("anim%d", index)
				fmt.Fprintf(&source, "@keyframes %s { to { opacity: .5; } }\n", names[index])
			}
			fmt.Fprintf(&source, "div { animation-name: %s; animation-duration: 1s; }", strings.Join(names, ","))
			sheet, err := css.Parse(source.String())
			if err != nil {
				b.Fatal(err)
			}
			styles, err := Compute(root, sheet)
			if err != nil {
				b.Fatal(err)
			}
			var cache MutationCache
			var changes []NodeChange
			roots := []*dom.Node{node}
			run := func() {
				changes = changes[:0]
				if _, _, err := RecomputeSubtreesWithChanges(root, sheet, Viewport{}, nil, styles, roots, &changes, &cache); err != nil {
					b.Fatal(err)
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
