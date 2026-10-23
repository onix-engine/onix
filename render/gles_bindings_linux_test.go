//go:build linux && !android

package render

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGLESMeshBindingReuse(t *testing.T) {
	compiler, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("requires a C compiler")
	}
	flags, err := exec.Command("pkg-config", "--cflags", "glesv2").Output()
	if err != nil {
		t.Skip("requires GLES3 development headers")
	}
	source, err := os.ReadFile("opengles.go")
	if err != nil {
		t.Fatal(err)
	}
	// Compile the production C wrapper against a driver that checks the state
	// seen by every draw, without creating a window or an OpenGL context.
	start := strings.Index(string(source), "typedef struct {\n\tGLuint texture;\n\tGLuint normal_texture;")
	if start < 0 {
		t.Fatal("missing native mesh draw declaration")
	}
	end := strings.Index(string(source[start:]), "\ntypedef struct {\n\tuintptr_t base;")
	if end < 0 {
		t.Fatal("missing end of native mesh draw wrapper")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gles_mesh_run.inc"), source[start:start+end], 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	binary := filepath.Join(dir, "gles_bindings_test")
	args := append([]string{"-std=c11", "-O2", "-Wall", "-Wextra", "-Werror", "-I" + dir, "tests/gles_bindings_test.c", "-o", binary}, strings.Fields(string(flags))...)
	if output, err := exec.CommandContext(ctx, compiler, args...).CombinedOutput(); err != nil {
		t.Fatalf("compile native GLES binding test: %v\n%s", err, output)
	}
	if output, err := exec.CommandContext(ctx, binary).CombinedOutput(); err != nil {
		t.Fatalf("native GLES binding test: %v\n%s", err, output)
	} else {
		t.Log(strings.TrimSpace(string(output)))
	}
}
