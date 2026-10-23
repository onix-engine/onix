package project

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/diiyw/onix/internal/sourcejson"
	"github.com/diiyw/onix/internal/typedbin"
)

func preparedSpawnTestFile[T any](t *testing.T, value T) *fstest.MapFile {
	t.Helper()
	data, err := typedbin.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return &fstest.MapFile{Data: data}
}

func TestSceneCatalogRequiresPreparedSpawnPlanInRelease(t *testing.T) {
	for _, prepared := range []bool{false, true} {
		name := "source"
		if prepared {
			name = "prepared"
		}
		t.Run(name, func(t *testing.T) {
			manifest := sceneManifest{Nodes: []SceneNodeSpec{{ID: "root"}}}
			if prepared {
				// Invalid authoring data must never be recompiled when a plan exists.
				manifest.Nodes = []SceneNodeSpec{{}}
				manifest.Prepared = true
				manifest.SpawnPlan = sceneSpawnPlan{Nodes: []sceneSpawnNode{{Spec: SceneNodeSpec{ID: "root"}}}}
			}
			source := fstest.MapFS{
				"resources/scenes.json": preparedSpawnTestFile(t, sceneCatalog{
					Entry: "main", Scenes: []SceneCatalogEntry{{ID: "main", Path: "scenes/main.scene.json"}},
				}),
				"resources/scenes/main.scene.json": preparedSpawnTestFile(t, manifest),
			}
			entry, plans, ids, err := loadSceneCatalog(source, "resources")
			if !prepared && !sourcejson.Enabled {
				if err == nil || !strings.Contains(err.Error(), `scene "main": release builds require a preprocessed spawn plan`) {
					t.Fatalf("unprepared release scene error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			plan := plans["main"]
			if entry != "main" || len(ids) != 1 || ids[0] != entry || len(plan.Nodes) != 1 || plan.Nodes[0].Spec.ID != "root" {
				t.Fatalf("loaded scene entry=%q ids=%v plan=%+v", entry, ids, plan)
			}
		})
	}
}

func TestPrefabResourcesRequirePreparedSpawnPlanInRelease(t *testing.T) {
	for _, prepared := range []bool{false, true} {
		name := "source"
		if prepared {
			name = "prepared"
		}
		t.Run(name, func(t *testing.T) {
			manifest := prefabManifest{Root: SceneNodeSpec{ID: "root"}}
			if prepared {
				manifest.Root = SceneNodeSpec{}
				manifest.Prepared = true
				manifest.SpawnPlan = sceneSpawnPlan{Nodes: []sceneSpawnNode{{Spec: SceneNodeSpec{ID: "root"}}}}
			}
			source := fstest.MapFS{"resources/prefabs/effect.prefab.json": preparedSpawnTestFile(t, manifest)}
			plans, ids, err := loadPrefabResources(source, "resources", []PrefabResourceSpec{{ID: "effect", Path: "prefabs/effect.prefab.json"}})
			if !prepared && !sourcejson.Enabled {
				if err == nil || !strings.Contains(err.Error(), `prefab "effect": release builds require a preprocessed spawn plan`) {
					t.Fatalf("unprepared release prefab error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			plan := plans["effect"]
			if len(ids) != 1 || ids[0] != "effect" || plan == nil || len(plan.plan.Nodes) != 1 || plan.plan.Nodes[0].Spec.ID != "root" {
				t.Fatalf("loaded prefab ids=%v plan=%+v", ids, plan)
			}
		})
	}
}
