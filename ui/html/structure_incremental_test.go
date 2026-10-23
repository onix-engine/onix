package html

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
)

func TestHTMLStructureStylesStayWithinParent(t *testing.T) {
	const css = `<style>#parent:empty{background:red}#parent{color:blue}#parent span:first-child{color:red}#parent span:last-child{font-size:20px}#parent span:nth-child(2){background:#ffff00}</style>`
	const before = `<div id="parent"><span id="first">First</span><span id="second">Second</span></div>`
	const siblings = `<div id="other"><input value="Unchanged"/></div>`
	cases := []struct {
		name, after string
		mutate      func(*testing.T, *ecs.World, ecs.Entity)
	}{
		{"remove", `<div id="parent"><span id="second">Second</span></div>`, func(t *testing.T, w *ecs.World, o ecs.Entity) { incrementalHTMLNode(t, w, o, "first").Remove() }},
		{"append", `<div id="parent"><span id="first">First</span><span id="second">Second</span><span id="first">First</span></div>`, func(t *testing.T, w *ecs.World, o ecs.Entity) {
			incrementalHTMLNode(t, w, o, "parent").Append(incrementalHTMLNode(t, w, o, "first").Clone())
		}},
		{"clear", `<div id="parent"></div>`, func(t *testing.T, w *ecs.World, o ecs.Entity) { incrementalHTMLNode(t, w, o, "parent").Clear() }},
		{"replace", `<div id="parent">Replacement</div>`, func(t *testing.T, w *ecs.World, o ecs.Entity) {
			incrementalHTMLNode(t, w, o, "parent").SetText("Replacement")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			world, owner := incrementalHTMLFixture(t, css+before+siblings+strings.Repeat(`<div>Unchanged</div>`, 1000))
			tc.mutate(t, world, owner)
			incrementalSync(t, world)
			state, _ := ecs.Get[htmlRenderState](world, owner)
			if state.SyncStats.Styled > 8 {
				t.Fatalf("unrelated branches restyled: %+v", state.SyncStats)
			}
			assertHTMLPartialMatchesFresh(t, world, owner, css+tc.after+siblings+strings.Repeat(`<div>Unchanged</div>`, 1000))
			for node := range state.Styles {
				if !htmlNodeWithin(node, state.Root) {
					t.Fatal("detached node retained by style map")
				}
			}
		})
	}
}

func TestHTMLStructureStyleRootsMergeAndRemainDisjoint(t *testing.T) {
	source := `<div id="outer"><div id="left"><span id="child">A</span></div><div id="right"><span>B</span></div></div><div>Unchanged</div>`
	world, owner := incrementalHTMLFixture(t, source)
	child := incrementalHTMLNode(t, world, owner, "child").Clone()
	incrementalHTMLNode(t, world, owner, "left").Clear()
	incrementalHTMLNode(t, world, owner, "right").Clear()
	incrementalSync(t, world)
	state, _ := ecs.Get[htmlRenderState](world, owner)
	if state.SyncStats.Styled != 2 {
		t.Fatalf("disjoint roots visited %d nodes", state.SyncStats.Styled)
	}
	incrementalHTMLNode(t, world, owner, "left").Append(child)
	incrementalHTMLNode(t, world, owner, "outer").Clear()
	incrementalSync(t, world)
	state, _ = ecs.Get[htmlRenderState](world, owner)
	if state.SyncStats.Styled != 1 {
		t.Fatalf("removed root was restyled: %+v", state.SyncStats)
	}
	assertHTMLPartialMatchesFresh(t, world, owner, `<div id="outer"></div><div>Unchanged</div>`)
}

func TestHTMLStructureMediaStylesStayLocal(t *testing.T) {
	const css = `<style>@media(min-width:100px){#parent:empty{color:red}}</style>`
	world, owner := incrementalHTMLFixture(t, css+`<div id="parent"><span>A</span></div><div>Unchanged</div>`)
	incrementalHTMLNode(t, world, owner, "parent").Clear()
	incrementalSync(t, world)
	state, _ := ecs.Get[htmlRenderState](world, owner)
	if state.SyncStats.Styled > 2 {
		t.Fatal("media query forced unrelated branches to restyle")
	}
	assertHTMLPartialMatchesFresh(t, world, owner, css+`<div id="parent"></div><div>Unchanged</div>`)
}

