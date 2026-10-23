package css

import (
	"testing"
	"unsafe"

	"github.com/diiyw/onix/html/dom"
)

func TestMatchContextResetInvalidatesBorrowedData(t *testing.T) {
	parent := dom.NewElement("section", nil)
	first := dom.NewElement("span", map[string]string{"id": "first", "class": "item selected"})
	second := dom.NewElement("input", map[string]string{"id": "second", "class": "item"})
	parent.AppendChild(first)
	parent.AppendChild(dom.NewText("ignored"))
	parent.AppendChild(second)
	sheet, err := Parse(`#first, #second, .selected, .item:nth-child(2), span:first-of-type, input:last-child, .item + .item, section:focus-within {color:red}`)
	if err != nil {
		t.Fatal(err)
	}
	state := &State{Focused: second}
	ctx := NewMatchContext()
	check := func() {
		t.Helper()
		for _, node := range []*dom.Node{parent, first, second} {
			for _, selector := range sheet.Rules[0].Selectors {
				if got, want := MatchWithStateAndContext(selector, node, state, ctx), MatchWithState(selector, node, state); got != want {
					t.Fatalf("%s on %s: cached=%v fresh=%v", selector.String(), node.Data, got, want)
				}
			}
		}
	}
	check()
	ctx.Reset()
	if len(ctx.nodes) != 0 || len(ctx.typeCounts) != 0 {
		t.Fatal("reset retained DOM or type keys")
	}
	first.Attributes["id"] = "second"
	second.Attributes["id"] = "first"
	first.Attributes["class"] = "item"
	second.Attributes["class"] = "item selected"
	parent.Children[0], parent.Children[2] = parent.Children[2], parent.Children[0]
	state.Focused = first
	check()
	ctx.Reset()
	state.Focused = nil
	check()
}

func TestMatchContextOversizedWorkspaceReleased(t *testing.T) {
	ctx := NewMatchContext()
	for range maxCachedMatchNodes + 1 {
		ctx.store(dom.NewElement("div", nil), nodeMatchData{})
	}
	ctx.typeCounts = map[string]int32{"custom-tag": 1}
	ctx.Reset()
	if ctx.nodes != nil || ctx.typeCounts != nil {
		t.Fatal("oversized matcher workspace was pooled")
	}
	node := dom.NewElement("span", map[string]string{"class": "item"})
	sheet, err := Parse(`.item {color:red}`)
	if err != nil {
		t.Fatal(err)
	}
	if !MatchWithStateAndContext(sheet.Rules[0].Selectors[0], node, nil, ctx) {
		t.Fatal("matcher failed after releasing oversized workspace")
	}
	ctx.Reset()
	if allocs := testing.AllocsPerRun(100, func() { MatchWithStateAndContext(sheet.Rules[0].Selectors[0], node, nil, ctx); ctx.Reset() }); allocs != 0 {
		t.Fatalf("small matcher did not return to zero allocation reuse: %g", allocs)
	}
}

func TestMatchContextRecordLayout(t *testing.T) {
	// The cache copies no extra strings; this is a regression guard for padding.
	if unsafe.Sizeof(uintptr(0)) == 8 && unsafe.Sizeof(nodeMatchData{}) > 72 {
		t.Fatalf("matcher record grew to %d bytes", unsafe.Sizeof(nodeMatchData{}))
	}
}
