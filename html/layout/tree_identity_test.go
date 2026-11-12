package layout

import (
	"strings"
	"testing"

	"github.com/onix-engine/onix/html/dom"
	"github.com/onix-engine/onix/html/style"
)

func TestRetainedBoxIDsSurviveTopologyChanges(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div id="list"><div id="a"><div id="leaf"></div></div><div id="b"></div></div><div id="destination"><div id="slot"></div></div>`)
	t.Cleanup(func() { _ = face.Close() })
	var cache Cache
	updateRetained(t, &cache, root, styles, face, nil, true)
	original := make(map[*dom.Node]*Box, len(cache.nodes))
	ids := make(map[*dom.Node]uint64, len(cache.nodes))
	for node, box := range cache.nodes {
		original[node], ids[node] = box, box.StableID
	}
	list, a, slot := retainedNode(root, "list"), retainedNode(root, "a"), retainedNode(root, "slot")
	for _, mutate := range []func(){
		func() { list.InsertBefore(&dom.Node{Type: dom.ElementNode, Data: "div"}, a) },
		func() { list.RemoveChild(a); list.AppendChild(a) },
		func() { list.RemoveChild(a); slot.AppendChild(a) },
		func() { slot.Parent.RemoveChild(slot); list.AppendChild(slot) },
	} {
		mutate()
		var err error
		styles, err = style.ComputeWithViewport(root, nil, style.Viewport{Width: 320, Height: 200})
		if err != nil {
			t.Fatal(err)
		}
		got := updateRetained(t, &cache, root, styles, face, nil, true)
		for node, box := range original {
			if cache.BoxForNode(node) != box || box.StableID == 0 || box.StableID != ids[node] {
				t.Fatal("topology edit replaced a retained box or changed its ID")
			}
		}
		seen := make(map[uint64]bool, len(cache.nodes))
		for _, box := range cache.nodes {
			if box.StableID == 0 || seen[box.StableID] {
				t.Fatal("zero or duplicate box ID")
			}
			seen[box.StableID] = true
		}
		fresh, err := BuildWithFonts(root, styles, face, face, 320, 200)
		if err != nil {
			t.Fatal(err)
		}
		assertRetainedMatchesFull(t, got, fresh)
		if got.GeometryHash() != fresh.GeometryHash() {
			t.Fatal("geometry hash depends on allocation history")
		}
	}
}

func TestRetainedBoxIndexRetiresIDs(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div id="parent"><div id="child"></div></div>`)
	t.Cleanup(func() { _ = face.Close() })
	var cache Cache
	updateRetained(t, &cache, root, styles, face, nil, true)
	parent, child := retainedNode(root, "parent"), retainedNode(root, "child")
	previous := cache.BoxForNode(child)
	child.Parent.RemoveChild(child)
	updateRetained(t, &cache, root, styles, face, nil, true)
	if cache.BoxForNode(child) != nil {
		t.Fatal("removed node remains in the layout index")
	}
	parent.AppendChild(child)
	updateRetained(t, &cache, root, styles, face, nil, true)
	if current := cache.BoxForNode(child); current == nil || current == previous || current.StableID <= previous.StableID {
		t.Fatal("reattached node reused a retired box ID")
	}
	lastID := cache.nextID
	replacement, replacementStyles, replacementFace := retainedFixture(t, `<div></div>`)
	t.Cleanup(func() { _ = replacementFace.Close() })
	updateRetained(t, &cache, replacement, replacementStyles, replacementFace, nil, true)
	if cache.BoxForNode(parent) != nil || cache.BoxForNode(child) != nil || cache.Root.StableID <= lastID {
		t.Fatal("document replacement retained old membership or reused an ID")
	}
}

func TestRetainedTopologyRotationDoesNotAllocate(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div id="list">`+strings.Repeat(`<div><div></div></div>`, 64)+`</div>`)
	t.Cleanup(func() { _ = face.Close() })
	list := retainedNode(root, "list")
	var cache Cache
	updateRetained(t, &cache, root, styles, face, nil, true)
	rotate := func() {
		first := list.Children[0]
		copy(list.Children, list.Children[1:])
		list.Children[len(list.Children)-1] = first
		if _, err := cache.Update(root, styles, face, face, 320, 200, nil, nil, true, false); err != nil {
			panic(err)
		}
	}
	for range 4 {
		rotate()
	}
	if allocations := testing.AllocsPerRun(100, rotate); allocations != 0 {
		t.Fatalf("warm topology rotation allocated %v objects", allocations)
	}
	if cache.Work.Created != 0 {
		t.Fatal("rotation created new boxes")
	}
}
