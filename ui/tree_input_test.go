package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/scene"
)

func TestTreeInputRoutesMixedTypesAndHonorsConsumption(t *testing.T) {
	world := newUIWorld(t)
	stringTree := SpawnTree(world, TreeOptions[string]{Width: 200, Nodes: []TreeNodeSpec[string]{{ID: "root", Label: "Root"}}})
	intTree := SpawnTree(world, TreeOptions[int]{Width: 200, Nodes: []TreeNodeSpec[int]{{ID: 7, Label: "Seven", Leaf: true}}})
	var stringNode, intNode ecs.Entity
	for entity, _ := range world.Query[TreeNode[string]]().All {
		stringNode = entity
		break
	}
	for entity, _ := range world.Query[TreeNode[int]]().All {
		intNode = entity
		break
	}
	var strings ecs.MessageReader[TreeEvent[string]]
	var ints ecs.MessageReader[TreeEvent[int]]
	for _, event := range []scene.InputEvent{
		{Entity: stringNode, Kind: scene.InputEventMove},
		{Entity: stringNode, Kind: scene.InputEventTap, Handled: true},
		{Entity: intNode, Kind: scene.InputEventTap},
		{Entity: stringNode, Kind: scene.InputEventTap},
	} {
		world.WriteMessage(event)
	}
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if got := strings.Read(world); len(got) != 1 || got[0].Tree != stringTree || got[0].Node != stringNode || got[0].ID != "root" {
		t.Fatalf("string tree events = %+v", got)
	}
	if got := ints.Read(world); len(got) != 1 || got[0].Tree != intTree || got[0].Node != intNode || got[0].ID != 7 {
		t.Fatalf("int tree events = %+v", got)
	}
	branch, _ := world.Get[TreeNode[string]](stringNode)
	leaf, _ := world.Get[TreeNode[int]](intNode)
	selected, _ := world.Get[Tree[int]](intTree)
	if !branch.Expanded || leaf.Expanded || !selected.HasSelection || selected.Selected != 7 {
		t.Fatalf("unexpected branch, leaf, selection: %+v, %+v, %+v", branch, leaf, selected)
	}
	// Re-running the system must not deliver the same click again.
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if len(strings.Read(world)) != 0 || len(ints.Read(world)) != 0 {
		t.Fatal("tree click delivered twice")
	}
	branchMut := world.GetMut[TreeNode[string]](stringNode)
	branchMut.Disabled = true
	world.Despawn(intNode)
	world.WriteMessage(scene.InputEvent{Entity: stringNode, Kind: scene.InputEventTap})
	world.WriteMessage(scene.InputEvent{Entity: intNode, Kind: scene.InputEventTap})
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	branch, _ = world.Get[TreeNode[string]](stringNode)
	if !branch.Expanded || len(strings.Read(world)) != 0 || len(ints.Read(world)) != 0 {
		t.Fatal("disabled or despawned node accepted a click")
	}
}

func TestTreeInputHasNoSteadyStateAllocations(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[Widgets](world)
	world.InsertResource(input.NewState())
	trees := world.NewSpawner1[Tree[int]]()
	nodes := world.NewSpawner3[TreeNode[int], treeActivation, Control]()
	tree := trees.Spawn(Tree[int]{})
	node := nodes.Spawn(
		TreeNode[int]{Tree: tree, ID: 1}, treeActivation{activate: activateTreeNode[int]}, Control{},
	)
	var events ecs.MessageReader[TreeEvent[int]]
	delivered := 0
	frame := func() {
		world.WriteMessage(scene.InputEvent{Entity: node, Kind: scene.InputEventTap})
		if err := UpdateSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		for range events.All(world) {
			delivered++
		}
		world.AdvanceMessages()
	}
	frame()
	frame()
	if allocations := testing.AllocsPerRun(100, frame); allocations != 0 {
		t.Fatalf("tree input allocations = %v, want 0", allocations)
	}
	if delivered != 103 {
		t.Fatalf("delivered clicks = %d, want 103", delivered)
	}
}
