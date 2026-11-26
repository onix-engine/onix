package scene

import (
	"testing"

	"github.com/onix-engine/onix/ecs"
)

func TestTransformCachesStoreOnlyParticipatingEntities(t *testing.T) {
	w := ecs.NewWorld()
	spawn := w.NewSpawner1[int]()
	spawn.Reserve(100000)
	for range 100000 {
		spawn.Spawn(0)
	}
	uiSpawn := w.NewSpawner1[UITransform]()
	u := uiSpawn.Spawn(NewUITransform())
	spatialSpawn := w.NewSpawner1[Transform]()
	e := spatialSpawn.Spawn(NewTransform())
	if err := SyncGlobalTransformSystem("").Run(w); err != nil {
		t.Fatal(err)
	}
	if err := SyncUIGlobalTransformSystem("").Run(w); err != nil {
		t.Fatal(err)
	}
	spatial := w.FrameValue[transformWorkspaceTag, transformWorkspace]()
	ui := w.FrameValue[uiTransformWorkspaceScratch, uiTransformWorkspace]()
	if spatial.nodes.Len() != 1 || ui.nodes.Len() != 1 {
		t.Fatal("cache retained unrelated entities")
	}
	w.Despawn(u)
	w.Despawn(e)
	if err := SyncGlobalTransformSystem("").Run(w); err != nil {
		t.Fatal(err)
	}
	if err := SyncUIGlobalTransformSystem("").Run(w); err != nil {
		t.Fatal(err)
	}
	if spatial.nodes.Len() != 0 || ui.nodes.Len() != 0 {
		t.Fatal("cache retained retired records")
	}
}

func checkGlobalX(t *testing.T, w *ecs.World, entity ecs.Entity, want float32) {
	t.Helper()
	if err := SyncGlobalTransformSystem("").Run(w); err != nil {
		t.Fatal(err)
	}
	global, ok := w.Get[GlobalTransform](entity)
	if !ok || global.Matrix[12] != want {
		t.Fatalf("entity %v: global x=%v present=%t, want %v", entity, global.Matrix[12], ok, want)
	}
}

func TestTransformLeafReplacementAndComponentChanges(t *testing.T) {
	w := ecs.NewWorld()
	spawn := w.NewSpawner2[Transform, Parent]()
	parent := spawn.Spawn(TransformFromXYZ(10, 0, 0), Parent{})
	leaf := spawn.Spawn(NewTransform(), Parent{parent})
	for range 64 {
		spawn.Spawn(NewTransform(), Parent{})
	}
	retired := leaf
	if err := (ecs.SystemConfig{System: func(c *ecs.SystemContext) error {
		if err := syncGlobalTransforms(c); err != nil {
			return err
		}
		w.Despawn(leaf)
		leaf = spawn.Spawn(TransformFromXYZ(2, 0, 0), Parent{parent})
		return nil
	}}).Run(w); err != nil {
		t.Fatal(err)
	}
	checkGlobalX(t, w, leaf, 12)
	cache := w.ScopeValue[transformWorkspaceTag, transformWorkspace]()
	if cache.recomputed != 1 || cache.node(retired) != nil {
		t.Fatalf("leaf replacement recomputed %d matrices, retired handle cached=%t", cache.recomputed, cache.node(retired) != nil)
	}
	w.Remove[Parent](leaf)
	checkGlobalX(t, w, leaf, 2)
	if err := w.Insert(leaf, Parent{parent}); err != nil {
		t.Fatal(err)
	}
	checkGlobalX(t, w, leaf, 12)
	w.Remove[Transform](parent)
	checkGlobalX(t, w, leaf, 2)
	if err := w.Insert(parent, TransformFromXYZ(20, 0, 0)); err != nil {
		t.Fatal(err)
	}
	checkGlobalX(t, w, leaf, 22)
	w.Remove[GlobalTransform](leaf)
	checkGlobalX(t, w, leaf, 22)

	// The despawn falls outside the entity journal before the next sync.
	w.Despawn(parent)
	unrelated := w.NewSpawner1[int]()
	for range 256 {
		unrelated.Spawn(0)
	}
	checkGlobalX(t, w, leaf, 2)
	if cache.node(parent) != nil {
		t.Fatal("expired journal left a retired transform in the cache")
	}
}

func TestTransformScopeIncludesNewlyLiveParent(t *testing.T) {
	w := ecs.NewWorld()
	spawn := w.NewSpawner2[Transform, Parent]()
	outside := spawn.Spawn(TransformFromXYZ(100, 0, 0), Parent{})
	bundle := ecs.NewBundle(2)
	bundle.Put(TransformFromXYZ(10, 0, 0))
	bundle.Put(Parent{outside})
	parent := w.Commands().Spawn(bundle)
	leaf := spawn.Spawn(TransformFromXYZ(2, 0, 0), Parent{parent})
	var scope ecs.QueryScope
	scope.Reset([]ecs.Entity{leaf, parent})
	w.SetQueryScope(&scope)
	checkGlobalX(t, w, leaf, 2) // The system flushes the reserved parent after syncing.
	checkGlobalX(t, w, leaf, 12)
	spawn.Spawn(NewTransform(), Parent{})
	checkGlobalX(t, w, leaf, 12)
	if cache := w.ScopeValue[transformWorkspaceTag, transformWorkspace](); cache.nodes.Len() != 2 || cache.recomputed != 0 {
		t.Fatal("an out-of-scope spawn changed the scoped transform cache")
	}
	scope.Reset([]ecs.Entity{leaf})
	checkGlobalX(t, w, leaf, 2)
	w.SetQueryScope(nil)
	checkGlobalX(t, w, leaf, 112)
	w.SetQueryScope(&scope)
	checkGlobalX(t, w, leaf, 2)
}

func BenchmarkTransformReplaceParentedLeaf(b *testing.B) {
	w := ecs.NewWorld()
	spawn := w.NewSpawner3[Transform, Parent, GlobalTransform]()
	local := NewTransform()
	spawn.Reserve(10000)
	var parent ecs.Entity
	for range 9999 {
		parent = spawn.Spawn(local, Parent{}, GlobalTransform{})
	}
	leaf := spawn.Spawn(local, Parent{parent}, GlobalTransform{})
	sync := SyncGlobalTransformSystem("")
	if err := sync.Run(w); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		w.Despawn(leaf)
		leaf = spawn.Spawn(local, Parent{parent}, GlobalTransform{})
		if err := sync.Run(w); err != nil {
			b.Fatal(err)
		}
	}
}
