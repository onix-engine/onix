//go:build !onix_release

package project

import (
	"fmt"
	"io/fs"
	"path"
)

// PreprocessModuleAssets compiles native asset families from the module plan.
// Authoring manifests are never loaded here. Paths stay relative to each module
// until the resulting private files are placed in the project resource package.
func PreprocessModuleAssets(source fs.FS, plan *ProjectPlan, options PreprocessOptions) ([]PreprocessedFile, error) {
	used := make([]bool, len(plan.Mods))
	var total, completed int
	for _, s := range plan.Scenes {
		for _, i := range s.Instances {
			if !used[i.Mod] {
				total++
				used[i.Mod] = true
			}
		}
	}
	var result []PreprocessedFile
	for i := range plan.Mods {
		if !used[i] {
			continue
		}
		mod := &plan.Mods[i]
		private, err := fs.Sub(source, string(mod.ID))
		if err != nil {
			return nil, err
		}
		models := map[string]ModelSpec{}
		spines := map[string]SpineSpec{}
		var shaderAssets modShaderPreprocessor
		content := func(c ModComponents) error {
			if m := c.Model; m != nil {
				if previous, ok := models[m.ID]; ok && !equalModelSpec(previous, *m) {
					return fmt.Errorf("%s: conflicting private model id %q", mod.ID, m.ID)
				}
				models[m.ID] = *m
			}
			if s := c.Spine; s != nil {
				if previous, ok := spines[s.Source.ID]; ok && previous != s.Source {
					return fmt.Errorf("%s: conflicting private spine id %q", mod.ID, s.Source.ID)
				}
				spines[s.Source.ID] = s.Source
			}
			return nil
		}
		if err := content(mod.Spec.Components); err != nil {
			return nil, err
		}
		for i, e := range mod.Elements {
			shaderAssets.add(i, e)
			if err := content(e.Spec.Components); err != nil {
				return nil, err
			}
		}
		modelSpecs := make([]ModelSpec, 0, len(models))
		for _, k := range sortedModKeys(models) {
			modelSpecs = append(modelSpecs, models[k])
		}
		spineSpecs := make([]SpineSpec, 0, len(spines))
		for _, k := range sortedModKeys(spines) {
			spineSpecs = append(spineSpecs, spines[k])
		}
		if options.Progress != nil {
			options.Progress(string(mod.ID), "models", completed, total)
		}
		compiled, err := preprocessModelResources(private, ".", modelSpecs, options.TextureEncoder)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", mod.ID, err)
		}
		if options.Progress != nil {
			options.Progress(string(mod.ID), "collisions", completed, total)
		}
		collision, err := preprocessCollisionResources(private, ".", modelSpecs)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", mod.ID, err)
		}
		compiled = append(compiled, collision...)
		if options.Progress != nil {
			options.Progress(string(mod.ID), "spines", completed, total)
		}
		skeletons, err := preprocessSpines(private, ".", spineSpecs)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", mod.ID, err)
		}
		compiled = append(compiled, skeletons...)
		if options.Progress != nil {
			options.Progress(string(mod.ID), "shaders", completed, total)
		}
		shaders, err := shaderAssets.compile(private, mod, options)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", mod.ID, err)
		}
		compiled = append(compiled, shaders...)
		completed++
		if options.Progress != nil {
			options.Progress(string(mod.ID), "ready", completed, total)
		}
		for _, f := range compiled {
			f.Path = path.Join(string(mod.ID), f.Path)
			if f.SourcePath != "" {
				f.SourcePath = path.Join(string(mod.ID), f.SourcePath)
			}
			result = append(result, f)
		}
	}
	return result, nil
}
