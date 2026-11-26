package graphics

import "github.com/onix-engine/onix/render"

var meshShaderOps = shaderResourceOps[render.MeshShaderResource]{
	kind:        "mesh",
	unsupported: "graphics: current renderer does not support custom mesh shaders",
	disposed:    "graphics: mesh shader is disposed",
	create: func(driver render.Driver, descriptor render.ShaderDescriptor) (render.MeshShaderHandle, bool, error) {
		backend, ok := driver.(render.MeshShaderDriver)
		if !ok {
			return render.MeshShaderHandle{}, false, nil
		}
		handle, err := backend.CreateMeshShader(descriptor)
		return handle, true, err
	},
	destroy: func(driver render.Driver, handle render.MeshShaderHandle) error {
		backend, ok := driver.(render.MeshShaderDriver)
		if !ok {
			return nil
		}
		return backend.DestroyMeshShader(handle)
	},
}

// MeshShader owns a user-defined 3D shader and recreates its native pipeline
// when the installed render device changes.
type MeshShader = shaderResource[render.MeshShaderResource]

func NewMeshShader(descriptor render.ShaderDescriptor) (*MeshShader, error) {
	return newShaderResource(descriptor, &meshShaderOps)
}
