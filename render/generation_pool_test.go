package render

import (
	"errors"
	"testing"
)

type poolTestResource struct{}

func TestDestroyNativeResourceKeepsHandleUntilSuccess(t *testing.T) {
	var pool GenerationPool[TextureResource, uint32]
	handle := pool.Insert(23)
	failure := errors.New("destroy failed")
	if err := destroyNativeResource(&pool, handle, "texture", func(uint32) error { return failure }); err != failure {
		t.Fatal("native destruction error was lost", err)
	}
	if native, ok := pool.Get(handle); !ok || native != 23 {
		t.Fatal("failed destruction removed the live resource")
	}
	if err := destroyNativeResource(&pool, handle, "texture", func(native uint32) error {
		if native != 23 {
			t.Fatal("destruction received the wrong native resource", native)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := pool.Get(handle); ok {
		t.Fatal("successful destruction retained the handle")
	}
	if err := destroyNativeResource(&pool, handle, "texture", func(uint32) error {
		t.Fatal("stale handle reached the native destroy callback")
		return nil
	}); err == nil {
		t.Fatal("stale handle was accepted")
	}
}

func TestGenerationPoolReusesSlotsAndRejectsStaleHandles(t *testing.T) {
	var pool GenerationPool[poolTestResource, string]
	first := pool.Insert("first")
	if value, ok := pool.Remove(first); !ok || value != "first" {
		t.Fatalf("remove = %q, %v", value, ok)
	}
	second := pool.Insert("second")
	if first.ID() != second.ID() || first.Generation() == second.Generation() {
		t.Fatalf("slot was not reused with a new generation: first=%s second=%s", first, second)
	}
	if _, ok := pool.Get(first); ok {
		t.Fatal("stale handle resolved after slot reuse")
	}
	if value, ok := pool.Get(second); !ok || value != "second" {
		t.Fatalf("get = %q, %v", value, ok)
	}
}

func TestGenerationPoolRefKeepsHandleAndRejectsStaleLookups(t *testing.T) {
	var pool GenerationPool[MeshResource, nativeMesh]
	first := pool.Insert(nativeMesh{native: 11})
	value := pool.getRef(first)
	value.deformation = meshDeformationLayout{skinOffset: 1, morphOffset: 9, vertexCount: 32, targetCount: 4, requiredJoints: 64}
	if pool.Len() != 1 {
		t.Fatal("updating a live slot changed its residency")
	}
	if got, ok := pool.Get(first); !ok || got != *value {
		t.Fatalf("updated metadata was not persisted: %+v, %v", got, ok)
	}
	want := *value
	if got, ok := pool.Remove(first); !ok || got != want {
		t.Fatalf("removal returned old metadata: %+v, %v", got, ok)
	}
	if pool.getRef(first) != nil {
		t.Fatal("borrowed a removed slot")
	}
	secondValue := nativeMesh{native: 22}
	second := pool.Insert(secondValue)
	for _, invalid := range []MeshHandle{{}, first, NewHandle[MeshResource](2, 1)} {
		if pool.getRef(invalid) != nil {
			t.Fatalf("borrowed slot using invalid handle %s", invalid)
		}
	}
	if got, ok := pool.Get(second); !ok || got != secondValue {
		t.Fatalf("stale update changed the reused slot: %+v, %v", got, ok)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		*pool.getRef(second) = secondValue
	}); allocations != 0 {
		t.Fatalf("borrowing metadata allocated %g times", allocations)
	}
	pool.Clear()
	if pool.getRef(second) != nil {
		t.Fatal("borrowed from a cleared pool")
	}
}

func TestGenerationPoolFirstReleaseDoesNotAllocate(t *testing.T) {
	// Each measurement removes a fresh pool's resources, so a warmed free list
	// cannot conceal allocation on the first scene unload.
	var pools [101]GenerationPool[poolTestResource, uint32]
	for i := range pools {
		for value := range uint32(128) {
			pools[i].Insert(value)
		}
	}
	index := 0
	allocations := testing.AllocsPerRun(100, func() {
		pool := &pools[index]
		index++
		for value := range uint32(128) {
			got, ok := pool.Remove(NewHandle[poolTestResource](value+1, 1))
			if !ok || got != value {
				t.Fatal("fresh resource was not removed")
			}
		}
	})
	if allocations != 0 {
		t.Fatalf("first scene unload allocated %g times", allocations)
	}
	for i := range pools {
		if pools[i].Len() != 0 || pools[i].freeCount != 128 {
			t.Fatal("free-list count is incorrect")
		}
	}
}

func TestGenerationPoolEmbeddedFreeListPreservesHandlesAndReferences(t *testing.T) {
	var pool GenerationPool[poolTestResource, *int]
	values := make([]int, 257)
	handles := make([]Handle[poolTestResource], len(values))
	for i := range handles {
		values[i] = i
		handles[i] = pool.Insert(&values[i])
	}
	// Remove a nonsequential subset, then reuse in LIFO order without growing.
	removed := []int{256, 0, 128, 7, 200, 1}
	for _, i := range removed {
		value, ok := pool.Remove(handles[i])
		if !ok || value != &values[i] || pool.slots[i].value != nil {
			t.Fatal("removed slot retained or changed the value")
		}
		if _, ok := pool.Remove(handles[i]); ok {
			t.Fatal("duplicate removal changed the free list")
		}
		future := NewHandle[poolTestResource](handles[i].ID(), handles[i].Generation()+1)
		if _, ok := pool.Get(future); ok {
			t.Fatal("unoccupied slot accepted a future-generation handle")
		}
	}
	visited := 0
	pool.Range(func(value *int) bool {
		if value == nil {
			t.Fatal("Range visited an empty slot")
		}
		visited++
		return true
	})
	if visited != len(values)-len(removed) || visited != pool.Len() {
		t.Fatal("Range and live count disagree")
	}
	for i := len(removed) - 1; i >= 0; i-- {
		old := handles[removed[i]]
		handle := pool.Insert(&values[removed[i]])
		if handle.ID() != old.ID() || handle.Generation() != old.Generation()+1 {
			t.Fatal("free-list reuse changed ID or generation")
		}
		if pool.getRef(old) != nil {
			t.Fatal("stale handle reached reused storage")
		}
	}
	if pool.Len() != len(values) || len(pool.slots) != len(values) || pool.free != 0 {
		t.Fatal("reuse grew slots or lost a free-list node")
	}
	// Generation overflow must still skip the reserved zero generation.
	pool.slots[0].generation = ^uint32(0)
	old := NewHandle[poolTestResource](1, ^uint32(0))
	pool.Remove(old)
	if next := pool.Insert(&values[0]); next.Generation() != 1 {
		t.Fatal("wrapped generation was not reset")
	}
	pool.Clear()
	if pool.Len() != 0 || pool.slots != nil || pool.free != 0 || pool.freeCount != 0 {
		t.Fatal("Clear retained pool state")
	}
}
