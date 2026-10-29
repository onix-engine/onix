//go:build !onix_release

package project

import (
	"os"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/internal/typedbin"
	"github.com/diiyw/onix/scene/render3d"
)

func TestFogAndToneMappingSceneBinaryAndLiveEdits(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[{"id":"camera","components":{"camera":{"toneMapping":"aces","fog":{"mode":"exp2","density":0.04}}}}]}`)
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
	camera, _ := scene.World.Get[render3d.Camera](entity)
	if camera.ToneMapping != graphics.ToneMappingACES || !camera.Fog.Enabled || camera.Fog.Mode != render3d.FogExp2 || camera.Fog.Density != .04 || camera.Fog.Brightness != 800 || camera.Fog.Start != 20 {
		t.Fatal("fog/curve defaults lost", camera)
	}
	components := Components(SceneComponentCamera)
	components.Camera.Fog = NewFogSpec()
	components.Camera.ToneMapping = graphics.ToneMappingACES
	cloned := components.Clone()
	cloned.Camera.Fog.Start = 10
	if components.Camera.Fog.Start != 20 {
		t.Fatal("fog clone aliases source")
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
	if *spec.Components.Camera.Fog != *components.Camera.Fog || spec.Components.Camera.ToneMapping != graphics.ToneMappingACES {
		t.Fatal("binary plan lost fog/curve")
	}
	spec.Components.Camera.Fog.Start = 10
	if err := scene.ApplyNode(spec); err != nil {
		t.Fatal(err)
	}
	camera, _ = scene.World.Get[render3d.Camera](entity)
	if camera.Fog.Mode != render3d.FogLinear || camera.Fog.Start != 10 {
		t.Fatal("live fog edit lost")
	}
	spec.Components.Camera.Fog = nil
	spec.Components.Camera.ToneMapping = ""
	if err := scene.ApplyNode(spec); err != nil {
		t.Fatal(err)
	}
	camera, _ = scene.World.Get[render3d.Camera](entity)
	if camera.Fog.Enabled || camera.ToneMapping != "" {
		t.Fatal("live fog/curve removal lost")
	}
}

func TestFogAndToneSourceValidation(t *testing.T) {
	for _, raw := range []string{`{"toneMapping":"unknown"}`, `{"fog":{"mode":"height"}}`, `{"fog":{"start":-1}}`, `{"fog":{"end":0}}`, `{"fog":{"density":-1}}`, `{"fog":{"brightness":-1}}`, `{"fog":{"color":"invalid"}}`, `{"fog":{"unknown":1}}`} {
		value, err := decodeJSON[Camera3DSpec]([]byte(raw), "camera")
		if err == nil {
			err = validateSceneComponents("main", "camera", SceneComponents{Camera: &value})
		}
		if err == nil {
			t.Fatal("invalid fog/curve source accepted", raw)
		}
	}
}
