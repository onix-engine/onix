//go:build windows && cgo && onix_opengl

package render

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenGLNativeRendering(t *testing.T) {
	compiler, err := exec.LookPath("gcc")
	if err != nil {
		t.Skip("OpenGL pixel regression requires MinGW-w64 gcc")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	root := t.TempDir()
	binary := filepath.Join(root, "opengl_test.exe")
	var args []string
	for index, source := range []string{spriteMultiVertexGLSL, spriteMultiFragmentGLSL, glesSpriteVertex, glesSpriteFragment, glesShapeVertex, glesShapeFragment, glesMeshVertex, glesMeshFragment, glesMeshVertex, glesMeshUnlitFragment, glesMeshVertex, glesMeshDepthFragment, glesMeshVertex, glesMeshBasicFragment, glesPickingVertex, glesPickingFragment} {
		name := filepath.Join(root, string(rune('a'+index))+".glsl")
		if err := os.WriteFile(name, []byte(platformGLShaderSource(source)), 0600); err != nil {
			t.Fatal(err)
		}
		args = append(args, name)
	}
	compile := exec.CommandContext(ctx, compiler, "-std=c11", "-O2", "-DONIX_DESKTOP_GL", "-I.", "tests/opengl_native_test.c", "opengl_context_windows.c", "gl_loader_windows.c", "-lopengl32", "-lgdi32", "-o", binary)
	if output, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("compile OpenGL test: %v\n%s", err, output)
	}
	// The native harness creates a hidden window, compiles production shaders,
	// and reads an offscreen target without opening the editor.
	if output, err := exec.CommandContext(ctx, binary, args...).CombinedOutput(); err != nil {
		t.Fatalf("OpenGL rendering: %v\n%s", err, output)
	} else {
		t.Log(string(output))
	}
}
