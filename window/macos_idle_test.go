//go:build darwin && cgo && !ios

package window

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/onix-engine/onix/input"
)

func TestMacOSMouseDownScopesWindowDragEvent(t *testing.T) {
	c := &Context{events: new(macEventBuffer)}
	event := &c.events[0]
	event.kind = 6 // ONIX_MAC_EVENT_MOUSE_DOWN
	called := false
	c.OnMouseDown(func(x, y float64, button input.MouseButton) {
		called = true
		if c.mouseDownEvent != event {
			t.Fatal("window drag did not receive the dispatched press")
		}
		// Even without a native window, cgo validates the retained press before
		// the native function can return. Context must never be part of it.
		c.StartWindowDrag()
	})
	c.handleEvent(event)
	if !called || c.mouseDownEvent != nil {
		t.Fatal("mouse-down event escaped its callback")
	}
	if allocs := testing.AllocsPerRun(100, func() { c.handleEvent(event) }); allocs != 0 {
		t.Fatalf("mouse-down event allocated %g times", allocs)
	}
	c.OnMouseDown(nil)
	c.handleEvent(event)
}

func TestMacOSRedrawResumesFramePacing(t *testing.T) {
	c := &Context{framePacer: newFramePacer(), events: new(macEventBuffer)}
	c.framePacer.setTargetFPS(1)
	now := time.Now()
	if !c.framePacer.ready(now) || c.framePacer.ready(now.Add(time.Millisecond)) {
		t.Fatal("frame pacing did not defer the next frame")
	}
	event := &c.events[0]
	event.kind = 15 // ONIX_MAC_EVENT_REDRAW
	c.handleEvent(event)
	if !c.framePacer.ready(now.Add(time.Millisecond)) {
		t.Fatal("restored window still waits for the next paced frame")
	}
	if allocs := testing.AllocsPerRun(100, func() { c.handleEvent(event) }); allocs != 0 {
		t.Fatalf("redraw event allocated %g times", allocs)
	}
}

func TestMacOSNativeIdle(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "macos_idle_test")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := []string{"clang", "-std=gnu11", "-fblocks", "-Werror", "-Wno-deprecated-declarations",
		"tests/macos_idle_test.m", "macos_native.m", "-o", binary,
		"-framework", "AppKit", "-framework", "Foundation", "-framework", "QuartzCore", "-framework", "CoreGraphics"}
	if output, err := exec.CommandContext(ctx, "xcrun", args...).CombinedOutput(); err != nil {
		t.Fatalf("compile native macOS idle test: %v\n%s", err, output)
	}
	for _, name := range []string{"dispatch", "wake", "callback", "reopen", "drag", "keys-poll", "keys-wait"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if output, err := exec.CommandContext(ctx, binary, name).CombinedOutput(); err != nil {
				t.Fatalf("native macOS idle test: %v\n%s", err, output)
			}
		})
	}
}
