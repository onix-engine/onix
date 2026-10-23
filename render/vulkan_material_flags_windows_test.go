//go:build windows && cgo

package render

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestVulkanNativeMaterialFlags(t *testing.T) {
	compiler, err := exec.LookPath("gcc")
	if err != nil {
		t.Skip("native Vulkan pixel test requires gcc")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "vulkan_material_flags_test.exe")
	compile := exec.CommandContext(ctx, compiler, "-std=c11", "-O2", "-I.", "tests/vulkan_material_flags_test.c", "-ld3d11", "-ldxgi", "-lole32", "-luuid", "-lm", "-o", binary)
	if output, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("compile material flags test: %v\n%s", err, output)
	}
	// The native harness uses a hidden window and reads an offscreen target.
	if output, err := exec.CommandContext(ctx, binary).CombinedOutput(); err != nil {
		t.Fatalf("material flags pixel test: %v\n%s", err, output)
	} else {
		t.Log(string(output))
	}
}
