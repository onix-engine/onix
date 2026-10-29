package scene

import (
	"testing"

	"github.com/diiyw/onix/ecs"
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
