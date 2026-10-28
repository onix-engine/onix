package scene

import (
	"fmt"
	"testing"

	"github.com/diiyw/onix/ecs"
)

func hierarchyFixture(count int) (*ecs.World, []ecs.Entity) {
	w := ecs.NewWorld()
	s := ecs.NewSpawner3[Parent, Activity, Visibility](w)
	s.Reserve(count)
	entities := make([]ecs.Entity, count)
	for i := range entities {
		var parent ecs.Entity
		if i != 0 {
			parent = entities[(i-1)/2]
		}
		entities[i] = s.Spawn(Parent{parent}, Activity{true}, Visibility{true})
	}
	return w, entities
}

func TestHierarchyCachesOnlyAffectedSubtrees(t *testing.T) {
	w, entities := hierarchyFixture(127)
	for _, e := range entities {
		if !IsInteractive(w, e) {
			t.Fatal("new hierarchy is not enabled")
		}
	}
	h := HierarchyFor(w)
	version := h.Version()
	if err := ecs.Insert(w, entities[0], "unrelated"); err != nil {
		t.Fatal(err)
	}
	if !IsInteractive(w, entities[126]) || h.Version() != version || h.invalidated != 0 || h.recomputed != 0 {
		t.Fatal("unrelated component migration invalidated hierarchy")
	}
	ecs.GetMut[Activity](w, entities[62]).Active = false
	if IsActive(w, entities[125]) || !IsVisible(w, entities[125]) || IsInteractive(w, entities[126]) {
		t.Fatal("activity was not inherited independently of visibility")
	}
	if h.invalidated != 3 || h.recomputed != 3 || h.Version() != version {
		t.Fatalf("local change touched unrelated nodes: invalidated=%d recomputed=%d", h.invalidated, h.recomputed)
	}
	ecs.GetMut[Parent](w, entities[125]).Entity = entities[61]
	if !IsInteractive(w, entities[125]) || IsInteractive(w, entities[126]) {
		t.Fatal("reparent did not update inherited state")
	}
	if h.invalidated != 1 {
		t.Fatalf("reparent invalidated %d nodes", h.invalidated)
	}
	ecs.Remove[Activity](w, entities[62])
	if !IsInteractive(w, entities[126]) {
		t.Fatal("removing local state did not restore inherited defaults")
	}
	for range 3 {
		ecs.GetMut[Visibility](w, entities[62]).Visible = !ecs.GetRef[Visibility](w, entities[62]).Visible
		IsInteractive(w, entities[126])
	}
	if allocs := testing.AllocsPerRun(30, func() {
		ecs.GetMut[Visibility](w, entities[62]).Visible = !ecs.GetRef[Visibility](w, entities[62]).Visible
		IsInteractive(w, entities[126])
	}); allocs != 0 {
		t.Fatalf("warm local changes allocated %v times", allocs)
	}
}

func TestHierarchyScopesCyclesAndSlotReuse(t *testing.T) {
	w, entities := hierarchyFixture(7)
	var scope ecs.QueryScope
	scope.Reset(entities[3:4])
	w.SetQueryScope(&scope)
	ecs.GetMut[Visibility](w, entities[0]).Visible = false
	for e := range ecs.Query[Activity](w).All {
		if IsInteractive(w, e) || !IsActive(w, e) {
			t.Fatal("scoped lookup lost an ancestor outside the scope")
		}
	}
	ecs.GetMut[Parent](w, entities[0]).Entity = entities[3]
	if IsActive(w, entities[3]) {
		t.Fatal("cycle was treated as an active hierarchy")
	}
	ecs.Remove[Parent](w, entities[0])
	if !IsActive(w, entities[3]) {
		t.Fatal("cycle repair did not invalidate descendants")
	}
	w.Despawn(entities[0])
	s := ecs.NewSpawner2[Activity, Visibility](w)
	replacement := s.Spawn(Activity{false}, Visibility{false})
	if replacement == entities[0] || !IsInteractive(w, entities[3]) || IsInteractive(w, replacement) {
		t.Fatal("reused slot inherited stale parent state")
	}
}

func TestHierarchySameTickWrites(t *testing.T) {
	w, entities := hierarchyFixture(7)
	system := ecs.SystemConfig{Name: "writes", System: func(c *ecs.SystemContext) error {
		for _, active := range []bool{false, true, false} {
			ecs.GetMut[Activity](c.World, entities[0]).Active = active
			if IsActive(c.World, entities[6]) != active {
				t.Fatal("missed a write in the current system tick")
			}
		}
		return nil
	}}
	if err := system.Run(w); err != nil {
		t.Fatal(err)
	}
}

func TestTransformCachesIgnoreComponentMigration(t *testing.T) {
	w := ecs.NewWorld()
	spawner := ecs.NewSpawner4[Transform, UITransform, Parent, Size2D](w)
	root := spawner.Spawn(TransformFromXYZ(10, 0, 0), UITransformFromXY(10, 0), Parent{}, Size2D{})
	child := spawner.Spawn(TransformFromXYZ(2, 0, 0), UITransformFromXY(2, 0), Parent{root}, Size2D{})
	worldSync, uiSync := SyncGlobalTransformSystem(""), SyncUIGlobalTransformSystem("")
	run := func() {
		t.Helper()
		if err := worldSync.Run(w); err != nil {
			t.Fatal(err)
		}
		if err := uiSync.Run(w); err != nil {
			t.Fatal(err)
		}
	}
	run()
	if err := ecs.Insert(w, root, "unrelated"); err != nil {
		t.Fatal(err)
	}
	run()
	if ecs.FrameValue[transformWorkspaceTag, transformWorkspace](w).recomputed != 0 || ecs.FrameValue[uiTransformWorkspaceScratch, uiTransformWorkspace](w).recomputed != 0 {
		t.Fatal("component migration recomputed copied transform values")
	}
	// Shared child links must still propagate changes after the rows move.
	ecs.GetMut[Transform](w, root).Translation[0] = 20
	ecs.GetMut[UITransform](w, root).Translation.X.Value = 20
	run()
	global, _ := ecs.Get[GlobalTransform](w, child)
	uiGlobal, _ := ecs.Get[UIGlobalTransform](w, child)
	if global.Matrix[12] != 22 || uiGlobal.Matrix[12] != 22 {
		t.Fatal("migrated parent lost transform descendants")
	}
}

func BenchmarkInheritedActivity(b *testing.B) {
	for _, depth := range []int{1, 16, 128} {
		for _, cached := range []bool{false, true} {
			b.Run(fmt.Sprintf("depth=%d/cached=%t", depth, cached), func(b *testing.B) {
				w := ecs.NewWorld()
				s := ecs.NewSpawner2[Parent, Activity](w)
				var e ecs.Entity
				for range depth {
					e = s.Spawn(Parent{e}, Activity{true})
				}
				IsActive(w, e)
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					if cached {
						if !IsActive(w, e) {
							b.Fatal("inactive")
						}
					} else {
						for current := e; current != 0; {
							if value, ok := ecs.Get[Activity](w, current); ok && !value.Active {
								b.Fatal("inactive")
							}
							parent, ok := ecs.Get[Parent](w, current)
							if !ok {
								break
							}
							current = parent.Entity
						}
					}
				}
			})
		}
	}
}
