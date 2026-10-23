package project

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
)

func TestModuleShaderProgramsAreSharedAndUniformsBelongToElements(t *testing.T) {
	render.Uninstall()
	source := fstest.MapFS{
		"resources/project.json":     {Data: []byte(`{"entry":"main.json"}`)},
		"resources/scenes/main.json": {Data: []byte(`{"modules":[{"id":"effect","mod":"effect"}]}`)},
		"effect/mod.json": {Data: []byte(`{"elements":[
			{"id":"first","components":{"sprite":{"color":"#ffffff"},"shader":{"wgslFragment":"resources/effect.wgsl","custom":[[0.25,0,0,0],[0,0,0,0],[0,0,0,0],[0,0,0,0]]}}},
			{"id":"second","components":{"sprite":{"color":"#ffffff"},"shader":{"wgslFragment":"resources/effect.wgsl","custom":[[0.75,0,0,0],[0,0,0,0],[0,0,0,0],[0,0,0,0]]}}}
		]}`)},
		"effect/resources/effect.wgsl": {Data: []byte("fragment")},
	}
	plan, err := CompileProject(source, ".")
	if err != nil {
		t.Fatal(err)
	}
	files, err := PreprocessModuleAssets(source, plan, PreprocessOptions{ShaderTarget: ShaderTargetWebGL2, ShaderCompiler: shaderCompileTestCompiler{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.Remove {
			delete(source, file.Path)
		} else {
			source[file.Path] = &fstest.MapFile{Data: file.Data}
		}
	}
	mod := &plan.Mods[0]
	resources, err := OpenModResources(source, mod, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resources.Dispose() })
	world := ecs.NewWorld()
	var entities [2]ecs.Entity
	for i, element := range mod.Elements {
		bundle, err := resources.ModContent(element.Spec.Components)
		if err != nil {
			t.Fatal(err)
		}
		entities[i] = world.Spawn(bundle)
	}
	first := ecs.GetMut[scene.Shader[graphics.Material2DShader]](world, entities[0])
	second := ecs.GetRef[scene.Shader[graphics.Material2DShader]](world, entities[1])
	if first.Program == nil || first.Program != second.Program || first.Custom[0][0] != .25 || second.Custom[0][0] != .75 {
		t.Fatal("shader programs were not shared or element uniforms were lost")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		ecs.GetMut[scene.Shader[graphics.Material2DShader]](world, entities[0]).Custom[0][0] += .01
	}); allocations != 0 {
		t.Fatalf("shader uniform updates allocated %g", allocations)
	}
	if second.Custom[0][0] != .75 {
		t.Fatal("updating one element changed the other element's uniforms")
	}
	if err := resources.Dispose(); err != nil || first.Program.UploadError() == nil {
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
