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

func TestGLESMeshDirectionalLightPixels(t *testing.T) {
	compiler, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("requires cc and EGL/GLES3 development libraries")
	}
	flags, err := exec.Command("pkg-config", "--cflags", "--libs", "egl", "glesv2").Output()
	if err != nil {
		t.Skip("requires EGL/GLES3 development libraries")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dir := t.TempDir()
	binary := filepath.Join(dir, "gles_mesh_test")
	args := append([]string{"-std=c11", "-O2", "tests/gles_mesh_test.c", "-o", binary, "-lm"}, strings.Fields(string(flags))...)
	if output, err := exec.CommandContext(ctx, compiler, args...).CombinedOutput(); err != nil {
		t.Fatalf("compile GLES test: %v\n%s", err, output)
	}
	// GLES and WebGL2 embed the same generated fragment stage.

	for _, shader := range []string{"shaders/gles_mesh.frag"} {
		t.Run(filepath.Base(shader), func(t *testing.T) {
			command := exec.CommandContext(ctx, binary, shader)
			command.Env = append(os.Environ(), "EGL_PLATFORM=surfaceless", "LIBGL_ALWAYS_SOFTWARE=1", "LP_NUM_THREADS=2")
			output, err := command.CombinedOutput()
			if status, ok := err.(*exec.ExitError); ok && status.ExitCode() == 77 {
				t.Skip(string(output))
			}
			if err != nil {
				t.Fatalf("GLES light pixels: %v\n%s", err, output)
			}
			t.Log(strings.TrimSpace(string(output)))
		})
	}
}
