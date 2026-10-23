//go:build linux && !android && cgo

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

func runNativeUploadTest(t *testing.T, compiler string, args []string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "native_upload_test")
	args = append(args, "-o", binary)
	if output, err := exec.CommandContext(ctx, compiler, args...).CombinedOutput(); err != nil {
		t.Fatalf("compile upload test: %v\n%s", err, output)
	}
	command := exec.CommandContext(ctx, binary)
	command.Env = append(os.Environ(), "EGL_PLATFORM=surfaceless", "LIBGL_ALWAYS_SOFTWARE=1", "LP_NUM_THREADS=2")
	output, err := command.CombinedOutput()
	if status, ok := err.(*exec.ExitError); ok && status.ExitCode() == 77 {
		t.Skip(string(output))
	}
	if err != nil {
		t.Fatalf("native upload test: %v\n%s", err, output)
	}
	t.Log(strings.TrimSpace(string(output)))
}

func TestGLESUploadPool(t *testing.T) {
	compiler, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("requires a C compiler")
	}
	flags, err := exec.Command("pkg-config", "--cflags", "--libs", "egl", "glesv2").Output()
	if err != nil {
		t.Skip("requires EGL/GLES3 development libraries")
	}
	source, err := os.ReadFile("opengles.go")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(source), "static void onix_gl_vertex_attrib_pointer(")
	end := strings.Index(string(source), "\n*/\nimport \"C\"")
	if start < 0 || end <= start {
		t.Fatal("missing production GLES wrappers")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gles_draw_wrappers.inc"), source[start:end], 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-std=c11", "-O2", "-Wall", "-Wextra", "-Werror", "-Wno-unused-function", "-I.", "-I" + dir, "tests/gles_upload_test.c"}
	runNativeUploadTest(t, compiler, append(args, strings.Fields(string(flags))...))
}

func TestMetalUploadPool(t *testing.T) {
	compiler, err := exec.LookPath("clang")
	if err != nil {
		t.Skip("requires clang and the Objective-C runtime")
	}
	include, err := exec.Command("cc", "-print-file-name=include").Output()
	if err != nil {
		t.Skip("requires Objective-C headers")
	}
	if _, err := os.Stat(filepath.Join(strings.TrimSpace(string(include)), "objc", "runtime.h")); err != nil {
		t.Skip("requires Objective-C headers")
	}
	source, err := os.ReadFile("metal_native.m")
	if err != nil {
		t.Fatal(err)
	}
	// Compile the actual production allocator against native mock Metal objects.
	// The mock checks CPU ownership and ranges; Apple GPU execution stays a
	// separate platform check.
	section := func(start, end string) string {
		t.Helper()
		a := strings.Index(string(source), start)
		if a < 0 {
			t.Fatalf("missing Metal section %q", start)
		}
		b := strings.Index(string(source[a:]), end)
		if b < 0 {
			t.Fatalf("missing Metal section end %q", end)
		}
		return string(source[a : a+b])
	}
	dir := t.TempDir()
	sections := map[string]string{
		"metal_resource_types.inc": section("typedef struct {\n    id<MTLTexture> texture;", "ONIX_NATIVE_SLOTS(onix_metal_textures"),
		"metal_driver.inc":         section("struct OnixMetalDriver {", "\nvoid onix_metal_buffer_memory"),
		"metal_mesh_draws.inc":     section("int onix_metal_draw_meshes(", "\n_Static_assert"),
		"metal_upload_types.inc":   section("typedef struct {\n\tid<MTLBuffer> buffer;", "struct OnixMetalDriver {"),
		"metal_upload_reset.inc":   section("static void onix_metal_reset_uploads(", "\nint onix_metal_begin_frame"),
		"metal_upload.inc":         section("static int onix_metal_upload(", "\nint onix_metal_draw_sprites"),
	}
	for name, contents := range sections {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	runNativeUploadTest(t, compiler, []string{"-O2", "-Wall", "-Wextra", "-Werror", "-Wno-unused-function", "-isystem", strings.TrimSpace(string(include)), "-I.", "-I" + dir, "tests/metal_upload_test.m", "-l:libobjc.so.4"})
}

func TestGLESSurfaceDepth(t *testing.T) {
	compiler, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("requires a C compiler")
	}
	flags, err := exec.Command("pkg-config", "--cflags", "--libs", "egl", "glesv2").Output()
	if err != nil {
		t.Skip("requires EGL/GLES3 development libraries")
	}
	source, err := os.ReadFile("opengles.go")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(source), "static void onix_gl_blit_surface(")
	end := strings.Index(string(source), "\ntypedef struct {\n\tGLint camera_position;")
	if start < 0 || end <= start {
		t.Fatal("missing production GLES frame wrappers")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gles_frame_wrappers.inc"), source[start:end], 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-std=c11", "-O2", "-Wall", "-Wextra", "-Werror", "-Wno-unused-function", "-I" + dir, "tests/gles_lazy_test.c"}
	runNativeUploadTest(t, compiler, append(args, strings.Fields(string(flags))...))
}
