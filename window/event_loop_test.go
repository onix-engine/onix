//go:build cgo && (windows || (darwin && !ios) || (linux && !android && !onix_drm))

package window

import (
	"testing"
	"time"
)

func TestEventLoopWaitsForEarliestWindowDeadline(t *testing.T) {
	first, second := &Context{}, &Context{}
	first.SetIdleDeadline(func() (time.Time, bool) { return time.Time{}, true })
	deadline := time.Now().Add(time.Minute)
	second.SetIdleDeadline(func() (time.Time, bool) { return deadline, true })
	var loop EventLoop
	loop.Add(first, nil, nil)
	loop.Add(second, nil, nil)
	if wait := loop.waitDuration(); wait <= 0 || wait > time.Minute {
		t.Fatalf("deadline wait=%v", wait)
	}
	second.SetIdleDeadline(func() (time.Time, bool) { return time.Time{}, true })
	if wait := loop.waitDuration(); wait != -1 {
		t.Fatalf("idle windows must sleep until an event: %v", wait)
	}
	second.SetIdleDeadline(func() (time.Time, bool) { return time.Time{}, false })
	if wait := loop.waitDuration(); wait != 0 {
		t.Fatalf("new window must start immediately: %v", wait)
	}
	second.framePacer.nextFrame = time.Now().Add(time.Second)
	if wait := loop.waitDuration(); wait <= 0 || wait > time.Second {
		t.Fatalf("active window ignored frame pacing: %v", wait)
	}
	if allocations := testing.AllocsPerRun(100, func() { loop.waitDuration() }); allocations != 0 {
		t.Fatalf("idle scheduling allocated %g times", allocations)
	}
	first.shouldClose = true
	if wait := loop.waitDuration(); wait != 0 {
		t.Fatal("closing a window during a frame must not wait for another event")
	}
	loop.close(0)
	if len(loop.windows) != 1 || loop.windows[0].context != second {
		t.Fatal("closing one window removed its sibling")
	}
	loop.Close()
	if len(loop.windows) != 0 {
		t.Fatal("application shutdown retained a window")
	}
}
