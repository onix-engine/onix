//go:build linux && !android && cgo

package window

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDRMOutputRestoration(t *testing.T) {
	compiler, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("native DRM output test requires cc")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "drm_native_test")
	compile := exec.CommandContext(ctx, compiler, "-std=gnu11", "-O1", "tests/drm_native_test.c", "-ldl", "-o", binary)
	if output, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("compile native DRM output test: %v\n%s", err, output)
	}
	if output, err := exec.CommandContext(ctx, binary).CombinedOutput(); err != nil {
		t.Fatalf("native DRM output test: %v\n%s", err, output)
	}
}
