//go:build linux && cgo && !android && !onix_drm

package onix

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/onix-engine/onix/ecs"
)

func TestApplicationKeepsSiblingWindowsAlive(t *testing.T) {
	if os.Getenv("ONIX_TEST_MULTIPLE_WINDOWS") != "1" {
		tool, err := exec.LookPath("xvfb-run")
		if err != nil {
			t.Skip("multiple-window test requires xvfb-run")
		}
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, tool, "-a", os.Args[0], "-test.run=^TestApplicationKeepsSiblingWindowsAlive$", "-test.timeout=10s")
		command.WaitDelay = time.Second
		command.Env = append(os.Environ(), "ONIX_TEST_MULTIPLE_WINDOWS=1", "ONIX_LINUX_BACKEND=x11")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("multiple-window lifecycle: %v\n%s", err, output)
		}
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var application Application
	defer application.Close()
	var first, second *App
	var closed []string
	var firstFrames, secondFrames int
	pending := false
	first = NewApp()
	first.SetWindowSize(160, 120)
	first.SetRenderMode(RenderOnDemand)
	first.SetIdleEnabled(true)
	if err := first.AddSystem(StageUpdate, ecs.SystemConfig{Name: "first_window", System: func(*ecs.SystemContext) error {
		firstFrames++
		pending = second == nil
		if secondFrames > 0 {
			first.RequestClose()
		} else {
			first.WakeAt(time.Now().Add(10 * time.Millisecond))
		}
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if err := application.Add(first, func() { closed = append(closed, "first") }); err != nil {
		t.Fatal(err)
	}
	application.BetweenFrames = func() error {
		if !pending {
			return nil
		}
		pending = false
		second = NewApp()
		second.SetWindowSize(180, 140)
		second.SetRenderMode(RenderOnDemand)
		second.SetIdleEnabled(true)
		if second.Context == first.Context || second.Context.Device == first.Context.Device {
			t.Fatal("windows shared a render context or device")
		}
		if err := second.AddSystem(StageUpdate, ecs.SystemConfig{Name: "second_window", System: func(*ecs.SystemContext) error {
			secondFrames++
			if len(closed) == 1 {
				second.RequestClose()
			} else {
				second.WakeAt(time.Now().Add(10 * time.Millisecond))
			}
			return nil
		}}); err != nil {
			return err
		}
		return application.Add(second, func() { closed = append(closed, "second") })
	}
	if err := application.Run(); err != nil {
		t.Fatal(err)
	}
	if firstFrames < 2 || secondFrames < 2 || !slices.Equal(closed, []string{"first", "second"}) {
		t.Fatalf("frames=%d,%d closed=%v", firstFrames, secondFrames, closed)
	}
	application.Close()
	if len(closed) != 2 {
		t.Fatal("closed callbacks ran twice")
	}
}
