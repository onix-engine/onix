package ecs

import "testing"

func TestInQueryScopeTracksMembershipAndEntityMoves(t *testing.T) {
	world := NewWorld()
	spawn := world.NewSpawner1[scopeNumber]()
	first, second := spawn.Spawn(1), spawn.Spawn(2)
	var scope QueryScope
	scope.Reset([]Entity{first})
	world.SetQueryScope(&scope)
	check := func() {
		if !world.InQueryScope(first) || world.InQueryScope(second) || world.InQueryScope(0) {
			panic("scope membership changed")
		}
	}
	check()
	if err := world.Insert(first, struct{ Extra bool }{true}); err != nil {
		t.Fatal(err)
	}
	check()
	if allocs := testing.AllocsPerRun(100, check); allocs != 0 {
		t.Fatalf("membership check allocated %g times", allocs)
	}
	world.Despawn(first)
	replacement := spawn.Spawn(3)
	if world.InQueryScope(first) || world.InQueryScope(replacement) {
		t.Fatal("retired entity membership was inherited by its replacement")
	}
	scope.Reset([]Entity{replacement})
	if !world.InQueryScope(replacement) || world.InQueryScope(second) {
		t.Fatal("scope reset did not update membership")
	}
	world.SetQueryScope(nil)
	if !world.InQueryScope(second) || world.InQueryScope(first) {
		t.Fatal("unrestricted membership ignored entity lifetime")
	}
}
