package editor

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestCachedProjectBuildLayoutUsesProjectDistCache(t *testing.T) {
	root := t.TempDir()
	writeBuildTestFile(t, filepath.Join(root, "go.mod"), "module example.com/game\n")
	layout, err := newCachedProjectBuildLayout(root, filepath.Join(root, "apps", "demo"))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "apps", "demo", "dist", "cache")
	if layout.BaseDir != want {
		t.Fatalf("cached project directory = %q, want %q", layout.BaseDir, want)
	}
}

func TestRunOutputCaptureKeepsBoundedTail(t *testing.T) {
	var capture runOutputCapture
	prefix := bytes.Repeat([]byte{'a'}, maxRunOutputBytes+17)
	suffix := []byte("fatal: final failure\n")
	if _, err := capture.Write(prefix); err != nil {
		t.Fatal(err)
	}
	if _, err := capture.Write(suffix); err != nil {
		t.Fatal(err)
	}

	output := capture.String()
	if !strings.HasPrefix(output, truncatedRunOutputPrefix) {
		t.Fatalf("captured output is missing truncation marker: %q", output[:min(len(output), 64)])
	}
	retained := strings.TrimPrefix(output, truncatedRunOutputPrefix)
	if len(retained) != maxRunOutputBytes {
		t.Fatalf("retained output size = %d, want %d", len(retained), maxRunOutputBytes)
	}
	if !strings.HasSuffix(retained, string(suffix)) {
		t.Fatalf("captured output does not retain final failure: %q", retained[len(retained)-len(suffix):])
	}
	if len(capture.data) != maxRunOutputBytes {
		t.Fatalf("capture buffer size = %d, want %d", len(capture.data), maxRunOutputBytes)
	}
}

func TestRunOutputCaptureWriteDoesNotAllocateAfterInitialization(t *testing.T) {
	var capture runOutputCapture
	_, _ = capture.Write([]byte("initialize"))
	payload := []byte("runtime output\n")
	if allocations := testing.AllocsPerRun(100, func() {
		_, _ = capture.Write(payload)
	}); allocations != 0 {
		t.Fatalf("steady-state capture allocations = %v, want 0", allocations)
	}
}
