package project

import (
	"math"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
)

func TestModuleIndependentTrailAndMaterialCompileAndInstantiate(t *testing.T) {
	source := fstest.MapFS{
		"resources/project.json":     {Data: []byte(`{"entry":"main.json"}`)},
		"resources/scenes/main.json": {Data: []byte(`{"modules":[{"id":"effect","mod":"effect"}]}`)},
		"effect/mod.json":            {Data: []byte(`{"components":{"trail":{"lifetime":0.8,"maxSamples":12},"material":{"custom":[[0.5,0,0,0],[0,0,0,0],[0,0,0,0],[0,0,0,0]]}}}`)},
	}
	plan, err := CompileProject(source, ".")
	if err != nil {
		t.Fatal(err)
	}
	features := plan.RuntimeFeatures()
	if !slices.Contains(features, FeatureSpriteTrails2D) || !slices.Contains(features, FeatureRender2D) {
		t.Fatalf("independent components did not select required systems: %v", features)
	}
	components := plan.Mods[plan.Scenes[0].Instances[0].Mod].Spec.Components
	resources := newResources()
	first, err := resources.ModContent(components)
	if err != nil {
		t.Fatal(err)
	}
	trail, ok := ecs.BundleValue[scene.SpriteTrail2D](first)
	if !ok || trail.Lifetime != .8 || trail.MaxSamples != 12 {
		t.Fatal("Trail did not instantiate independently of Sprite")
	}
	material, ok := ecs.BundleValue[scene.MeshMaterial2D[ModMaterial2D]](first)
	if !ok || material.Material == nil || material.Material.Custom[0][0] != .5 {
		t.Fatal("default Material did not instantiate independently of Sprite")
	}
	second, err := resources.ModContent(components)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := ecs.BundleValue[scene.MeshMaterial2D[ModMaterial2D]](second)
	material.Material.Custom[0][0] = .75
	if other.Material == material.Material || other.Material.Custom[0][0] != .5 {
		t.Fatal("material uniforms are shared between instances")
	}
}

func TestModuleShaderComponentAppliesToSprite(t *testing.T) {
	resources := newResources()
	resources.source = fstest.MapFS{"effect.glsl": {Data: []byte("fragment")}}
	t.Cleanup(func() { _ = resources.Dispose() })
	components := ModComponents{
		Sprite: &ModSpriteSpec{Color: "#123456", Size: []float32{80, 40}},
		Trail:  &SpriteTrailSpec{MaxSamples: 16},
		Shader: &ShaderComponentSpec{Compiled: ShaderSources{WebGL2Fragment: "effect.glsl"}, Custom: [4][4]float32{{.5}}},
	}
	bundle, err := resources.ModContent(components)
	if err != nil {
		t.Fatal(err)
	}
	sprite, ok := ecs.BundleValue[scene.Sprite2D](bundle)
	if !ok || sprite.Texture == nil || sprite.Width != 80 || sprite.Height != 40 {
		t.Fatal("shader component did not prepare the solid sprite for shader rendering")
	}
	shader, _ := ecs.BundleValue[scene.Shader[graphics.Material2DShader]](bundle)
	if shader.Program == nil || shader.Custom[0][0] != .5 {
		t.Fatal("shader program or uniforms were lost")
	}
	if _, ok := ecs.BundleValue[scene.SpriteTrail2D](bundle); !ok {
		t.Fatal("Trail was lost when Shader was present")
	}
}

func TestModuleIndependentComponentValidation(t *testing.T) {
	compiler := modCompiler{}
	plan := ModPlan{}
	for _, test := range []struct {
		name       string
		components ModComponents
		want       string
	}{
		{"negative trail lifetime", ModComponents{Trail: &SpriteTrailSpec{Lifetime: -1}}, "lifetime must be non-negative"},
		{"invalid trail alpha", ModComponents{Trail: &SpriteTrailSpec{AlphaStart: 2}}, "alphaStart must be between"},
		{"empty shader source", ModComponents{Shader: &ShaderComponentSpec{WGSLFragment: " "}}, "requires at least one platform source"},
		{"non-finite uniforms", ModComponents{Material: &ModMaterial2DSpec{Custom: [4][4]float32{{float32(math.NaN())}}}}, "material.custom must be finite"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := compiler.validateContent("effect", test.components, &plan, modIndices{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("want %q, got %v", test.want, err)
			}
		})
	}
}
