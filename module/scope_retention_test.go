package module

import (
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/internal/reuse"
)

func TestScopeListsShareAndReleaseOverlayStorage(t *testing.T) {
	plan, registry, _ := managerPlan(t)
	r := newFixture(t, plan, registry, Options{})
	_, err := r.Manager().StartEntry()
	must(t, err)
	shared := func() {
		t.Helper()
		r.SyncScopes()
		if r.RenderScope() != &r.runningScope || r.renderEntities != nil || r.renderDistinct {
			t.Fatal("ordinary scene retained a duplicate render list")
		}
	}
	shared()
	if cap(r.despawnEntities) != 0 {
		t.Fatal("runtime eagerly reserved a world-sized destruction queue")
	}
	for range 2 {
		must(t, r.Manager().Switch(SceneSwitchRequest{Scene: "game.json", Overlay: true}))
		must(t, r.Dispatch())
		if r.RenderScope() != &r.renderScope || !r.renderDistinct || len(r.renderEntities) <= len(r.runningEntities) {
			t.Fatal("overlay did not rebuild its distinct render membership")
		}
		must(t, r.Manager().Switch(SceneSwitchRequest{Scene: "main.json"}))
		must(t, r.Dispatch())
		shared()
	}
	must(t, r.Close())
	if r.world.QueryScope() != nil || r.runningEntities != nil || r.renderEntities != nil || r.scopeNodes != nil || r.despawnEntities != nil {
		t.Fatal("runtime close retained scope storage")
	}
}

func TestScopeListsReclaimPeaksAndRebindMembership(t *testing.T) {
	plan, _ := compileFixture(t, map[string]string{"resources/scenes/main.json": `{"modules":[]}`})
	r := newFixture(t, plan, Registry{}, Options{})
	_, err := r.Manager().StartEntry()
	must(t, err)
	spawn := r.world.NewSpawner1[int]()
	entities := make([]ecs.Entity, 10000)
	for i := range entities {
		entities[i] = spawn.Spawn(i)
	}
	r.SyncScopes()
	for _, e := range entities[100:] {
		r.world.Despawn(e)
	}
	for range reuse.RetentionFrames*2 + 4 {
		r.SyncScopes()
		r.world.Maintain()
	}
	if cap(r.runningEntities) > 204 || cap(r.scopeNodes) > 204 || cap(r.renderEntities) != 0 || cap(r.despawnEntities) != 0 {
		t.Fatalf("module retained peak lists: running=%d render=%d nodes=%d despawn=%d", cap(r.runningEntities), cap(r.renderEntities), cap(r.scopeNodes), cap(r.despawnEntities))
	}
	count := 0
	for _, value := range r.world.Query[int]().All {
		if value.Get() >= 100 {
			t.Fatal("resized scope borrowed stale entity storage")
		}
		count++
	}
	if count != 100 {
		t.Fatalf("resized scope has %d members", count)
	}
	if n := testing.AllocsPerRun(100, r.SyncScopes); n != 0 {
		t.Fatalf("stable scope maintenance allocated %g", n)
	}
	t.Logf("10000 -> 100 entities: running cap=%d nodes cap=%d; render/despawn cap=0", cap(r.runningEntities), cap(r.scopeNodes))
}
