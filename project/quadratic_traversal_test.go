package project

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/ui"
	"github.com/go-gl/mathgl/mgl32"
)

func traversalModel(count int) compiledModelResource {
	resource := compiledModelResource{Vertices: make([]render.Vertex3D, 3), Indices: []uint32{0, 1, 2}, Nodes: make([]ModelNode, count)}
	for index := range resource.Nodes {
		resource.Nodes[index] = ModelNode{Parent: int32(index - 1), Local: [16]float32(mgl32.Ident4()), World: [16]float32(mgl32.Ident4())}
	}
	return resource
}

func TestModelHierarchyValidationChecksSharedAncestorsAndCycles(t *testing.T) {
	resource := traversalModel(5)
	for index, parent := range []int32{2, 0, -1, 2, 3} {
		resource.Nodes[index].Parent = parent
	}
	if err := resource.validate(); err != nil {
		t.Fatal("valid out-of-order hierarchy rejected:", err)
	}
	for _, test := range []struct {
		name   string
		index  int
		parent int32
	}{{"self_cycle", 0, 0}, {"ancestor_cycle", 2, 1}, {"out_of_range", 4, 99}, {"invalid_negative", 2, -2}} {
		t.Run(test.name, func(t *testing.T) {
			invalid := resource
			invalid.Nodes = slices.Clone(resource.Nodes)
			invalid.Nodes[test.index].Parent = test.parent
			if err := invalid.validate(); err == nil {
				t.Fatal("invalid hierarchy accepted")
			}
		})
	}
	resource.Nodes[2].World[0] = float32(math.NaN())
	if err := resource.validate(); err == nil {
		t.Fatal("previously visited ancestor skipped transform validation")
	}
}

func traversalScene(count int) *SceneInstance {
	s := &SceneInstance{
		World: ecs.NewWorld(),
		project: &Bundle{components: func(_ *Bundle, node sceneSpawnNode) (ecs.Bundle, error) {
			bundle := ecs.NewBundle(1)
			if node.Spec.Widget != nil {
				ecs.Put(&bundle, *node.Spec.Widget)
			}
			return bundle, nil
		}},
		nodes: make(map[ecs.Entity]sceneSpawnNode), Entities: make(map[string]ecs.Entity), EntityIDs: make(map[ecs.Entity]string),
	}
	spawn := ecs.NewSpawner1[scene.Parent](s.World)
	for index := range count + 1 {
		id, parent := "host", ecs.Entity(0)
		if index < count {
			id = fmt.Sprintf("node%d", index)
			if index > 0 {
				parent = s.Entities[fmt.Sprintf("node%d", index-1)]
			}
		}
		entity := spawn.Spawn(scene.Parent{Entity: parent})
		s.Entities[id], s.EntityIDs[entity] = entity, id
		s.nodes[entity] = sceneSpawnNode{Spec: SceneNodeSpec{ID: id}}
	}
	s.Roots = []ecs.Entity{s.Entities["node0"], s.Entities["host"]}
	return s
}

func TestCloneHierarchySnapshotAndWidgetTargetsStayIndependent(t *testing.T) {
	s := traversalScene(3)
	source := s.Entities["node0"]
	node := s.nodes[source]
	node.Spec.Widget = &ui.Widget{TargetID: "node1"}
	s.nodes[source] = node
	for _, prefix := range []string{"first", "second", "self"} {
		parent := s.Entities["host"]
		if prefix == "self" {
			parent = source
		}
		clones, err := s.CloneNodeUnder(parent, "node0", prefix)
		if err != nil {
			t.Fatal(err)
		}
		if len(clones) != 3 {
			t.Fatalf("clone included nodes outside the source snapshot: %v", clones)
		}
		for index := range 3 {
			entity := clones[fmt.Sprintf("%s-node%d", prefix, index)]
			want := parent
			if index > 0 {
				want = clones[fmt.Sprintf("%s-node%d", prefix, index-1)]
			}
			if relation := ecs.GetRef[scene.Parent](s.World, entity); relation == nil || relation.Entity != want {
				t.Fatalf("clone lost parent relation: %v, want %d", relation, want)
			}
		}
		widget := ecs.GetRef[ui.Widget](s.World, clones[prefix+"-node0"])
		if widget.Target != clones[prefix+"-node1"] || s.nodes[source].Spec.Widget.TargetID != "node1" {
			t.Fatal("clone modified its source widget or resolved another clone's target")
		}
	}
}

func TestRemoveNodesCollectsOverlappingRootsAndCyclesOnce(t *testing.T) {
	for _, cycle := range []bool{false, true} {
		s := traversalScene(5)
		if cycle {
			if err := ecs.Insert(s.World, s.Entities["node0"], scene.Parent{Entity: s.Entities["node4"]}); err != nil {
				t.Fatal(err)
			}
		}
		if removed := s.RemoveNodes("node4", "node2", "node0", "node2"); removed != 5 || s.World.EntityCount() != 1 || !s.World.IsAlive(s.Entities["host"]) {
			t.Fatalf("cycle=%v removal count=%d entities=%d", cycle, removed, s.World.EntityCount())
		}
		if s.RemoveNodes("node0") != 0 || len(s.nodes) != 1 || len(s.EntityIDs) != 1 || !slices.Equal(s.Roots, []ecs.Entity{s.Entities["host"]}) {
			t.Fatal("removal left stale scene indices")
		}
	}
}

func BenchmarkModelHierarchyTraversal(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			resource := traversalModel(count)
			b.ReportAllocs()
			for b.Loop() {
				if err := resource.validate(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkSceneCloneTraversal(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			s := traversalScene(count)
			parent := s.Entities["host"]
			run := func() {
				clones, err := s.CloneNodeUnder(parent, "node0", "copy")
				if err != nil || len(clones) != count {
					b.Fatalf("clone: %d nodes, error %v", len(clones), err)
				}
				if s.removeNodes(false, "copy-node0") != count {
					b.Fatal("clone cleanup missed nodes")
				}
			}
			run()
			b.ReportAllocs()
			for b.Loop() {
				run()
			}
		})
	}
}
