package project

import (
	"os"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/internal/typedbin"
	"github.com/diiyw/onix/scene/render3d"
)

func TestPhotometricSceneLifecycle(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[
	{"id":"camera","components":{"camera":{"exposure":{"mode":"fixed","ev100":0}}}},
	{"id":"point","components":{"pointLight":{"intensity":100,"unit":"candela"}}},
	{"id":"spot","components":{"spotLight":{"shape":"pyramid","unit":"ev100","intensity":-1,"aspectRatio":2}}},
	{"id":"area","components":{"areaLight":{"shape":"tube","unit":"nits","intensity":50,"width":2}}}
	]}`)
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
	areaEntity := scene.MustEntity("area")
	area, ok := ecs.Get[render3d.AreaLight](scene.World, areaEntity)
	if !ok || area.Shape != render3d.AreaTube || area.Nits() != 50 || area.Width != 2 {
		t.Fatal(area)
	}
	point, ok := ecs.Get[render3d.PointLight](scene.World, scene.MustEntity("point"))
	if !ok || point.Candela() != 100 {
		t.Fatal(point)
	}
	spot, ok := ecs.Get[render3d.SpotLight](scene.World, scene.MustEntity("spot"))
	if !ok || spot.Shape != render3d.SpotPyramid || spot.PhysicalIntensity() != 0.0625 {
		t.Fatal(spot)
	}
	camera, ok := ecs.Get[render3d.Camera](scene.World, scene.MustEntity("camera"))
	if !ok || camera.Exposure.Mode != graphics.ExposureFixed || camera.Exposure.Value() != 0 {
		t.Fatal(camera)
	}
	spec := SceneNodeSpec{ID: "area", Components: Components(SceneComponentAreaLight), ScaleX: 1, ScaleY: 1, ScaleZ: 1}
	spec.Components.AreaLight.Shape = render3d.AreaDisc
	spec.Components.AreaLight.Unit = graphics.LightNits
	spec.Components.AreaLight.Intensity = 80
	cloned := spec.Components.Clone()
	cloned.AreaLight.Intensity = 90
	if spec.Components.AreaLight.Intensity != 80 {
		t.Fatal("area clone aliases authored component")
	}
	if err := scene.ApplyNode(spec); err != nil {
		t.Fatal(err)
	}
	area, _ = ecs.Get[render3d.AreaLight](scene.World, areaEntity)
	if area.Shape != render3d.AreaDisc || area.Nits() != 80 {
		t.Fatal(area)
	}
	plan, err := compileSceneSpawnPlan([]SceneNodeSpec{spec})
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
	if !decoded.Nodes[0].HasAreaLight || decoded.Nodes[0].AreaLight.Nits() != 80 {
		t.Fatal("compiled scene lost photometry")
	}
	spec.Components = Components(SceneComponentPointLight)
	if err := scene.ApplyNode(spec); err != nil {
		t.Fatal(err)
	}
	if ecs.Has[render3d.AreaLight](scene.World, areaEntity) || !ecs.Has[render3d.PointLight](scene.World, areaEntity) {
		t.Fatal("replaced area light remained on entity")
	}
}

func TestPhotometricSourceValidation(t *testing.T) {
	for _, raw := range []string{
		`{"illuminance":1000}`, `{"unit":"nits"}`, `{"range":0}`, `{"unit":"lux","luxDistance":0}`,
	} {
		light, err := decodeJSON[PointLightSpec]([]byte(raw), "point")
		if err == nil {
			_, err = light.Light()
		}
		if err == nil {
			t.Fatalf("invalid point accepted: %s", raw)
		}
	}
	for _, raw := range []string{`{"shape":"box","unit":"lumen"}`, `{"outerConeAngle":3.141593}`, `{"shape":"pyramid","aspectRatio":0}`} {
		light, err := decodeJSON[SpotLightSpec]([]byte(raw), "spot")
		if err == nil {
			_, err = light.Light()
		}
		if err == nil {
			t.Fatalf("invalid spot accepted: %s", raw)
		}
	}
}
