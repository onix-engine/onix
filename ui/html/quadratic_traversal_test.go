package html

import (
	"fmt"
	"slices"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/html/css"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/style"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestHTMLAnimationTrackMatchingPreservesDuplicateTimes(t *testing.T) {
	state := htmlRenderState{}
	animation := htmlNodeAnimation{htmlNodeMotion: &htmlNodeMotion{}}
	track := func(name string) style.AnimationTrack {
		return style.AnimationTrack{Definition: style.CSSAnimation{Name: name}}
	}
	animation.Tracks = []htmlCSSAnimation{
		{Track: track("a"), Elapsed: 1}, {Track: track("b"), Elapsed: 2},
		{Track: track("a"), Elapsed: 3}, {Track: track("c"), Elapsed: 4},
	}
	next := []style.AnimationTrack{track("a"), track("a"), track("b"), track("a"), track("d")}
	syncHTMLAnimationTracks(&state, &animation, next)
	for i, want := range []float64{0, 1, 2, 3, 0} {
		if animation.Tracks[i].Elapsed != want || animation.Tracks[i].Track.Definition.Name != next[i].Definition.Name {
			t.Fatalf("track %d: %+v, want elapsed %v", i, animation.Tracks[i], want)
		}
	}
	for range 4 {
		syncHTMLAnimationTracks(&state, &animation, next)
	}
	if n := testing.AllocsPerRun(100, func() { syncHTMLAnimationTracks(&state, &animation, next) }); n != 0 {
		t.Fatalf("warm matching allocated %v objects", n)
	}
	syncHTMLAnimationTracks(&state, &animation, nil)
	syncHTMLAnimationTracks(&state, &animation, next)
	for _, active := range animation.Tracks {
		if active.Elapsed != 0 {
			t.Fatal("removed animation retained its time")
		}
	}
}

func TestHTMLMutationRootsCoalesceAndRefreshAncestry(t *testing.T) {
	root, left, right := dom.NewDocument(), dom.NewElement("div", nil), dom.NewElement("div", nil)
	child := dom.NewElement("span", nil)
	root.AppendChild(left)
	root.AppendChild(right)
	left.AppendChild(child)
	detached := dom.NewElement("div", nil)
	state := htmlRenderState{htmlDocumentState: htmlDocumentState{Root: root, Mutations: map[*dom.Node]htmlMutation{
		left: htmlMutationStyle, child: htmlMutationStyle, right: htmlMutationStyle, detached: htmlMutationStyle,
	}}}
	check := func(want ...*dom.Node) {
		t.Helper()
		if prepareHTMLMutationRoots(&state) {
			t.Fatal("style-only mutations forced layout")
		}
		if len(state.MutationRoots) != len(want) {
			t.Fatalf("roots=%v, want %v", state.MutationRoots, want)
		}
		for _, node := range want {
			if !slices.Contains(state.MutationRoots, node) {
				t.Fatal("missing mutation root")
			}
		}
	}
	check(left, right)
	delete(state.Mutations, left)
	check(child, right)
	left.Children = nil
	right.AppendChild(child)
	check(right)
	root.AppendChild(detached)
	check(right, detached)
	if n := testing.AllocsPerRun(100, func() { prepareHTMLMutationRoots(&state) }); n != 0 {
		t.Fatalf("warm root coalescing allocated %v objects", n)
	}
	stylesheet, err := css.Parse(`div + div {color:red}`)
	if err != nil {
		t.Fatal(err)
	}
	state.Stylesheet = stylesheet
	state.Mutations = map[*dom.Node]htmlMutation{child: htmlMutationStyle, right: htmlMutationStyle}
	check(root)
}

func quadraticHTMLBindingFixture(count int) (*ecs.World, ecs.Entity, []ecs.Entity) {
	world := ecs.NewWorld()
	state := htmlRenderState{htmlDocumentState: htmlDocumentState{Root: dom.NewDocument(), NodeByID: make(map[string]*dom.Node)}}
	owners := ecs.NewSpawner1[htmlRenderState](world)
	owner := owners.Spawn(state)
	children := ecs.NewSpawner3[htmlChild, scene.UITransform, scene.Parent](world)
	for i := range count {
		id := fmt.Sprint(i)
		node := dom.NewElement("div", map[string]string{"id": id})
		state.Root.AppendChild(node)
		state.NodeByID[id] = node
		state.NodeOffsets = append(state.NodeOffsets, htmlNodeOffset{ID: id, Offset: mgl32.Vec2{float32(i), 1}})
		state.Children = append(state.Children, children.Spawn(htmlChild{Owner: owner, Node: node}, scene.NewUITransform(), scene.Parent{Entity: owner}))
	}
	*ecs.GetMut[htmlRenderState](world, owner) = state
	SetNodeOffset(world, owner, "0", 0, 1)
	return world, owner, state.Children
}

func TestHTMLBindingBatchIgnoresDeadScrollAndRefreshesRoutes(t *testing.T) {
	world, owner, entities := quadraticHTMLBindingFixture(32)
	state := ecs.GetMut[htmlRenderState](world, owner)
	target := state.Root.Children[1]
	state.Root.Children = slices.Delete(state.Root.Children, 1, 2)
	state.Root.Children[0].AppendChild(target)
	first, second := htmlBindingArea(world, 0, 0), htmlBindingArea(world, 0, 0)
	SetNodeScroll(world, owner, "0", first, 0, 0)
	SetNodeScroll(world, owner, "1", second, 0, 0)
	parent, _ := ecs.Get[scene.Parent](world, entities[1])
	if parent.Entity != first {
		t.Fatal("first registered ancestor did not own the scroll")
	}
	world.Despawn(first)
	SetNodeOffset(world, owner, "0", 5, 7)
	parent, _ = ecs.Get[scene.Parent](world, entities[1])
	if parent.Entity != second {
		t.Fatal("dead earlier scroll prevented live descendant binding")
	}
	// Register the ancestor last: precedence follows registration, not depth.
	state = ecs.GetMut[htmlRenderState](world, owner)
	state.NodeOffsets[0], state.NodeOffsets[1] = state.NodeOffsets[1], state.NodeOffsets[0]
	SetNodeOffset(world, owner, "0", 5, 7)
	child, _ := ecs.Get[htmlChild](world, entities[1])
	if child.offset != (mgl32.Vec2{5, 7}) {
		t.Fatal("late ancestor offset lost to an earlier descendant")
	}
	if n := testing.AllocsPerRun(100, func() { SetNodeOffset(world, owner, "0", 5, 7) }); n != 0 {
		t.Fatalf("warm binding batch allocated %v objects", n)
	}
	state = ecs.GetMut[htmlRenderState](world, owner)
	state.Root.Children[0].Children = nil
	state.Root.AppendChild(target)
	SetNodeOffset(world, owner, "0", 5, 7)
	child, _ = ecs.Get[htmlChild](world, entities[1])
	if child.offset != (mgl32.Vec2{1, 1}) {
		t.Fatal("reparented node retained an old ancestor route")
	}
}

func BenchmarkHTMLNodeBindingTraversal(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world, owner, _ := quadraticHTMLBindingFixture(count)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				SetNodeOffset(world, owner, "0", 0, 1)
			}
		})
	}
}

