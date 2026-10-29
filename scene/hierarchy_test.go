package scene

import (
	"fmt"
	"testing"

	"github.com/diiyw/onix/ecs"
)

func hierarchyFixture(count int) (*ecs.World, []ecs.Entity) {
	w := ecs.NewWorld()
	s := w.NewSpawner3[Parent, Activity, Visibility]()
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
	if err := w.Insert(entities[0], "unrelated"); err != nil {
		t.Fatal(err)
	}
	if !IsInteractive(w, entities[126]) || h.Version() != version || h.invalidated != 0 || h.recomputed != 0 {
		t.Fatal("unrelated component migration invalidated hierarchy")
	}
	w.GetMut[Activity](entities[62]).Active = false
	if IsActive(w, entities[125]) || !IsVisible(w, entities[125]) || IsInteractive(w, entities[126]) {
		t.Fatal("activity was not inherited independently of visibility")
	}
	if h.invalidated != 3 || h.recomputed != 3 || h.Version() != version {
		t.Fatalf("local change touched unrelated nodes: invalidated=%d recomputed=%d", h.invalidated, h.recomputed)
	}
	w.GetMut[Parent](entities[125]).Entity = entities[61]
	if !IsInteractive(w, entities[125]) || IsInteractive(w, entities[126]) {
		t.Fatal("reparent did not update inherited state")
	}
	if h.invalidated != 1 {
		t.Fatalf("reparent invalidated %d nodes", h.invalidated)
	}
	w.Remove[Activity](entities[62])
	if !IsInteractive(w, entities[126]) {
		t.Fatal("removing local state did not restore inherited defaults")
	}
	for range 3 {
		w.GetMut[Visibility](entities[62]).Visible = !w.GetRef[Visibility](entities[62]).Visible
		IsInteractive(w, entities[126])
	}
	if allocs := testing.AllocsPerRun(30, func() {
		w.GetMut[Visibility](entities[62]).Visible = !w.GetRef[Visibility](entities[62]).Visible
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
	w.GetMut[Visibility](entities[0]).Visible = false
	for e := range w.Query[Activity]().All {
		if IsInteractive(w, e) || !IsActive(w, e) {
			t.Fatal("scoped lookup lost an ancestor outside the scope")
		}
	}
	w.GetMut[Parent](entities[0]).Entity = entities[3]
	if IsActive(w, entities[3]) {
		t.Fatal("cycle was treated as an active hierarchy")
	}
	w.Remove[Parent](entities[0])
	if !IsActive(w, entities[3]) {
		t.Fatal("cycle repair did not invalidate descendants")
	}
	w.Despawn(entities[0])
	s := w.NewSpawner2[Activity, Visibility]()
	replacement := s.Spawn(Activity{false}, Visibility{false})
	if replacement == entities[0] || !IsInteractive(w, entities[3]) || IsInteractive(w, replacement) {
		t.Fatal("reused slot inherited stale parent state")
	}
}

func TestHierarchySameTickWrites(t *testing.T) {
	w, entities := hierarchyFixture(7)
	system := ecs.SystemConfig{Name: "writes", System: func(c *ecs.SystemContext) error {
		for _, active := range []bool{false, true, false} {
			c.World.GetMut[Activity](entities[0]).Active = active
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
	spawner := w.NewSpawner4[Transform, UITransform, Parent, Size2D]()
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
	if err := w.Insert(root, "unrelated"); err != nil {
		t.Fatal(err)
	}
	run()
	if w.FrameValue[transformWorkspaceTag, transformWorkspace]().recomputed != 0 || w.FrameValue[uiTransformWorkspaceScratch, uiTransformWorkspace]().recomputed != 0 {
		t.Fatal("component migration recomputed copied transform values")
	}
	// Shared child links must still propagate changes after the rows move.
	w.GetMut[Transform](root).Translation[0] = 20
	w.GetMut[UITransform](root).Translation.X.Value = 20
	run()
	global, _ := w.Get[GlobalTransform](child)
	uiGlobal, _ := w.Get[UIGlobalTransform](child)
	if global.Matrix[12] != 22 || uiGlobal.Matrix[12] != 22 {
		t.Fatal("migrated parent lost transform descendants")
	}
}

func BenchmarkInheritedActivity(b *testing.B) {
	for _, depth := range []int{1, 16, 128} {
		for _, cached := range []bool{false, true} {
			b.Run(fmt.Sprintf("depth=%d/cached=%t", depth, cached), func(b *testing.B) {
				w := ecs.NewWorld()
				s := w.NewSpawner2[Parent, Activity]()
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
							if value, ok := w.Get[Activity](current); ok && !value.Active {
								b.Fatal("inactive")
							}
							parent, ok := w.Get[Parent](current)
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
