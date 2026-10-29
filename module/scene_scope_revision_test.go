package module

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func TestScopeVersionsTrackMembershipInsteadOfStorage(t *testing.T) {
	plan, registry, probe := managerPlan(t)
	r := newFixture(t, plan, registry, Options{})
	_, err := r.Manager().StartEntry()
	must(t, err)
	s := r.world.NewSpawner1[scene.Parent]()
	left := s.Spawn(scene.Parent{Entity: probe.buttons[0].root})
	right := s.Spawn(scene.Parent{Entity: probe.buttons[0].root})
	child := s.Spawn(scene.Parent{Entity: left})
	r.SyncScopes()
	_, version := r.world.QueryScopeKey()
	hierarchy := r.scopeHierarchy
	must(t, r.world.Insert(child, scopeProbe{}))
	r.SyncScopes()
	if _, got := r.world.QueryScopeKey(); got != version || r.scopeHierarchy != hierarchy {
		t.Fatal("unrelated component migration reset execution caches")
	}
	r.world.GetMut[scene.Parent](child).Entity = right
	r.SyncScopes()
	if _, got := r.world.QueryScopeKey(); got != version || r.entityContext(child) != probe.buttons[0] {
		t.Fatal("same-scene reparent changed membership or lost ownership")
	}
	_, err = r.Manager().Preload(ScenePreloadRequest{Scene: "game.json"})
	must(t, err)
	_, version = r.world.QueryScopeKey()
	r.world.GetMut[scene.Parent](child).Entity = probe.buttons[1].root
	r.SyncScopes()
	if _, got := r.world.QueryScopeKey(); got == version || r.entityContext(child) != probe.buttons[1] {
		t.Fatal("cross-scene reparent did not update membership and ownership")
	}
	for range r.world.Query[scopeProbe]().All {
		t.Fatal("preloaded descendant entered execution scope")
	}
}

func BenchmarkScopeUnrelatedMigration(b *testing.B) {
	plan, _ := compileFixture(b, map[string]string{"resources/scenes/main.json": `{"modules":[]}`})
	r := newFixture(b, plan, Registry{}, Options{})
	_, err := r.Manager().StartEntry()
	must(b, err)
	s := r.world.NewSpawner1[scene.Parent]()
	var entity ecs.Entity
	for range 10000 {
		entity = s.Spawn(scene.Parent{})
	}
	step := func() {
		must(b, r.world.Insert(entity, scopeProbe{}))
		r.SyncScopes()
		r.world.Remove[scopeProbe](entity)
		r.SyncScopes()
	}
	step()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		step()
	}
}
