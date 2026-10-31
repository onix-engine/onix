package layout

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/html/css"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/style"
)

func retainedFixture(tb testing.TB, source string) (*dom.Node, style.Map, *font.Face) {
	tb.Helper()
	root, err := dom.Parse(source)
	if err != nil {
		tb.Fatal(err)
	}
	styles, err := style.ComputeWithViewport(root, nil, style.Viewport{Width: 320, Height: 200})
	if err != nil {
		tb.Fatal(err)
	}
	face, err := font.LoadDefault()
	if err != nil {
		tb.Fatal(err)
	}
	return root, styles, face
}

func retainedNode(root *dom.Node, id string) *dom.Node {
	if value, _ := root.Attr("id"); value == id {
		return root
	}
	for _, child := range root.Children {
		if node := retainedNode(child, id); node != nil {
			return node
		}
	}
	return nil
}

func updateRetained(tb testing.TB, cache *Cache, root *dom.Node, styles style.Map, face *font.Face, dirty []*dom.Node, topology bool) *Box {
	tb.Helper()
	box, err := cache.Update(root, styles, face, face, 320, 200, dirty, nil, topology, false)
	if err != nil {
		tb.Fatal(err)
	}
	return box
}

func assertRetainedMatchesFull(t *testing.T, got, want *Box) {
	t.Helper()
	if got.StableID != want.StableID || got.Node != want.Node || !reflect.DeepEqual(got.Style, want.Style) || got.Padding != want.Padding || got.Border != want.Border || got.Margin != want.Margin || len(got.Children) != len(want.Children) {
		detail := ""
		if debugStyleDiff != nil && got.Style != nil && want.Style != nil {
			detail = debugStyleDiff(got.Style, want.Style)
		}
		t.Fatalf("box identity/style differs: %s id=%v node=%v style=%v pad=%v border=%v margin=%v children=%d/%d %s",
			got.StableID, got.StableID == want.StableID, got.Node == want.Node, reflect.DeepEqual(got.Style, want.Style),
			got.Padding == want.Padding, got.Border == want.Border, got.Margin == want.Margin, len(got.Children), len(want.Children), detail)
	}
	assertRetainedRect(t, got.Content, want.Content)
	assertRetainedRect(t, got.FlowMarginBox, want.FlowMarginBox)
	if math.Abs(got.TextHeight-want.TextHeight) > 1e-8 {
		t.Fatalf("text height differs at %s: %g != %g", got.StableID, got.TextHeight, want.TextHeight)
	}
	assertRetainedText(t, got.TextFragments, want.TextFragments)
	if len(got.ImageFragments) != len(want.ImageFragments) || len(got.ControlFragments) != len(want.ControlFragments) {
		t.Fatal("fragment counts differ")
	}
	for i, fragment := range got.ImageFragments {
		assertRetainedRect(t, fragment.Rect, want.ImageFragments[i].Rect)
	}
	for i, fragment := range got.ControlFragments {
		expected := want.ControlFragments[i]
		assertRetainedRect(t, fragment.Rect, expected.Rect)
		assertRetainedRect(t, fragment.Content, expected.Content)
		assertRetainedText(t, fragment.TextFragments, expected.TextFragments)
	}
	for i, child := range got.Children {
		if child.Parent != got {
			t.Fatal("stale parent")
		}
		assertRetainedMatchesFull(t, child, want.Children[i])
	}
}

var debugStyleDiff func(a, b *style.ComputedStyle) string

func assertRetainedRect(t *testing.T, a, b Rect) {
	t.Helper()
	if math.Abs(a.X-b.X) > 1e-8 || math.Abs(a.Y-b.Y) > 1e-8 || math.Abs(a.Width-b.Width) > 1e-8 || math.Abs(a.Height-b.Height) > 1e-8 {
		t.Fatalf("rect differs: %+v != %+v", a, b)
	}
}
func assertRetainedText(t *testing.T, a, b []TextFragment) {
	t.Helper()
	if len(a) != len(b) {
		t.Fatalf("text fragments: %d != %d", len(a), len(b))
	}
	for i, f := range a {
		w := b[i]
		assertRetainedRect(t, f.Rect, w.Rect)
		if f.Text != w.Text || len(f.Glyphs) != len(w.Glyphs) || !reflect.DeepEqual(f.Style, w.Style) {
			t.Fatal("text/style/glyph count differs")
		}
		for j, g := range f.Glyphs {
			h := w.Glyphs[j]
			if g.Rune != h.Rune || math.Abs(g.X-h.X) > 1e-8 || math.Abs(g.Y-h.Y) > 1e-8 || g.Advance != h.Advance {
				t.Fatalf("glyph differs: %+v != %+v", g, h)
			}
		}
	}
}

