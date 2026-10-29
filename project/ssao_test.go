//go:build !onix_release

package project

import (
	"os"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/internal/typedbin"
	"github.com/diiyw/onix/scene/render3d"
)

func TestSSAOSceneLoadingCloningAndBinaryPlan(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[{"id":"camera","components":{"camera":{"ssao":{"radius":0.8,"samples":32}}}}]}`)
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
	camera, ok := scene.World.Get[render3d.Camera](scene.MustEntity("camera"))
	if !ok || !camera.SSAO.Enabled || camera.SSAO.Radius != .8 || camera.SSAO.Samples != 32 || camera.SSAO.Bias != .025 || camera.SSAO.ResolutionScale != .5 {
		t.Fatalf("SSAO settings or defaults were lost: %+v", camera.SSAO)
	}
	components := Components(SceneComponentCamera)
	components.Camera.SSAO = NewSSAOSpec()
	cloned := components.Clone()
	cloned.Camera.SSAO.Radius = 2
	if components.Camera.SSAO.Radius != .5 {
		t.Fatal("cloned SSAO settings alias their source")
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
	if *decoded.Nodes[0].Spec.Components.Camera.SSAO != *components.Camera.SSAO {
		t.Fatal("binary scene plan lost SSAO settings")
	}
}

func TestSSAOSourceValidation(t *testing.T) {
	for _, raw := range []string{
		`{"ssao":{"radius":0}}`, `{"ssao":{"bias":-1}}`,
		`{"ssao":{"bias":0.5}}`, `{"ssao":{"samples":12}}`,
		`{"ssao":{"resolutionScale":0.1}}`, `{"ssao":{"strength":5}}`,
		`{"ssao":{"unknown":1}}`,
	} {
		value, err := decodeJSON[Camera3DSpec]([]byte(raw), "camera")
		if err == nil {
			err = validateSceneComponents("main", "camera", SceneComponents{Camera: &value})
		}
		if err == nil {
			t.Fatalf("invalid SSAO settings accepted: %s", raw)
		}
	}
}