func TestHTMLStructureFilterDefinitionsInvalidateOutsideBranch(t *testing.T) {
	const prefix = `<style>#target{filter:url(#fx)}</style><svg><filter id="fx">`
	const primitive = `<feGaussianBlur id="blur" stdDeviation="2"></feGaussianBlur>`
	const suffix = `</filter></svg><div id="target">Filtered</div><div>Unchanged</div>`
	for _, action := range []string{"remove", "clear", "replace", "append"} {
		t.Run(action, func(t *testing.T) {
			world, owner := incrementalHTMLFixture(t, prefix+primitive+suffix)
			filter := incrementalHTMLNode(t, world, owner, "fx")
			blur := incrementalHTMLNode(t, world, owner, "blur")
			// Prime the incremental definition cache before testing its invalidation.
			incrementalHTMLNode(t, world, owner, "target").SetAttribute("class", "warm")
			incrementalSync(t, world)
			middle := ""
			switch action {
			case "remove":
				blur.Remove()
			case "clear":
				filter.Clear()
			case "replace":
				filter.SetText("Text")
				middle = "Text"
			case "append":
				filter.Append(blur.Clone())
				middle = primitive + primitive
			}
			incrementalSync(t, world)
			fresh, freshOwner := incrementalHTMLFixture(t, prefix+middle+strings.Replace(suffix, `id="target"`, `id="target" class="warm"`, 1))
			got, _ := ecs.Get[htmlRenderState](world, owner)
			want, _ := ecs.Get[htmlRenderState](fresh, freshOwner)
			node := incrementalHTMLNode(t, world, owner, "target")
			expected := incrementalHTMLNode(t, fresh, freshOwner, "target")
			if !reflect.DeepEqual(got.Styles[node.node], want.Styles[expected.node]) {
				t.Fatal("filter consumer retained old definition")
			}
			if got.SyncStats.Styled < 4 {
				t.Fatal("filter changed without document cascade")
			}
		})
	}
}

func BenchmarkHTMLStructureLocalCascade(b *testing.B) {
	for _, count := range []int{10, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world, owner := incrementalHTMLFixture(b, `<div id="parent"><span>Target</span></div>`+strings.Repeat(`<div>Unchanged</div>`, count))
			parent := incrementalHTMLNode(b, world, owner, "parent")
			child := Node{world: world, owner: owner, node: parent.node.Children[0]}
			for range 2 {
				child.Remove()
				incrementalSync(b, world)
				parent.Append(child)
				incrementalSync(b, world)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if i%2 == 0 {
					child.Remove()
				} else {
					parent.Append(child)
				}
				incrementalSync(b, world)
			}
			b.StopTimer()
			state, _ := ecs.Get[htmlRenderState](world, owner)
			b.ReportMetric(float64(state.SyncStats.Styled), "styled/last-op")
		})
	}
}

func TestHTMLStructureRemovedFilterSubtreeAndReattach(t *testing.T) {
	const fallback = `<svg><filter id="fx"><feGaussianBlur stdDeviation="1"></feGaussianBlur></filter></svg>`
	const overriding = `<svg id="definitions"><filter id="fx"><feGaussianBlur stdDeviation="2"></feGaussianBlur></filter></svg>`
	const target = `<div id="target" style="filter:url(#fx)">Filtered</div><div>Unchanged</div>`
	for _, action := range []string{"remove", "clear", "replace"} {
		t.Run(action, func(t *testing.T) {
			world, owner := incrementalHTMLFixture(t, fallback+overriding+target)
			definitions := incrementalHTMLNode(t, world, owner, "definitions")
			parent := Node{world: world, owner: owner, node: definitions.node.Parent}
			// Prime both the definition cache and cascade before removing its subtree.
			incrementalHTMLNode(t, world, owner, "target").SetAttribute("class", "warm")
			incrementalSync(t, world)
			switch action {
			case "remove":
				definitions.Remove()
			case "clear":
				definitions.Clear()
			case "replace":
				definitions.SetText("Text")
			}
			incrementalSync(t, world)
			state, _ := ecs.Get[htmlRenderState](world, owner)
			node := incrementalHTMLNode(t, world, owner, "target")
			fresh, freshOwner := incrementalHTMLFixture(t, fallback+target)
			expected, _ := ecs.Get[htmlRenderState](fresh, freshOwner)
			expectedNode := incrementalHTMLNode(t, fresh, freshOwner, "target")
			if !reflect.DeepEqual(state.Styles[node.node].Filters, expected.Styles[expectedNode.node].Filters) {
				t.Fatal("removed filter subtree still overrides fallback definition")
			}
			if action == "remove" {
				parent.Append(definitions)
				incrementalSync(t, world)
				restored, _ := ecs.Get[htmlRenderState](world, owner)
				if reflect.DeepEqual(restored.Styles[node.node].Filters, expected.Styles[expectedNode.node].Filters) {
					t.Fatal("reinserted filter definition was not discovered")
				}
			}
		})
	}
}

func TestHTMLWarmStructureInvalidationDoesNotAllocate(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="parent"><span id="child">Text</span></div>`)
	parent := incrementalHTMLNode(t, world, owner, "parent")
	child := incrementalHTMLNode(t, world, owner, "child")
	mutate := func() { child.Remove(); parent.Append(child) }
	for range 5 {
		mutate()
	}
	if n := testing.AllocsPerRun(100, mutate); n != 0 {
		t.Fatalf("warm structural invalidation allocated %v times", n)
	}
	incrementalSync(t, world)
}
