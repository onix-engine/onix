package editor

import (
	"strings"
	"testing"

	"github.com/diiyw/onix/project"
)

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
