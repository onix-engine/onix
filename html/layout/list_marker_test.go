package layout

import (
	"testing"

	"github.com/onix-engine/onix/html/dom"
	"github.com/onix-engine/onix/html/style"
)

func TestListMarkerSharesFirstLineBaseline(t *testing.T) {
	for name, content := range map[string]string{
		"text":      `<span id="label">game-common/app/bean/redis/user/HUser.php</span>`,
		"code":      `<code id="label">game-common/app/bean/redis/user/HUser.php</code>`,
		"paragraph": `<p><code id="label">game-common/app/bean/redis/user/HUser.php</code></p>`,
	} {
		for _, lineHeight := range []string{"1", "1.7", "2"} {
			t.Run(name+"/"+lineHeight, func(t *testing.T) {
				_, _, _, boxes := tableFixture(t, `<ul><li id="item">`+content+`</li></ul>`,
					`ul {font-size:16px;line-height:`+lineHeight+`} code {display:inline-block;font-family:monospace;font-size:0.95em;padding:1px 6px}`)
				item := boxes["item"]
				want := item.TextFragments[0].Glyphs[0].Y
				if label := boxes["label"]; label != nil {
					want = firstBoxBaseline(label)
				}
				marker := item.TextFragments[len(item.TextFragments)-1]
				if marker.Text != "•" || len(marker.Glyphs) == 0 {
					t.Fatal("missing list marker")
				}
				if got := marker.Glyphs[0].Y; !closeFloat(got, want) {
					t.Fatalf("marker baseline = %g, text baseline = %g", got, want)
				}
			})
		}
	}
}

func TestListMarkerReflowDoesNotAllocate(t *testing.T) {
	root, styles, face := retainedFixture(t, `<ul><li id="item" style="line-height:1.7"><code style="display:inline-block;padding:1px 6px">game-common/app/bean/redis/user/HUser.php</code></li></ul>`)
	t.Cleanup(func() { _ = face.Close() })
	var cache Cache
	item := retainedNode(root, "item")
	dirty := []*dom.Node{item}
	updateRetained(t, &cache, root, styles, face, nil, true)
	mutable := styles.Exclusive(item)
	if allocations := testing.AllocsPerRun(50, func() {
		for _, size := range [...]float64{18, 16} {
			mutable.FontSize = style.LengthPx(size)
			if _, err := cache.Update(root, styles, face, face, 320, 200, dirty, nil, false, false); err != nil {
				panic(err)
			}
		}
	}); allocations != 0 {
		t.Fatalf("warm list marker layout allocated %g objects", allocations)
	}
}
