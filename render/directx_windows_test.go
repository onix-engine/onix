//go:build windows && cgo && !onix_metal && !onix_vulkan && !onix_opengl && !onix_webgpu && !onix_webgl

package render

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDirectXNativeRendering(t *testing.T) {
	for _, compiler := range []string{"gcc", "g++"} {
		if _, err := exec.LookPath(compiler); err != nil {
			t.Skip("DirectX pixel regression requires MinGW-w64 gcc and g++")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	root := t.TempDir()
	composition := filepath.Join(root, "composition.o")
	binary := filepath.Join(root, "directx_test.exe")
	commands := []*exec.Cmd{
		exec.CommandContext(ctx, "g++", "-std=c++17", "-O2", "-c", "directx_composition.cc", "-o", composition),
		exec.CommandContext(ctx, "gcc", "-std=c11", "-O2", "-I.", "tests/directx_native_test.c", composition, "-ld3d11", "-ldxgi", "-ld3dcompiler", "-ldcomp", "-lole32", "-luuid", "-lstdc++", "-o", binary),
		exec.CommandContext(ctx, binary),
	}
	// The native harness creates a hidden window and checks offscreen pixels.
	for _, command := range commands {
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", command.Path, err, output)
		}
		t.Log(string(output))
	}
}
