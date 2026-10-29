package project

import (
	"testing"
	"testing/fstest"

	"github.com/diiyw/onix/ecs"
	sceneapi "github.com/diiyw/onix/scene"
	engineui "github.com/diiyw/onix/ui"
)

func preparedSceneTestSource(t *testing.T, nodes []SceneNodeSpec) fstest.MapFS {
	t.Helper()
	plan, err := compileSceneSpawnPlan(nodes)
	if err != nil {
		t.Fatal(err)
	}
	return fstest.MapFS{
		"resources/project.json": preparedSpawnTestFile(t, gameManifest{}),
		"resources/scenes.json": preparedSpawnTestFile(t, sceneCatalog{
			Entry: "main", Scenes: []SceneCatalogEntry{{ID: "main", Path: "scenes/main.scene.json"}},
		}),
		"resources/scenes/main.scene.json": preparedSpawnTestFile(t, sceneManifest{Prepared: true, SpawnPlan: plan}),
	}
}

func TestRender2DProfilesRefreshHierarchyAndDispose(t *testing.T) {
	t.Run("render", testRender2DProfileRuntime[Render2DProfile])
	t.Run("animations", testRender2DProfileRuntime[Render2DAnimationsProfile])
}

func testRender2DProfileRuntime[P LoadProfile](t *testing.T) {
	source := preparedSceneTestSource(t, []SceneNodeSpec{
		{ID: "world", X: 3, Y: 4},
		{ID: "canvas", CoordinateSpace: CoordinateSpaceUI, X: 10, Y: 20, Children: []SceneNodeSpec{
			{ID: "child", CoordinateSpace: CoordinateSpaceUI, X: 4, Y: 6},
		}},
	})
	var profile P
	bundle, err := loadProfileBundle(source, ".", profile, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	world := scene.MustEntity("world")
	canvas := scene.MustEntity("canvas")
	child := scene.MustEntity("child")
	beforeWorld, _ := scene.World.Get[sceneapi.GlobalTransform](world)
	beforeUI, _ := scene.World.Get[sceneapi.UIGlobalTransform](child)
	transform := scene.World.GetMut[sceneapi.Transform](world)
	transform.Translation[1] += 7
	uiTransform := scene.World.GetMut[sceneapi.UITransform](canvas)
	uiTransform.Translation.X.Value += 5
	if err := scene.RefreshTransforms(); err != nil {
		t.Fatal(err)
	}
	afterWorld, _ := scene.World.Get[sceneapi.GlobalTransform](world)
	afterUI, _ := scene.World.Get[sceneapi.UIGlobalTransform](child)
	if afterWorld.Matrix[13] != beforeWorld.Matrix[13]+7 || afterUI.Matrix[12] != beforeUI.Matrix[12]+5 {
		t.Fatalf("hierarchy refresh world=%v UI=%v", afterWorld.Matrix, afterUI.Matrix)
	}
	scene.Dispose()
	if scene.World.EntityCount() != 0 {
		t.Fatalf("dispose left %d entities", scene.World.EntityCount())
	}
}

func TestPreparedScenePreservesWidgetLayoutAndControlCleanup(t *testing.T) {
	source := preparedSceneTestSource(t, []SceneNodeSpec{
		{ID: "canvas", CoordinateSpace: CoordinateSpaceUI, Width: 100, Height: 50, Anchor: sceneapi.Anchor{-0.5, 0.5}},
		{ID: "panel", CoordinateSpace: CoordinateSpaceUI, Width: 10, Height: 10, Anchor: sceneapi.Anchor{-0.5, 0.5},
			Widget: &engineui.Widget{Enabled: true, TargetID: "canvas", AlignRight: true, Right: sceneapi.Px(4)}},
	})
	bundle, err := loadTestProject(source, ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	defer scene.Dispose()
	canvas, panel := scene.MustEntity("canvas"), scene.MustEntity("panel")
	widget, ok := scene.World.Get[engineui.Widget](panel)
	transform, _ := scene.World.Get[sceneapi.UITransform](panel)
	if !ok || widget.Target != canvas || transform.Translation.X.Value != 86 {
		t.Fatalf("prepared widget=%+v transform=%+v", widget, transform)
	}
	if err := engineui.BindRelation[engineui.Control](scene.World, panel, canvas); err != nil {
		t.Fatal(err)
	}
	if err := engineui.BindVariant(scene.World, canvas, engineui.Variant[engineui.ControlVariant]{Entity: panel, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if removed := scene.RemoveNodes("panel"); removed != 1 {
		t.Fatalf("removed %d nodes", removed)
	}
	variants, _ := scene.World.Get[engineui.Variants[engineui.ControlVariant]](canvas)
	for _, variant := range variants.Items {
		if variant.Entity == panel {
			t.Fatal("removed control part remains bound to its surviving owner")
		}
	}
}