func TestRetainedLayoutMatchesFullAcrossRepeatedChanges(t *testing.T) {
	cases := []struct{ name, source string }{
		{"blocks", `<div id="target">One</div><div>Following text</div><div><div>Nested sibling</div></div>`},
		{"relative parent", `<div style="position:relative;left:5px;top:7px"><div id="target">One</div><div>Following text</div></div>`},
		{"absolute sibling", `<div style="position:relative"><div id="target">One</div><div style="position:absolute;bottom:0;right:0">Bottom</div></div>`},
		{"escaping absolute", `<div><div id="target">One</div><div><div style="position:absolute;top:4px;left:8px">Absolute</div>Text</div></div>`},
		{"flex row", `<div style="display:flex;width:280px;gap:5px"><div id="target">One</div><div style="flex:1">Flexible text with wrap</div></div><div>Following</div>`},
		{"flex column", `<div style="display:flex;flex-direction:column;height:180px"><div id="target">One</div><div style="flex:1">Flexible</div></div>`},
		{"flex wrap", `<div style="display:flex;flex-wrap:wrap;width:180px"><div id="target">One</div><div style="width:100px">Second</div><div style="width:100px">Third</div></div>`},
		{"inline block", `<div>Before<span id="target" style="display:inline-block">One</span>After</div><div>Tail</div>`},
		{"inline flex", `<div>Before<span style="display:inline-flex"><span id="target">One</span><span>Two</span></span>After</div>`},
		{"percentage descendants", `<div id="target" style="height:100px;width:200px"><div style="height:50%;width:50%">Percentage</div></div><div>Following</div>`},
		{"controls", `<div id="target">One</div><div><input value="Control"/><button>Button</button></div>`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root, styles, face := retainedFixture(t, test.source)
			var cache Cache
			target := retainedNode(root, "target")
			original := styles[target].Width
			initial := updateRetained(t, &cache, root, styles, face, nil, true)
			for i := 0; i < 30; i++ {
				if i%2 == 0 {
					styles.Exclusive(target).Width = style.LengthPx(90)
				} else {
					styles.Exclusive(target).Width = original
				}
				got := updateRetained(t, &cache, root, styles, face, []*dom.Node{target}, false)
				if got != initial {
					t.Fatal("root box replaced")
				}
				full, err := BuildWithFonts(root, styles, face, face, 320, 200)
				if err != nil {
					t.Fatal(err)
				}
				assertRetainedMatchesFull(t, got, full)
			}
		})
	}
}

func TestRetainedLayoutTopologyAndText(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div id="parent"><div id="target">One</div><div id="sibling">Second</div></div><div id="destination">Tail</div>`)
	var cache Cache
	target, parent, sibling, destination := retainedNode(root, "target"), retainedNode(root, "parent"), retainedNode(root, "sibling"), retainedNode(root, "destination")
	updateRetained(t, &cache, root, styles, face, nil, true)
	siblingBox := cache.nodes[sibling]
	steps := []func(){
		func() { target.Children[0].Data = "Many words of text which now wrap onto several lines" },
		func() { styles.Exclusive(target).Display = style.Keyword("none") },
		func() { styles.Exclusive(target).Display = style.Keyword("block") },
		func() { parent.Children = parent.Children[:1]; destination.AppendChild(sibling) },
		func() { styles.Exclusive(destination).Display = style.Keyword("flex") },
		func() { styles.Exclusive(sibling).Order = style.Number(-1) },
		func() { styles.Exclusive(destination).Display = style.Keyword("block") },
	}
	for i, step := range steps {
		step()
		got := updateRetained(t, &cache, root, styles, face, []*dom.Node{target, parent, destination, sibling}, true)
		full, err := BuildWithFonts(root, styles, face, face, 320, 200)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(fmt.Sprint(i), func(t *testing.T) { assertRetainedMatchesFull(t, got, full) })
		if cache.nodes[sibling] != siblingBox {
			t.Fatal("sibling box replaced")
		}
	}
}

func TestRetainedLayoutSkipsAndTranslatesUnaffectedSubtrees(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div id="target" style="height:20px">One</div><div id="sibling"><div id="text">Unchanged text</div></div>`)
	var cache Cache
	updateRetained(t, &cache, root, styles, face, nil, true)
	target, text := retainedNode(root, "target"), retainedNode(root, "text")
	textBox := cache.nodes[text]
	glyph := &textBox.TextFragments[0].Glyphs[0]
	styles.Exclusive(target).Height = style.LengthPx(50)
	updateRetained(t, &cache, root, styles, face, []*dom.Node{target}, false)
	if cache.Work.Reflowed != 2 || cache.Work.Translated != 1 || cache.Work.Created != 0 {
		t.Fatalf("unexpected work: %+v", cache.Work)
	}
	if &textBox.TextFragments[0].Glyphs[0] != glyph {
		t.Fatal("translated text was shaped again")
	}
	full, err := BuildWithFonts(root, styles, face, face, 320, 200)
	if err != nil {
		t.Fatal(err)
	}
	assertRetainedMatchesFull(t, cache.Root, full)
	allocations := testing.AllocsPerRun(100, func() {
		if _, err := cache.Update(root, styles, face, face, 320, 200, nil, nil, false, false); err != nil {
			panic(err)
		}
	})
	if allocations != 0 || cache.Work.Reflowed != 0 || cache.Work.Reused != 1 {
		t.Fatalf("clean update allocations=%v work=%+v", allocations, cache.Work)
	}
}

