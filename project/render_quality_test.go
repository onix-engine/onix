//go:build !onix_release

package project

import (
	"os"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/internal/typedbin"
	"github.com/diiyw/onix/scene/render3d"
)

func TestCameraQualityAndFXAASceneBinaryAndLiveEdits(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[{"id":"camera","components":{"camera":{"quality":"medium","fxaa":true}}}]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bundle.Dispose() })
	scene, err := bundle.InstantiateEntryScene(ecs.NewWorld())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(scene.Dispose)
	entity := scene.MustEntity("camera")
	camera, ok := ecs.Get[render3d.Camera](scene.World, entity)
	if !ok || camera.Quality != render3d.QualityMedium || !camera.FXAA {
		t.Fatal("camera quality/FXAA lost during spawn", camera)
	}
	components := Components(SceneComponentCamera)
	components.Camera.Quality, components.Camera.FXAA = render3d.QualityHigh, true
	cloned := components.Clone()
	cloned.Camera.Quality = render3d.QualityLow
	if components.Camera.Quality != render3d.QualityHigh {
		t.Fatal("camera clone aliases source")
	}
	plan, err := compileSceneSpawnPlan([]SceneNodeSpec{{ID: "camera", Components: components}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := typedbin.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := typedbin.Unmarshal[sceneSpawnPlan](data)
	if err != nil {
		t.Fatal(err)
	}
	spec := decoded.Nodes[0].Spec
	if spec.Components.Camera.Quality != render3d.QualityHigh || !spec.Components.Camera.FXAA {
		t.Fatal("binary plan lost camera settings")
	}
	spec.Components.Camera.Quality, spec.Components.Camera.FXAA = render3d.QualityLow, false
	if err := scene.ApplyNode(spec); err != nil {
		t.Fatal(err)
	}
	camera, _ = ecs.Get[render3d.Camera](scene.World, entity)
	if camera.Quality != render3d.QualityLow || camera.FXAA {
		t.Fatal("live edits lost camera settings")
	}
	value, err := decodeJSON[Camera3DSpec]([]byte(`{"quality":"ultra"}`), "camera")
	if err == nil {
		err = validateSceneComponents("main", "camera", SceneComponents{Camera: &value})
	}
	if err == nil {
		t.Fatal("invalid quality accepted")
	}
}
