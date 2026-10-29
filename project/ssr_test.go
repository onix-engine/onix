//go:build !onix_release

package project

import (
	"os"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/internal/typedbin"
	"github.com/diiyw/onix/scene/render3d"
)

func TestSSRSceneLoadingCloningAndBinaryPlan(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[{"id":"camera","components":{"camera":{"ssr":{"maxDistance":24,"steps":128}}}}]}`)
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
	if !ok || !camera.SSR.Enabled || camera.SSR.MaxDistance != 24 || camera.SSR.Steps != 128 || camera.SSR.Thickness != .1 || camera.SSR.MaxRoughness != .6 {
		t.Fatalf("SSR settings or defaults were lost: %+v", camera.SSR)
	}
	components := Components(SceneComponentCamera)
	components.Camera.SSR = NewSSRSpec()
	cloned := components.Clone()
	cloned.Camera.SSR.MaxDistance = 2
	if components.Camera.SSR.MaxDistance != 50 {
		t.Fatal("cloned SSR settings alias their source")
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
	if *decoded.Nodes[0].Spec.Components.Camera.SSR != *components.Camera.SSR {
		t.Fatal("binary scene plan lost SSR settings")
	}
}

func TestSSRSourceValidation(t *testing.T) {
	for _, raw := range []string{
		`{"ssr":{"maxDistance":0}}`, `{"ssr":{"thickness":0}}`,
		`{"ssr":{"thickness":50}}`, `{"ssr":{"steps":48}}`,
		`{"ssr":{"maxRoughness":0.045}}`, `{"ssr":{"strength":2}}`,
		`{"ssr":{"edgeFade":0}}`, `{"ssr":{"unknown":1}}`,
	} {
		value, err := decodeJSON[Camera3DSpec]([]byte(raw), "camera")
		if err == nil {
			err = validateSceneComponents("main", "camera", SceneComponents{Camera: &value})
		}
		if err == nil {
			t.Fatalf("invalid SSR settings accepted: %s", raw)
		}
	}
}
