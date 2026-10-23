//go:build darwin && cgo

package render

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestMetalLazyResources(t *testing.T) {
	testMetalNative(t, "metal_lazy_test")
}

func TestMetalMeshVariants(t *testing.T) {
	testMetalNative(t, "metal_mesh_variants_test")
}

func testMetalNative(t *testing.T, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), name)
	command := exec.CommandContext(ctx, "xcrun", "clang", "-fno-objc-arc", "-O2", "tests/"+name+".m", "-framework", "Foundation", "-framework", "Metal", "-framework", "QuartzCore", "-framework", "CoreVideo", "-o", binary)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("compile Metal regression: %v\n%s", err, output)
	}
	command = exec.CommandContext(ctx, binary)
	command.Env = append(os.Environ(), "MTL_DEBUG_LAYER=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Metal regression: %v\n%s", err, output)
	} else {
		t.Log(string(output))
	}
}
