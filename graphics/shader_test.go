package graphics

import (
	"testing"

	"github.com/diiyw/onix/render"
)

func TestMeshShaderCanBeCreatedBeforeRendererAndDisposed(t *testing.T) {
	render.Uninstall()
	shader, err := NewMeshShader(render.ShaderDescriptor{WebGL2FragmentSource: "fragment"})
	if err != nil {
		t.Fatalf("create deferred mesh shader: %v", err)
	}
	if shader.RenderHandle().Valid() {
		t.Fatal("deferred mesh shader unexpectedly has a GPU handle")
	}
	if err := shader.Dispose(); err != nil {
		t.Fatalf("dispose deferred mesh shader: %v", err)
	}
	descriptor := shader.Descriptor()
	if shader.UploadError() == nil || len(descriptor.VulkanVertexSPIRV) != 0 || len(descriptor.VulkanFragmentSPIRV) != 0 ||
		descriptor.MetalSource != "" || descriptor.WebGL2VertexSource != "" || descriptor.WebGL2FragmentSource != "" {
		t.Fatal("disposed mesh shader remained uploadable")
	}
}

func TestMaterial2DShaderCanBeCreatedBeforeRendererAndDisposed(t *testing.T) {
	render.Uninstall()
	shader, err := NewMaterial2DShader(render.ShaderDescriptor{WebGL2FragmentSource: "fragment"})
	if err != nil {
		t.Fatalf("create deferred Material2D shader: %v", err)
	}
	if shader.RenderHandle().Valid() {
		t.Fatal("deferred Material2D shader unexpectedly has a GPU handle")
	}
	if err := shader.Dispose(); err != nil {
		t.Fatalf("dispose deferred Material2D shader: %v", err)
	}
	descriptor := shader.Descriptor()
	if shader.UploadError() == nil || len(descriptor.VulkanVertexSPIRV) != 0 || len(descriptor.VulkanFragmentSPIRV) != 0 ||
		descriptor.MetalSource != "" || descriptor.WebGL2VertexSource != "" || descriptor.WebGL2FragmentSource != "" {
		t.Fatal("disposed Material2D shader remained uploadable")
	}
}
