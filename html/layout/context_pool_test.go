package layout

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/diiyw/onix/html/dom"
)

func TestLayoutContextResetReleasesDocument(t *testing.T) {
	node := dom.NewElement("div", nil)
	ctx := newLayoutContext()
	ctx.inlineSegments = []inlineSegment{{Node: node}}
	ctx.controlTokens = []controlToken{{text: "document"}}
	ctx.controlTokens = ctx.controlTokens[:0]
	ctx.textScratch = []textLayoutScratch{{inlineSegments: []inlineSegment{{Node: node}}}}
	ctx.retained = new(Cache)
	ctx.floatContext = new(floatContext)
	ctx.err = errors.New("failed layout")
	ctx.imageSizes = map[string]imageSizeResult{"document": {valid: true}}
	ctx.reset()
	if ctx.retained != nil || ctx.floatContext != nil || ctx.err != nil || len(ctx.imageSizes) != 0 {
		t.Fatal("pooled context retained document state")
	}
	if ctx.inlineSegments[:cap(ctx.inlineSegments)][0].Node != nil || ctx.controlTokens[:cap(ctx.controlTokens)][0].text != "" || ctx.textScratch[0].inlineSegments[:1][0].Node != nil {
		t.Fatal("pooled backing array retained document references")
	}
	ctx.inlineSegments = make([]inlineSegment, 4097)
	ctx.shapeGlyphs = make([]shapedGlyph, 4097)
	ctx.textScratch[0].shapeGlyphs = make([]shapedGlyph, 4097)
	ctx.reset()
	if ctx.inlineSegments != nil || ctx.shapeGlyphs != nil || ctx.textScratch[0].shapeGlyphs != nil {
		t.Fatal("oversized layout workspace was retained")
	}
}

func TestBuildContextReusePreservesLayout(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div style="width:140px">Mixed 文本 <span>inline text</span><span style="display:inline-block;width:25px">box</span><input value="hello"></div>`)
	type snapshot struct {
		content   Rect
		fragments []TextFragment
	}
	var capture func(*Box) []snapshot
	capture = func(box *Box) []snapshot {
		fragments := slices.Clone(box.TextFragments)
		for index := range fragments {
			fragments[index].Glyphs = slices.Clone(fragments[index].Glyphs)
		}
		result := []snapshot{{content: box.Content, fragments: fragments}}
		for _, child := range box.Children {
			result = append(result, capture(child)...)
		}
		return result
	}
	first, err := BuildWithFonts(root, styles, face, face, 320, 200)
	if err != nil {
		t.Fatal(err)
	}
	want := capture(first)
	for range 20 {
		if _, err := BuildWithFonts(root, styles, face, nil, 80, 100); err != nil {
			t.Fatal(err)
		}
	}
	again, err := BuildWithFonts(root, styles, face, face, 320, 200)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(capture(first), want) || !reflect.DeepEqual(capture(again), want) {
		t.Fatal("pool reuse changed an existing or subsequent layout")
	}
	for worker := range 4 {
		t.Run(string(rune('A'+worker)), func(t *testing.T) {
			t.Parallel()
			for range 5 {
				box, err := BuildWithFonts(root, styles, face, face, 320, 200)
				if err != nil || !reflect.DeepEqual(capture(box), want) {
					t.Fatalf("concurrent layout changed: %v", err)
				}
			}
		})
	}
}

func TestLayoutScratchClearsFlushedLines(t *testing.T) {
	root, styles, face := retainedFixture(t, `<pre id="target">a b c d e f g h i j
z</pre>`)
	ctx := newLayoutContext()
	ctx.textScratch = []textLayoutScratch{{lineItems: make([]lineItem, 0, 1024)}}
	_, _, _, _, _, err := layoutText(TextLayoutOptions{
		Node: retainedNode(root, "target"), Face: face, StyleMap: styles, Width: 320, Context: ctx,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range ctx.textScratch[0].lineItems[:cap(ctx.textScratch[0].lineItems)] {
		if !reflect.DeepEqual(item, lineItem{}) {
			t.Fatal("a flushed line retained document references")
		}
	}
}
