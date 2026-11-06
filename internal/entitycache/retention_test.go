package entitycache

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/internal/reuse"
)

func TestTrimPruneAndGenerationAfterPeak(t *testing.T) {
	w := ecs.NewWorld()
	spawn := w.NewSpawner1[int]()
	entities := make([]ecs.Entity, 4096)
	var store Store[int]
	for i := range entities {
		entities[i] = spawn.Spawn(i)
		*store.Ensure(entities[i]) = i
	}
	store.Prune(func(e ecs.Entity, value *int) bool { return *value < 32 })
	for range reuse.RetentionFrames + 1 {
		store.Trim()
	}
	if store.Len() != 32 || cap(store.entries) > 64 || cap(store.indices) > 64 {
		t.Fatalf("peak retained: entries %d/%d, index %d/%d", store.Len(), cap(store.entries), len(store.indices), cap(store.indices))
	}
	count := 0
	for e, value := range store.All {
		if *value != *w.GetRef[int](e) || store.Get(e) != value {
			t.Fatal("prune/trim corrupted the dense index")
		}
		count++
	}
	if count != 32 {
		t.Fatal("dense traversal lost records")
	}
	// Regrow a trimmed index, then recycle the slot with a new generation.
	old := entities[len(entities)-1]
	*store.Ensure(old) = 99
	w.Despawn(old)
	replacement := spawn.Spawn(7)
	if *store.Ensure(replacement) != 0 || store.Get(old) != nil {
		t.Fatal("regrown index inherited a retired generation")
	}
	store.Delete(replacement)
	for range reuse.RetentionFrames + 1 {
		store.Trim()
	}
	if n := testing.AllocsPerRun(100, func() {
		store.Prune(func(_ ecs.Entity, _ *int) bool { return true })
		store.Trim()
	}); n != 0 {
		t.Fatalf("stable prune/trim allocated %g", n)
	}
}
