package layout

import (
	"runtime"
	"strings"
	"testing"
	"weak"

	"github.com/onix-engine/onix/font"
	"github.com/onix-engine/onix/html/dom"
	"github.com/onix-engine/onix/internal/reuse"
)

func textWorkspaceCapacities(ctx *layoutContext) [4]int {
	result := [4]int{cap(ctx.lineItems), cap(ctx.inlineSegments), cap(ctx.controlTokens), cap(ctx.shapeGlyphs)}
	for _, scratch := range ctx.textScratch {
		result[0] += cap(scratch.lineItems)
		result[1] += cap(scratch.inlineSegments)
		result[2] += cap(scratch.controlTokens)
		result[3] += cap(scratch.shapeGlyphs)
	}
	return result
}

func TestRetainedLayoutReclaimsWorkspacePeaks(t *testing.T) {
	root, styles, face := retainedFixture(t, `<main><div id="target" style="white-space:nowrap">`+strings.Repeat("x", 4096)+strings.Repeat(`<span>word </span>`, 1024)+`</div><section id="blocks">`+strings.Repeat(`<div style="height:1px"></div>`, 1024)+`</section></main>`)
	defer face.Close()
	var cache Cache
	target := retainedNode(root, "target")
	blocks := retainedNode(root, "blocks")
	updateRetained(t, &cache, root, styles, face, nil, true)
	peak := textWorkspaceCapacities(&cache.context)
	oldIndex := cache.nodes
	if peak[0] < 1024 || peak[1] < 1024 || peak[3] < 4096 {
		t.Fatalf("fixture did not grow text workspaces: %v", peak)
	}
	for _, node := range target.Children[1:] {
		node.Parent = nil
	}
	clear(target.Children[1:])
	target.Children = target.Children[:1]
	target.Children[0].Data = "small"
	dom.SynchronizeTextContent(target)
	for _, node := range blocks.Children {
		node.Parent = nil
	}
	clear(blocks.Children)
	blocks.Children = blocks.Children[:0]
	dirty := []*dom.Node{target, blocks}
	updateRetained(t, &cache, root, styles, face, dirty, true)
	for range reuse.RetentionFrames*2 + 2 {
		updateRetained(t, &cache, root, styles, face, dirty, false)
	}
	after := textWorkspaceCapacities(&cache.context)
	if after[0] >= peak[0] || after[1] >= peak[1] || after[3] >= peak[3] || cap(cache.Changed) > 64 || cap(cache.Removed) > 64 {
		t.Fatalf("layout retained its peak: workspace %v -> %v, changed/removed=%d/%d", peak, after, cap(cache.Changed), cap(cache.Removed))
	}
	probe := dom.NewElement("probe")
	oldIndex[probe] = &Box{}
	if cache.nodes[probe] != nil {
		t.Fatal("layout node map retained its historical buckets")
	}
	fresh, err := BuildWithFonts(root, styles, face, face, 320, 200)
	if err != nil {
		t.Fatal(err)
	}
	assertRetainedMatchesFull(t, cache.Root, fresh)
	if n := testing.AllocsPerRun(100, func() {
		updateRetained(t, &cache, root, styles, face, dirty, false)
	}); n != 0 {
		t.Fatalf("reclaimed layout allocated %g on the warm path", n)
	}
	t.Logf("line/segment/control/glyph workspace capacities: %v -> %v", peak, after)
}

func abandonedShapedFace(t *testing.T) weak.Pointer[font.Face] {
	t.Helper()
	face := loadLayoutTestFace(t)
	if _, _, err := shapeToken(face, textToken{Text: "weak font lifetime"}, 16, 0, nil); err != nil {
		t.Fatal(err)
	}
	return weak.Make(face)
}

func TestShapeTokenCacheDoesNotRetainFont(t *testing.T) {
	pointer := abandonedShapedFace(t)
	for range 10 {
		runtime.GC()
		if pointer.Value() == nil {
			return
		}
	}
	t.Fatal("global shaping cache kept an unused font alive")
}