func BenchmarkRetainedBlockLayout(b *testing.B) {
	for _, count := range []int{10, 1000} {
		for _, retained := range []bool{false, true} {
			b.Run(fmt.Sprintf("nodes=%d/retained=%t", count, retained), func(b *testing.B) {
				root, styles, face := retainedFixture(b, `<div id="target" style="width:100px;height:30px">One</div>`+strings.Repeat(`<div>Unchanged text</div>`, count))
				target := retainedNode(root, "target")
				dirty := []*dom.Node{target}
				var cache Cache
				updateRetained(b, &cache, root, styles, face, nil, true)
				for range 2 {
					styles.Exclusive(target).Width = style.LengthPx(110)
					updateRetained(b, &cache, root, styles, face, dirty, false)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					styles.Exclusive(target).Width = style.LengthPx(float64(100 + i%2*10))
					var err error
					if retained {
						_, err = cache.Update(root, styles, face, face, 320, 200, dirty, nil, false, false)
					} else {
						_, err = BuildWithFonts(root, styles, face, face, 320, 200)
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// Verify the public style-change signal carries topology differences before
// the previous computed style is overwritten in place.
func TestRetainedLayoutCascadeTopologySignal(t *testing.T) {
	root, styles, _ := retainedFixture(t, `<div id="target">One</div>`)
	target := retainedNode(root, "target")
	target.Attributes["style"] = "display:flex"
	var cache style.MutationCache
	var changes []style.NodeChange
	_, _, err := style.RecomputeSubtreesWithChanges(root, &css.Stylesheet{}, style.Viewport{Width: 320, Height: 200}, nil, styles, []*dom.Node{target}, &changes, &cache)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) == 0 || !changes[0].Tree {
		t.Fatal("display mutation did not mark topology")
	}
}

func TestRetainedLayoutWarmResizeDoesNotAllocate(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div id="target" style="width:100px;height:30px">One</div><div><div>Unaffected</div></div>`)
	var cache Cache
	target := retainedNode(root, "target")
	dirty := []*dom.Node{target}
	updateRetained(t, &cache, root, styles, face, nil, true)
	mutable := styles.Exclusive(target)
	allocations := testing.AllocsPerRun(100, func() {
		for _, width := range [...]float64{110, 100} {
			mutable.Width = style.LengthPx(width)
			if _, err := cache.Update(root, styles, face, face, 320, 200, dirty, nil, false, false); err != nil {
				panic(err)
			}
		}
	})
	if allocations != 0 {
		t.Fatalf("warm resize allocated %v objects", allocations)
	}
}

func TestRetainedLayoutViewportAndPositionChanges(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div id="target"><div style="width:50%;height:20px">Percent</div><div style="position:absolute;bottom:0;right:0">Absolute</div></div>`)
	var cache Cache
	target := retainedNode(root, "target")
	updateRetained(t, &cache, root, styles, face, nil, true)
	for i := 0; i < 20; i++ {
		width := float64(320 + i%2*100)
		if i%2 == 0 {
			styles.Exclusive(target).Position = style.Keyword("relative")
			styles.Exclusive(target).Height = style.LengthPx(100)
		} else {
			styles.Exclusive(target).Position = style.Keyword("static")
			styles.Exclusive(target).Height = style.Auto()
		}
		got, err := cache.Update(root, styles, face, face, width, 200, []*dom.Node{target}, nil, true, false)
		if err != nil {
			t.Fatal(err)
		}
		full, err := BuildWithFonts(root, styles, face, face, width, 200)
		if err != nil {
			t.Fatal(err)
		}
		assertRetainedMatchesFull(t, got, full)
	}
}

func TestRetainedLayoutMixedMutationSequence(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div id="container" style="width:280px;position:relative"><div id="first"><span>Wrapping words of text</span></div><div id="flex" style="display:flex;flex-wrap:wrap"><div id="item">One</div><div style="width:70px">Two</div></div><div id="last"><div style="position:absolute;bottom:0;right:0">Absolute</div>Last</div></div>`)
	nodes := []*dom.Node{retainedNode(root, "container"), retainedNode(root, "first"), retainedNode(root, "flex"), retainedNode(root, "item"), retainedNode(root, "last")}
	declarations := []string{
		"display:block;width:90px", "display:block;width:80%;padding:3px", "display:flex;flex-wrap:wrap;width:180px;gap:5px", "display:block;position:relative;left:5px;top:3px", "display:block;position:absolute;bottom:4px;right:2px;width:100px", "display:none", "display:inline-block;width:120px", "display:flex;flex-direction:column;height:100px", "display:block;width:250px;font-size:20px", "display:block;min-height:40px;max-width:140px",
	}
	var cache Cache
	updateRetained(t, &cache, root, styles, face, nil, true)
	var styleCache style.MutationCache
	var changes []style.NodeChange
	var dirty []*dom.Node
	for i := 0; i < 100; i++ {
		node := nodes[(i*7+i/5)%len(nodes)]
		node.Attributes["style"] = declarations[(i*3+i/10)%len(declarations)]
		changes = changes[:0]
		dirty = dirty[:0]
		_, _, err := style.RecomputeSubtreesWithChanges(root, nil, style.Viewport{Width: 320, Height: 200}, nil, styles, []*dom.Node{root}, &changes, &styleCache)
		if err != nil {
			t.Fatal(err)
		}
		topology := false
		for _, change := range changes {
			if change.Impact == style.ChangeRebuild {
				dirty = append(dirty, change.Node)
			}
			topology = topology || change.Tree
		}
		got := updateRetained(t, &cache, root, styles, face, dirty, topology)
		full, err := BuildWithFonts(root, styles, face, face, 320, 200)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(fmt.Sprint(i), func(t *testing.T) { assertRetainedMatchesFull(t, got, full) })
	}
}

func TestRetainedCacheReportsRetiredSubtreeOnce(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div id="parent"><div id="child"><div id="grandchild">Text</div></div></div><div id="other">Keep</div>`)
	cache := &Cache{}
	updateRetained(t, cache, root, styles, face, nil, true)
	parent, child, grandchild := retainedNode(root, "parent"), retainedNode(root, "child"), retainedNode(root, "grandchild")
	oldChild, oldGrandchild := cache.nodes[child], cache.nodes[grandchild]
	child.Parent = nil
	clear(parent.Children)
	parent.Children = parent.Children[:0]
	updateRetained(t, cache, root, styles, face, []*dom.Node{parent}, true)
	if len(cache.Removed) != 2 {
		t.Fatalf("removed boxes=%d, want 2", len(cache.Removed))
	}
	found := map[*Box]bool{}
	for _, box := range cache.Removed {
		found[box] = true
	}
	if !found[oldChild] || !found[oldGrandchild] || cache.nodes[child] != nil || cache.nodes[grandchild] != nil {
		t.Fatal("removed subtree identities differ")
	}
	scratch := cache.Removed
	updateRetained(t, cache, root, styles, face, nil, false)
	if len(cache.Removed) != 0 {
		t.Fatal("retired boxes reported twice")
	}
	for _, box := range scratch {
		if box != nil {
			t.Fatal("retired subtree retained in scratch")
		}
	}
}
