package layout

import (
	"strings"
	"testing"

	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/style"
)

func TestInlineBlockWidthConstraintsUseContainingBlock(t *testing.T) {
	for rule, width := range map[string]float64{"max-width:100%": 240, "max-width:50%": 120, "width:50%": 120} {
		t.Run(rule, func(t *testing.T) {
			_, _, _, boxes := tableFixture(t, `<p id="host"><code id="code">`+strings.Repeat("game-common/app/", 8)+`</code></p>`, `#host{width:240px}#code{display:inline-block;box-sizing:border-box;padding:2px 6px;white-space:pre-wrap;overflow-wrap:break-word;`+rule+`}`)
			code := boxes["code"]
			if !closeFloat(code.BorderBox().Width, width) {
				t.Fatalf("inline width=%g, want %g", code.BorderBox().Width, width)
			}
			if len(code.TextFragments) < 2 || code.TextFragments[len(code.TextFragments)-1].Line == 0 {
				t.Fatal("constrained inline block did not wrap")
			}
		})
	}
}

func TestInlineBlockPaddedReflowDoesNotAllocate(t *testing.T) {
	root, styles, face := retainedFixture(t, `<p style="width:240px"><code id="code" style="display:inline-block;max-width:100%;box-sizing:border-box;padding:2px 6px;white-space:pre-wrap;overflow-wrap:break-word">`+strings.Repeat("game-common/app/", 8)+`</code> after</p>`)
	t.Cleanup(func() { _ = face.Close() })
	var cache Cache
	code := retainedNode(root, "code")
	dirty := []*dom.Node{code}
	updateRetained(t, &cache, root, styles, face, nil, true)
	mutable := styles.Exclusive(code)
	if allocations := testing.AllocsPerRun(50, func() {
		for _, padding := range [...]float64{8, 6} {
			mutable.Padding.Left, mutable.Padding.Right = style.LengthPx(padding), style.LengthPx(padding)
			if _, err := cache.Update(root, styles, face, face, 320, 200, dirty, nil, false, false); err != nil {
				panic(err)
			}
		}
	}); allocations != 0 {
		t.Fatalf("warm padded inline layout allocated %g objects", allocations)
	}
}
