package main

import (
	"fmt"
	"os"

	"github.com/onix-engine/onix/editor"
	"github.com/onix-engine/onix/project"
)

func main() {
	const path = "ui/html/shaders/blur.wgsl"
	source, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	compiler := editor.NagaShaderCompiler{}
	for _, output := range []struct {
		path   string
		target project.ShaderTarget
	}{
		{target: project.ShaderTargetVulkan, path: "ui/html/shaders/blur.spv"},
		{target: project.ShaderTargetMetal, path: "ui/html/shaders/blur.metal"},
		{target: project.ShaderTargetWebGL2, path: "ui/html/shaders/blur.glsl"},
	} {
		compiled, err := compiler.CompileShader(project.ShaderCompileRequest{ID: "html-blur", SourcePath: path, Source: source, Target: output.target})
		if err != nil {
			panic(fmt.Errorf("%s: %w", output.target, err))
		}
		data := compiled.Fragment
		if output.target == project.ShaderTargetMetal {
			data = compiled.Metal
		}
		if err := os.WriteFile(output.path, data, 0644); err != nil {
			panic(err)
		}
	}
}
