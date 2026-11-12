package html

import (
	"strings"
	"testing"
	"unsafe"

	"github.com/onix-engine/onix/ecs"
)

func TestHTMLComponentKeyedCommitReleasesDescriptions(t *testing.T) {
	nodes := []VNode{
		VNodeElement(1, "li", []VNodeAttribute{{Name: "title", Value: "kept"}}, []VNode{TextVNode(2, "span", strings.Repeat("nested", 1024))}),
		TextVNode(3, "li", "removed"),
	}
	definition, err := Define[struct{}, struct{}](NewTemplate(`<ul data-ref="list"></ul>`), "", Behavior[struct{}, struct{}]{Render: func(_ RenderContext[struct{}, struct{}], v *View) {
		v.Keyed("list", nodes)
		// Replacing a pending list with a shorter one must also clear its tail.
		v.Keyed("list", nodes[:1])
	}})
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	pages := world.NewSpawner1[Surface]()
	owner := pages.Spawn(New(`<main></main>`))
	entity, err := definition.MountComponent(world, owner, struct{}{}, struct{}{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	render := func() {
		instance := world.GetMut[htmlComponentInstance](entity)
		definition.runtime.render(world, entity, instance)
	}
	render()
	instance := world.GetRef[htmlComponentInstance](entity)
	list := instance.Keyed["list"]
	storage := list.Pending.Values[:cap(list.Pending.Values)]
	for range 3 {
		render()
		if list.Pending.Active || len(list.Pending.Values) != 0 || &storage[0] != &list.Pending.Values[:cap(list.Pending.Values)][0] {
			t.Fatal("commit did not release descriptions or reuse its buffer")
		}
		for _, node := range storage {
			if node.Tag != "" || node.Text != "" || node.Attributes != nil || node.Children != nil || node.Key != 0 {
				t.Fatal("committed VNode buffer retained caller data")
			}
		}
	}
	if len(list.Items) != 1 || htmlNodeText(list.Items[0].Node) != nodes[0].Children[0].Text || nodes[0].Attributes[0].Value != "kept" || nodes[1].Text != "removed" {
		t.Fatal("clearing descriptions changed the DOM or caller-owned nodes")
	}
	if allocations := testing.AllocsPerRun(100, render); allocations != 0 {
		t.Fatalf("warm keyed commits allocated %g times", allocations)
	}
}

func BenchmarkHTMLComponentKeyedCommit(b *testing.B) {
	nodes := make([]VNode, 1000)
	for i := range nodes {
		nodes[i] = TextVNode(uint64(i+1), "li", "item")
	}
	definition, err := Define[struct{}, struct{}](NewTemplate(`<ul data-ref="list"></ul>`), "", Behavior[struct{}, struct{}]{Render: func(_ RenderContext[struct{}, struct{}], v *View) {
		v.Keyed("list", nodes)
	}})
	if err != nil {
		b.Fatal(err)
	}
	world := ecs.NewWorld()
	pages := world.NewSpawner1[Surface]()
	owner := pages.Spawn(New(`<main></main>`))
	entity, err := definition.MountComponent(world, owner, struct{}{}, struct{}{}, 1)
	if err != nil {
		b.Fatal(err)
	}
	render := func() {
		instance := world.GetMut[htmlComponentInstance](entity)
		definition.runtime.render(world, entity, instance)
	}
	for range 3 {
		render()
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		render()
	}
	instance := world.GetRef[htmlComponentInstance](entity)
	capacity := cap(instance.Keyed["list"].Pending.Values)
	b.ReportMetric(float64(uintptr(capacity)*unsafe.Sizeof(VNode{})), "vnode-B")
}
