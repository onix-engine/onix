package ecs

import "testing"

func TestTypedSpawnerRecordsOneStructuralChangeAndKeepsBoundColumns(t *testing.T) {
	world := NewWorld()
	nodes := NewSpawner8[testPosition, testVelocity, testName, testSize, testVisibility, testParent, testAnchor, testStack](world)
	before := world.structureVersion
	first := nodes.Spawn(testPosition{X: 1}, testVelocity{X: 2}, testName{Value: "first"}, testSize{Width: 3}, testVisibility{Visible: true}, testParent{}, testAnchorTopLeft, 4)
	if world.structureVersion != before+1 {
		t.Fatal("typed spawn did not record one structural change")
	}
	Remove[testVelocity](world, first)
	second := nodes.Spawn(testPosition{X: 5}, testVelocity{X: 6}, testName{Value: "second"}, testSize{Width: 7}, testVisibility{Visible: true}, testParent{Entity: first}, testAnchorCenter, 8)
	if GetRef[testPosition](world, first).X != 1 || GetRef[testVelocity](world, second).X != 6 ||
		GetRef[testName](world, second).Value != "second" || GetRef[testSize](world, second).Width != 7 ||
		GetRef[testParent](world, second).Entity != first || *GetRef[testAnchor](world, second) != testAnchorCenter ||
		*GetRef[testStack](world, second) != 8 {
		t.Fatal("bound columns changed after source row migration")
	}
}
