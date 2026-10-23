//go:build linux && !android && cgo && !onix_drm

package render

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVulkanNativeSteadyStateAllocations(t *testing.T) {
	compiler, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("native allocation test requires cc")
	}
	xvfb, err := exec.LookPath("Xvfb")
	if err != nil {
		t.Skip("native allocation test requires Xvfb")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "vulkan_native_test")
	compile := exec.CommandContext(ctx, compiler, "-std=c11", "-O2", "-I.", "tests/vulkan_native_test.c", "-l:libvulkan.so.1", "-lX11", "-lm", "-o", binary)
	if output, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("compile native allocation test: %v\n%s", err, output)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	server := exec.CommandContext(ctx, xvfb, "-displayfd", "3", "-screen", "0", "128x128x24", "-nolisten", "tcp")
	server.ExtraFiles = []*os.File{writer}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = server.Process.Kill()
		_ = server.Wait()
	}()
	writer.Close()
	display, err := bufio.NewReader(reader).ReadString('\n')
	if err != nil {
		t.Fatalf("start Xvfb: %v", err)
	}
	command := exec.CommandContext(ctx, binary)
	command.Env = append(os.Environ(), "DISPLAY=:"+strings.TrimSpace(display), "LP_NUM_THREADS=2")
	if _, err := os.Stat("/usr/share/vulkan/icd.d/lvp_icd.json"); err == nil {
		command.Env = append(command.Env, "VK_DRIVER_FILES=/usr/share/vulkan/icd.d/lvp_icd.json")
	}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("native allocation test: %v\n%s", err, output)
	} else {
		t.Log(strings.TrimSpace(string(output)))
	}
}
