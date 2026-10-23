package editor

import (
	"io/fs"
	"path"
	"runtime"

	"github.com/diiyw/onix/project"
)

// Preview resources use native shaders without changing authoring plans or files.
// Viewport resource stores retain the result until the content revision changes.
type workspaceModResources struct {
	resources *project.Resources
	plan      *project.ModPlan
}

func openWorkspaceModResources(w *WorkspaceDocument, mod *project.ModPlan, feature project.ResourceFeature) (workspaceModResources, error) {
	source := workspaceFS{w: w}
	private, err := fs.Sub(source, string(mod.ID))
	if err != nil {
		return workspaceModResources{}, err
	}
	compiled := *mod
	files, err := project.PreprocessModShaders(private, &compiled, project.PreprocessOptions{
		Root: ".", ShaderTarget: shaderTargetForBuildPlatform(hostBuildPlatform(runtime.GOOS)), ShaderCompiler: NagaShaderCompiler{},
	})
	if err != nil {
		return workspaceModResources{}, err
	}
	if len(files) > 0 {
		source.overrides = make(map[DocumentID][]byte, len(files))
	}
	for _, file := range files {
		data := file.Data
		if file.Remove {
			data = nil
		}
		source.overrides[DocumentID(path.Join(string(mod.ID), file.Path))] = data
	}
	resources, err := project.OpenModResources(source, &compiled, feature)
	return workspaceModResources{resources: resources, plan: &compiled}, err
}
