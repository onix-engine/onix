package ecs

import "testing"

func TestTypedSpawnerRecordsOneStructuralChangeAndKeepsBoundColumns(t *testing.T) {
	world := NewWorld()
	nodes := world.NewSpawner8[testPosition, testVelocity, testName, testSize, testVisibility, testParent, testAnchor, testStack]()
	before := world.structureVersion
	first := nodes.Spawn(testPosition{X: 1}, testVelocity{X: 2}, testName{Value: "first"}, testSize{Width: 3}, testVisibility{Visible: true}, testParent{}, testAnchorTopLeft, 4)
	if world.structureVersion != before+1 {
		t.Fatal("typed spawn did not record one structural change")
	}
	world.Remove[testVelocity](first)
	second := nodes.Spawn(testPosition{X: 5}, testVelocity{X: 6}, testName{Value: "second"}, testSize{Width: 7}, testVisibility{Visible: true}, testParent{Entity: first}, testAnchorCenter, 8)
	if world.GetRef[testPosition](first).X != 1 || world.GetRef[testVelocity](second).X != 6 ||
		world.GetRef[testName](second).Value != "second" || world.GetRef[testSize](second).Width != 7 ||
		world.GetRef[testParent](second).Entity != first || *world.GetRef[testAnchor](second) != testAnchorCenter ||
		*world.GetRef[testStack](second) != 8 {
		t.Fatal("bound columns changed after source row migration")
	}
}
