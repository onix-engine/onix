package editor

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diiyw/onix/project"
)

func TestNagaShaderCompilerCompilesCanonicalWGSL(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	compiler := NagaShaderCompiler{}
	source, err := os.ReadFile(filepath.Join(root, "projects", "shader2d", "effect", "shaders", "effect.frag.wgsl"))
	if err != nil {
		t.Fatal(err)
	}
	request := project.ShaderCompileRequest{ID: "effect", SourcePath: "shaders/effect.frag.wgsl", Source: source}

	request.Target = project.ShaderTargetVulkan
	vulkan, err := compiler.CompileShader(request)
	if err != nil {
		t.Fatal(err)
	}
	if !vulkan.HasFragmentEntry || len(vulkan.Fragment) < 4 || binary.LittleEndian.Uint32(vulkan.Fragment) != 0x07230203 {
		t.Fatalf("invalid generated SPIR-V: entries=%v bytes=%d", vulkan.HasFragmentEntry, len(vulkan.Fragment))
	}

	request.Target = project.ShaderTargetMetal
	metal, err := compiler.CompileShader(request)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(metal.Metal, []byte("[[texture(0)]]")) || !bytes.Contains(metal.Metal, []byte("constexpr metal::sampler")) {
		t.Fatalf("generated Metal source has the wrong bindings:\n%s", metal.Metal)
	}

	request.Target = project.ShaderTargetWebGL2
	webgl, err := compiler.CompileShader(request)
	if err != nil {
		t.Fatal(err)
	}
	if !webgl.WebGL2Generated || !strings.Contains(string(webgl.Fragment), "#version 300 es") || !strings.Contains(string(webgl.Fragment), "_group_0_binding_1_fs") {
		t.Fatalf("invalid generated WebGL2 source:\n%s", webgl.Fragment)
	}
	if !strings.Contains(string(webgl.Fragment), "_vs2fs_location0") {
		t.Fatalf("generated WebGL2 source has incompatible varyings:\n%s", webgl.Fragment)
	}

	request.Target = project.ShaderTargetWebGPU
	webgpu, err := compiler.CompileShader(request)
	if err != nil {
		t.Fatal(err)
	}
	if !webgpu.HasFragmentEntry || len(webgpu.Fragment) != 0 {
		t.Fatalf("WebGPU compilation returned generated source: %+v", webgpu)
	}

	meshSource, err := os.ReadFile(filepath.Join(root, "projects", "shader3d", "effect", "shaders", "effect.frag.wgsl"))
	if err != nil {
		t.Fatal(err)
	}
	request.ID, request.SourcePath, request.Source = "mesh-effect", "shaders/mesh-effect.frag.wgsl", meshSource
	request.Target = project.ShaderTargetMetal
	meshMetal, err := compiler.CompileShader(request)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(meshMetal.Metal, []byte("[[buffer(2)]]")) {
		t.Fatalf("generated Metal source has no Onix immediate buffer:\n%s", meshMetal.Metal)
	}
	request.Target = project.ShaderTargetWebGL2
	meshWebGL, err := compiler.CompileShader(request)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(meshWebGL.Fragment, []byte("_immediates_binding_fs.cameraPosition")) {
		t.Fatalf("generated WebGL2 source has incompatible uniforms:\n%s", meshWebGL.Fragment)
	}
}

func TestNagaShaderCompilerUsesOnixWebGLClipSpace(t *testing.T) {
	compiler := NagaShaderCompiler{}
	result, err := compiler.CompileShader(project.ShaderCompileRequest{
		ID:         "clip-space",
		SourcePath: "shaders/clip-space.wgsl",
		Source: []byte(`@vertex
fn sprite_vertex() -> @builtin(position) vec4<f32> {
    return vec4<f32>(0.0, 0.5, 0.25, 1.0);
}`),
		Target: project.ShaderTargetWebGL2,
	})
	if err != nil {
		t.Fatal(err)
	}
	vertex := string(result.Vertex)
	if !strings.Contains(vertex, "gl_Position.z = gl_Position.z * 2.0 - gl_Position.w;") {
		t.Fatalf("generated WebGL2 vertex shader has no depth conversion:\n%s", vertex)
	}
	if strings.Contains(vertex, "-gl_Position.y") {
		t.Fatalf("generated WebGL2 vertex shader flips Y:\n%s", vertex)
	}
}
