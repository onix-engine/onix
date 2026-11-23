package project

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/graphics"
	"github.com/onix-engine/onix/render"
	"github.com/onix-engine/onix/scene"
)

func TestModuleShaderProgramsAreSharedAndUniformsBelongToElements(t *testing.T) {
	render.Uninstall()
	const fragment = "resources/effect.frag"
	source := fstest.MapFS{
		"effect/" + fragment: {Data: []byte("generated fragment")},
	}
	resources, err := OpenModResources(source, &ModPlan{ID: "effect"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resources.Dispose() })
	world := ecs.NewWorld()
	var entities [2]ecs.Entity
	for i, uniform := range [2]float32{.25, .75} {
		bundle, err := resources.ModContent(ModComponents{
			Sprite: &ModSpriteSpec{Color: "#ffffff"},
			Shader: &ShaderComponentSpec{
				Compiled: ShaderSources{WebGL2Fragment: fragment, WebGL2Generated: true},
				Custom:   [4][4]float32{{uniform}},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		entities[i] = world.Spawn(bundle)
	}
	first := world.GetMut[scene.Shader[graphics.Material2DShader]](entities[0])
	second := world.GetRef[scene.Shader[graphics.Material2DShader]](entities[1])
	if first.Program == nil || first.Program != second.Program || first.Custom[0][0] != .25 || second.Custom[0][0] != .75 {
		t.Fatal("shader programs were not shared or element uniforms were lost")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		world.GetMut[scene.Shader[graphics.Material2DShader]](entities[0]).Custom[0][0] += .01
	}); allocations != 0 {
		t.Fatalf("shader uniform updates allocated %g", allocations)
	}
	if second.Custom[0][0] != .75 {
		t.Fatal("updating one element changed the other element's uniforms")
	}
	model, err := resources.ModContent(ModComponents{
		Model: &ModelSpec{ID: "cube", Mesh: &MeshSpec{Primitive: "cube", Size: 1}},
		Shader: &ShaderComponentSpec{
			Compiled: ShaderSources{WebGL2Fragment: fragment, WebGL2Generated: true},
			Custom:   [4][4]float32{{.5}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	entity := world.Spawn(model)
	mesh := world.GetRef[scene.Shader[graphics.MeshShader]](entity)
	if mesh == nil || mesh.Program == nil || mesh.Custom[0][0] != .5 || world.Has[scene.Shader[graphics.Material2DShader]](entity) {
		t.Fatal("model did not receive its mesh shader and uniforms")
	}
	if stats := resources.modShaders.statistics(); stats.Misses != 1 || stats.Hits != 2 {
		t.Fatalf("sprite and model should share compiled shader sources, got %+v", stats)
	}
	if err := resources.Dispose(); err != nil || first.Program.UploadError() == nil || mesh.Program.UploadError() == nil {
		t.Fatal("module shader program was not disposed with its resource store", err)
	}
}

func TestModuleShaderAuthoringRequiresElementComponents(t *testing.T) {
	tests := []struct{ name, mod, want string }{
		{"module registry", `{"shaders":{}}`, `unknown field "shaders"`},
		{"module root", `{"components":{"shader":{}}}`, "shader must be attached to an element"},
		{"material reference", `{"elements":[{"id":"effect","components":{"material":{"shader":"effect"}}}]}`, `unknown field "shader"`},
		{"foreign path", `{"elements":[{"id":"effect","components":{"shader":{"wgslFragment":"../foreign.wgsl"}}}]}`, "must stay within module"},
		{"missing source", `{"elements":[{"id":"effect","components":{"shader":{"wgslFragment":"missing.wgsl"}}}]}`, "missing.wgsl"},
		{"empty component", `{"elements":[{"id":"effect","components":{"shader":{}}}]}`, ""},
	}
	for _, field := range []string{"vulkanVertex", "vulkanFragment", "metal", "webGL2Vertex", "webGL2Fragment", "webGPUVertex", "webGPUFragment", "webGL2Generated", "compiled", "Compiled"} {
		tests = append(tests, struct{ name, mod, want string }{field,
			`{"elements":[{"id":"effect","components":{"shader":{"` + field + `":"generated"}}}]}`,
			`unknown field "` + field + `"`})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := fstest.MapFS{
				"resources/project.json":     {Data: []byte(`{"entry":"main.json"}`)},
				"resources/scenes/main.json": {Data: []byte(`{"modules":[{"id":"effect","mod":"effect"}]}`)},
				"effect/mod.json":            {Data: []byte(test.mod)},
			}
			_, err := CompileProject(source, ".")
			if test.want == "" {
				if err != nil {
					t.Fatal("adding an unconfigured shader component failed", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("want %q, got %v", test.want, err)
			}
		})
	}
}
