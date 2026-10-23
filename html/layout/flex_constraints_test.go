package layout

import (
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/style"
	"testing"
)

func TestRetainedFlexConstraintsWarmResizeDoesNotAllocate(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div id="target" style="display:flex;width:160px;height:80px"><div style="flex:1;min-width:80px"></div><div style="flex:1;max-width:20px"></div><div style="flex:1;margin-top:auto;padding:10px"></div></div>`)
	var cache Cache
	target := retainedNode(root, "target")
	dirty := []*dom.Node{target}
	updateRetained(t, &cache, root, styles, face, nil, true)
	mutable := styles.Exclusive(target)
	allocations := testing.AllocsPerRun(100, func() {
		for _, width := range [...]float64{180, 160} {
			mutable.Width = style.LengthPx(width)
			if _, err := cache.Update(root, styles, face, face, 320, 200, dirty, nil, false, false); err != nil {
				panic(err)
			}
		}
	})
	if allocations != 0 {
		t.Fatalf("warm constrained flex resize allocated %v objects", allocations)
	}
}
