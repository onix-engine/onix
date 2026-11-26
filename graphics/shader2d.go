package graphics

import "github.com/onix-engine/onix/render"

var material2DShaderOps = shaderResourceOps[render.Material2DShaderResource]{
	kind:        "Material2D",
	unsupported: "graphics: current renderer does not support Material2D shaders",
	disposed:    "graphics: Material2D shader is disposed",
	create: func(driver render.Driver, descriptor render.ShaderDescriptor) (render.Material2DShaderHandle, bool, error) {
		backend, ok := driver.(render.Material2DShaderDriver)
		if !ok {
			return render.Material2DShaderHandle{}, false, nil
		}
		handle, err := backend.CreateMaterial2DShader(descriptor)
		return handle, true, err
	},
	destroy: func(driver render.Driver, handle render.Material2DShaderHandle) error {
		backend, ok := driver.(render.Material2DShaderDriver)
		if !ok {
			return nil
		}
		return backend.DestroyMaterial2DShader(handle)
	},
}

// Material2DShader owns a user-defined sprite pipeline and recreates it when
// the installed render device changes.
type Material2DShader = shaderResource[render.Material2DShaderResource]

func NewMaterial2DShader(descriptor render.ShaderDescriptor) (*Material2DShader, error) {
	return newShaderResource(descriptor, &material2DShaderOps)
}
