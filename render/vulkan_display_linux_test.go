//go:build linux && !android && cgo && (!onix_drm || onix_drm_vulkan)

package render

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestVulkanDRMDisplayLifecycle(t *testing.T) {
	compiler, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("native DRM display test requires cc")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "vulkan_display_test")
	compile := exec.CommandContext(ctx, compiler, "-std=c11", "-O1", "-I.", "tests/vulkan_display_test.c", "-l:libvulkan.so.1", "-o", binary)
	if output, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("compile native DRM display test: %v\n%s", err, output)
	}
	if output, err := exec.CommandContext(ctx, binary).CombinedOutput(); err != nil {
		t.Fatalf("native DRM display test: %v\n%s", err, output)
	}
}
