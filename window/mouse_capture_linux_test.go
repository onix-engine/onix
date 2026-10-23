//go:build linux && cgo && !android && !onix_drm

package window

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeMouseCapture(t *testing.T) {
	for _, tool := range []string{"cc", "pkg-config", "xvfb-run"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("native mouse capture test requires %s", tool)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	flags, err := exec.CommandContext(ctx, "pkg-config", "--cflags", "--libs", "x11", "xext", "xrender", "xi", "xtst", "wayland-client", "wayland-cursor", "xkbcommon").CombinedOutput()
	if err != nil {
		t.Skipf("native mouse capture test requires XTest development files: %s", flags)
	}
	binary := filepath.Join(t.TempDir(), "mouse_capture_test")
	args := []string{"-std=gnu11", "-O1", "tests/mouse_capture_test.c", "linux_native.c", "xdg-shell-protocol.c", "xdg-decoration-unstable-v1-protocol.c", "text-input-unstable-v3-protocol.c", "relative-pointer-unstable-v1-protocol.c", "pointer-constraints-unstable-v1-protocol.c", "-o", binary}
	args = append(args, strings.Fields(string(flags))...)
	if output, err := exec.CommandContext(ctx, "cc", args...).CombinedOutput(); err != nil {
		t.Fatalf("compile native mouse capture test: %v\n%s", err, output)
	}
	if output, err := exec.CommandContext(ctx, "xvfb-run", "-a", binary).CombinedOutput(); err != nil {
		t.Fatalf("native mouse capture test: %v\n%s", err, output)
	} else {
		t.Log(strings.TrimSpace(string(output)))
	}
}
