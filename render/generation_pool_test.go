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
