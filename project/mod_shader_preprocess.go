//go:build !onix_release

package project

import (
	"io/fs"
	"slices"
)

type modShaderTarget struct {
	element, shader int
}

type modShaderPreprocessor struct {
	specs   []ShaderSpec
	targets []modShaderTarget
	indices map[[2]string]int
}

func (p *modShaderPreprocessor) add(index int, element ModElementPlan) {
	shader := element.Spec.Components.Shader
	if shader == nil || !shader.HasSource() {
		return
	}
	if p.indices == nil {
		p.indices = make(map[[2]string]int)
	}
	sources := shader.SourcePaths()
	slot, exists := p.indices[sources]
	if !exists {
		slot = len(p.specs)
		p.indices[sources] = slot
		p.specs = append(p.specs, ShaderSpec{ID: element.Spec.ID, ShaderSources: shader.wgslSources()})
	}
	p.targets = append(p.targets, modShaderTarget{element: index, shader: slot})
}

func (p *modShaderPreprocessor) compile(source fs.FS, mod *ModPlan, options PreprocessOptions) ([]PreprocessedFile, error) {
	if len(p.specs) == 0 {
		return nil, nil
	}
	files, err := preprocessShaderSpecs(source, ".", p.specs, options.ShaderTarget, options.ShaderCompiler)
	if err != nil {
		return nil, err
	}
	// Plans may share authoring slices and pointers with the editor's documents.
	mod.Elements = slices.Clone(mod.Elements)
	mod.Spec.Elements = slices.Clone(mod.Spec.Elements)
	for _, target := range p.targets {
		shader := *mod.Elements[target.element].Spec.Components.Shader
		shader.Compiled = p.specs[target.shader].ShaderSources
		mod.Elements[target.element].Spec.Components.Shader = &shader
		mod.Spec.Elements[target.element].Components.Shader = &shader
	}
	return files, nil
}

// PreprocessModShaders compiles element shader components using the module's
// private filesystem. Sources shared by elements are compiled only once.
func PreprocessModShaders(source fs.FS, mod *ModPlan, options PreprocessOptions) ([]PreprocessedFile, error) {
	var shaders modShaderPreprocessor
	for i, element := range mod.Elements {
		shaders.add(i, element)
	}
	return shaders.compile(source, mod, options)
}
