//go:build !onix_release

package project

import (
	"os"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/internal/typedbin"
	"github.com/diiyw/onix/scene/render3d"
)

func TestShadowSceneLoadingCloningAndBinaryPlan(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[
	{"id":"sun","components":{"directionalLight":{"shadows":{"resolution":64,"cascades":4,"distance":80,"splitLambda":0.7,"cascadeBlend":0.15}}}},
	{"id":"point","components":{"pointLight":{"shadows":{"resolution":64,"near":0.1}}}},
	{"id":"spot","components":{"spotLight":{"shape":"pyramid","aspectRatio":2,"shadows":{"resolution":64}}}}
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
	sun, _ := scene.World.Get[render3d.DirectionalLight](scene.MustEntity("sun"))
	point, _ := scene.World.Get[render3d.PointLight](scene.MustEntity("point"))
	spot, _ := scene.World.Get[render3d.SpotLight](scene.MustEntity("spot"))
	if !sun.Shadows.Enabled || sun.Shadows.Cascades != 4 || sun.Shadows.Distance != 80 || sun.Shadows.SplitLambda != .7 || sun.Shadows.CascadeBlend != .15 || !point.Shadows.Enabled || point.Shadows.Near != .1 || !spot.Shadows.Enabled || spot.Shadows.Near != .05 {
		t.Fatalf("shadow settings were lost: sun=%+v point=%+v spot=%+v", sun.Shadows, point.Shadows, spot.Shadows)
	}
	components := Components(SceneComponentPointLight, SceneComponentSpotLight, SceneComponentDirectionalLight)
	components.PointLight.Shadows, components.SpotLight.Shadows, components.DirectionalLight.Shadows = NewPunctualShadowSpec(), NewPunctualShadowSpec(), NewShadowSpec()
	cloned := components.Clone()
	cloned.PointLight.Shadows.Near = .2
	cloned.SpotLight.Shadows.Bias = .01
	cloned.DirectionalLight.Shadows.Cascades = 4
	if components.PointLight.Shadows.Near != .05 || components.SpotLight.Shadows.Bias != .0003 || components.DirectionalLight.Shadows.Cascades != 1 {
		t.Fatal("cloned shadows alias the authored settings")
	}
	plan, err := compileSceneSpawnPlan([]SceneNodeSpec{{ID: "light", Components: components}})
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
	want, got := plan.Nodes[0], decoded.Nodes[0]
	if got.Light != want.Light || got.PointLight != want.PointLight || got.SpotLight != want.SpotLight {
		t.Fatal("binary scene plan changed light or shadow parameters")
	}
}

func TestShadowSourceRejectsInvalidProjectionSettings(t *testing.T) {
	for _, raw := range []string{
		`{"shadows":{"resolution":100}}`, `{"shadows":{"near":0}}`,
		`{"range":1,"shadows":{"near":1}}`, `{"shadows":{"bias":-1}}`,
		`{"shadows":{"unknown":1}}`,
	} {
		value, err := decodeJSON[PointLightSpec]([]byte(raw), "point")
		if err == nil {
			_, err = value.Light()
		}
		if err == nil {
			t.Fatalf("invalid local shadow settings accepted: %s", raw)
		}
	}
	for _, raw := range []string{
		`{"shadows":{"cascades":5}}`, `{"shadows":{"cascades":4,"distance":0}}`,
		`{"shadows":{"splitLambda":1.1}}`, `{"shadows":{"cascadeBlend":0.6}}`,
	} {
		value, err := decodeJSON[DirectionalLightSpec]([]byte(raw), "sun")
		if err == nil {
			_, err = compileSceneDirectionalLight(SceneNodeSpec{Components: SceneComponents{DirectionalLight: &value}})
		}
		if err == nil {
			t.Fatalf("invalid cascade settings accepted: %s", raw)
		}
	}
}