func BenchmarkHTMLMutationRootTraversal(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			state := htmlRenderState{htmlDocumentState: htmlDocumentState{Root: dom.NewDocument(), Mutations: make(map[*dom.Node]htmlMutation)}}
			for range count {
				node := dom.NewElement("div", nil)
				state.Root.AppendChild(node)
				state.Mutations[node] = htmlMutationStyle
			}
			prepareHTMLMutationRoots(&state)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				prepareHTMLMutationRoots(&state)
			}
		})
	}
}

func TestHTMLComponentAttributeIndexPreservesScopeAndNormalizedNames(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="target" title="keep" class="scope user" data-old="remove"></div>`)
	node := incrementalHTMLNode(t, world, owner, "target")
	clearHTMLComponentAttributes(world, owner, node.node, []VNodeAttribute{{Name: " TITLE "}}, "scope")
	if title, _ := node.node.Attr("title"); title != "keep" {
		t.Fatal("normalized retained attribute was removed")
	}
	if class, _ := node.node.Attr("class"); class != "scope" {
		t.Fatal("scope class was removed or user class was retained")
	}
	if _, exists := node.node.Attr("data-old"); exists {
		t.Fatal("obsolete attribute was retained")
	}
}

func BenchmarkHTMLAnimationVisibilityTraversal(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			state := htmlRenderState{htmlDocumentState: htmlDocumentState{Root: dom.NewDocument()}, htmlStyleState: htmlStyleState{Styles: make(style.Map)}}
			parent := state.Root
			visible := &style.ComputedStyle{}
			visible.Display = style.Keyword("block")
			for i := range count {
				node := dom.NewElement("div", nil)
				parent.AppendChild(node)
				state.Styles[node] = visible
				if i == 0 {
					hidden := &style.ComputedStyle{}
					hidden.Display = style.Keyword("none")
					state.Styles[node] = hidden
				}
				parent = node
			}
			for range 10 {
				syncHTMLAnimations(&state)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				syncHTMLAnimations(&state)
			}
		})
	}
}

func BenchmarkHTMLComponentAttributeTraversal(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world := ecs.NewWorld()
			node := dom.NewElement("div", make(map[string]string))
			attributes := make([]VNodeAttribute, count)
			for i := range attributes {
				name := fmt.Sprintf("data-%d", i)
				node.Attributes[name] = "value"
				attributes[i] = VNodeAttribute{Name: name, Value: "value"}
			}
			clearHTMLComponentAttributes(world, 0, node, attributes, "")
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				clearHTMLComponentAttributes(world, 0, node, attributes, "")
			}
		})
	}
}
