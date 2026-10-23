//go:build !onix_release

package project

import (
	"bytes"
	"encoding/json"
	"testing"
	"testing/fstest"

	"github.com/diiyw/onix/internal/typedbin"
)

type countingModShaderCompiler struct{ calls int }

func (c *countingModShaderCompiler) CompileShader(request ShaderCompileRequest) (ShaderCompileResult, error) {
	c.calls++
	return (shaderCompileTestCompiler{}).CompileShader(request)
}

func TestPreprocessModShadersSharesSourcesAndPreservesAuthoring(t *testing.T) {
	source := fstest.MapFS{"resources/effect.wgsl": {Data: []byte("fragment")}}
	shader := &ShaderComponentSpec{WGSLFragment: "resources/effect.wgsl", Custom: [4][4]float32{{.25}}}
	mod := ModPlan{Spec: ModSpec{Elements: []ModElementSpec{
		{ID: "first", Components: ModComponents{Shader: shader}},
		{ID: "second", Components: ModComponents{Shader: shader}},
	}}}
	for _, element := range mod.Spec.Elements {
		mod.Elements = append(mod.Elements, ModElementPlan{Spec: element})
	}
	compiled := mod
	compiler := &countingModShaderCompiler{}
	files, err := PreprocessModShaders(source, &compiled, PreprocessOptions{ShaderTarget: ShaderTargetVulkan, ShaderCompiler: compiler})
	if err != nil {
		t.Fatal(err)
	}
	if compiler.calls != 1 || len(files) != 2 {
		t.Fatalf("shared shader compiled %d times into %d files", compiler.calls, len(files))
	}
	for i, element := range compiled.Elements {
		got := element.Spec.Components.Shader
		if got.Compiled.VulkanFragment == "" || got.Compiled.WGSLFragment != "" || got.WGSLFragment != shader.WGSLFragment || got.Custom[0][0] != .25 || got != compiled.Spec.Elements[i].Components.Shader {
			t.Fatal("compiled component lost source paths or uniforms")
		}
		if mod.Elements[i].Spec.Components.Shader.Compiled.HasSource() || mod.Spec.Elements[i].Components.Shader != shader {
			t.Fatal("shader preprocessing modified the authoring plan")
		}
	}
	authoredJSON, err := json.Marshal(mod.Spec)
	if err != nil {
		t.Fatal(err)
	}
	compiledJSON, err := json.Marshal(compiled.Spec)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(compiledJSON, authoredJSON) {
		t.Fatalf("compiled shader paths leaked into authoring JSON: %s", compiledJSON)
	}
	data, err := typedbin.Marshal(compiled)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := typedbin.Unmarshal[ModPlan](data)
	if err != nil {
		t.Fatal(err)
	}
	for i, element := range decoded.Elements {
		if *element.Spec.Components.Shader != *compiled.Elements[i].Spec.Components.Shader {
			t.Fatal("binary plan lost compiled shader paths or uniforms")
		}
	}
}
