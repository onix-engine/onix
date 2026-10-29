package entitycache

import (
	"testing"

	"github.com/diiyw/onix/ecs"
)

func TestStoreDeletionGenerationAndReuse(t *testing.T) {
	w := ecs.NewWorld()
	spawn := w.NewSpawner1[int]()
	a, b := spawn.Spawn(1), spawn.Spawn(2)
	var store Store[*int]
	value := 42
	*store.Ensure(a), *store.Ensure(b) = &value, &value
	store.Delete(a)
	if store.Get(a) != nil || store.Get(b) == nil || **store.Get(b) != 42 {
		t.Fatal("swap deletion corrupted index")
	}
	w.Despawn(b)
	c := spawn.Spawn(3)
	if store.Get(c) != nil {
		t.Fatal("recycled entity inherited an old generation")
	}
	if *store.Ensure(c) != nil || store.Get(b) != nil {
		t.Fatal("replacement retained an old value")
	}
	store.Reset()
	if store.Len() != 0 || store.Get(c) != nil {
		t.Fatal("reset retained a record")
	}
	if n := testing.AllocsPerRun(100, func() { store.Ensure(c); store.Reset() }); n != 0 {
		t.Fatalf("warm reuse allocated %g", n)
	}
}
