//go:build onix_release

package project

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/internal/typedbin"
	sceneapi "github.com/diiyw/onix/scene"
	engineui "github.com/diiyw/onix/ui"
)

func TestReleaseRuntimeRequiresPreprocessedProjectResources(t *testing.T) {
	projectData, err := typedbin.Marshal(gameManifest{})
	if err != nil {
		t.Fatal(err)
	}
	gameObjectData, err := typedbin.Marshal(sceneManifest{Prepared: true})
	if err != nil {
		t.Fatal(err)
	}
	catalogData, err := typedbin.Marshal(sceneCatalog{Entry: "main", Scenes: []SceneCatalogEntry{{ID: "main", Path: "scenes/main.scene.json"}}})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := loadTestProject(fstest.MapFS{
		"resources/project.json":           {Data: projectData},
		"resources/scenes.json":            {Data: catalogData},
		"resources/scenes/main.scene.json": {Data: gameObjectData},
	}, ".")
	if err != nil {
		t.Fatalf("load preprocessed release resources: %v", err)
	}
	if err := bundle.Dispose(); err != nil {
		t.Fatal(err)
	}

	_, err = loadTestProject(fstest.MapFS{
		"resources/project.json":           {Data: []byte(`{}`)},
		"resources/scenes.json":            {Data: catalogData},
		"resources/scenes/main.scene.json": {Data: gameObjectData},
	}, ".")
	if err == nil || !strings.Contains(err.Error(), "require preprocessed ONIXBIN1") {
		t.Fatalf("release JSON rejection error = %v", err)
	}
}

func TestReleaseRuntimeLoadsAuthoredUIComposition(t *testing.T) {
	sprite := SceneNodeSpec{
		ID: "run-normal", Components: Components(SceneComponentSprite), CoordinateSpace: CoordinateSpaceUI,
		Width: 120, Height: 40, ScaleX: 1, ScaleY: 1, ScaleZ: 1, Alpha: 1,
	}
	sprite.Components.Sprite.Texture = &TextureSpec{Color: "#2878ffff"}
	plan, err := compileSceneSpawnPlan([]SceneNodeSpec{{
		ID: "run", Control: &ControlSpec{}, CoordinateSpace: CoordinateSpaceUI,
		Width: 120, Height: 40, ScaleX: 1, ScaleY: 1, ScaleZ: 1, Alpha: 1,
		Children: []SceneNodeSpec{sprite},
	}})
	if err != nil {
		t.Fatalf("compile UI spawn plan: %v", err)
	}
	projectData, err := typedbin.Marshal(gameManifest{})
	if err != nil {
		t.Fatal(err)
	}
	gameObjectData, err := typedbin.Marshal(sceneManifest{Prepared: true, SpawnPlan: plan})
	if err != nil {
		t.Fatal(err)
	}
	catalogData, err := typedbin.Marshal(sceneCatalog{Entry: "main", Scenes: []SceneCatalogEntry{{ID: "main", Path: "scenes/main.scene.json"}}})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := loadTestProject(fstest.MapFS{
		"resources/project.json":           {Data: projectData},
		"resources/scenes.json":            {Data: catalogData},
		"resources/scenes/main.scene.json": {Data: gameObjectData},
	}, ".")
	if err != nil {
		t.Fatalf("load release UI resources: %v", err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatalf("instantiate release UI scene: %v", err)
	}
	defer scene.Dispose()
	if err := BindControl(scene, "run", engineui.Button{}); err != nil {
		t.Fatalf("bind release UI controls: %v", err)
	}

	root := scene.MustEntity("run")
	child := scene.MustEntity("run-normal")
	element, ok := ecs.Get[engineui.Relation[engineui.Control]](scene.World, child)
	if !ok || element.Owner != root || !ecs.Has[engineui.Part[engineui.BodyPart]](scene.World, child) {
		t.Fatalf("release UI element = %#v", element)
	}
	variants, ok := ecs.Get[engineui.Variants[engineui.ControlVariant]](scene.World, root)
	if !ok || !hasControlVariant(variants, child, engineui.VariantAlways) {
		t.Fatalf("release UI variants = %#v", variants)
	}
	visibility, _ := ecs.Get[sceneapi.Visibility](scene.World, child)
	if !visibility.Visible {
		t.Fatal("release UI body is hidden")
	}
}
