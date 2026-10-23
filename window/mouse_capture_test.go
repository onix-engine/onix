package window

import "testing"

func TestCapturedMouseDelta(t *testing.T) {
	var mouse mouseCaptureSupport
	if mouse.SetMouseCaptured(true) {
		t.Fatal("unsupported capture reported success")
	}
	mouse.addMouseDelta(100, 200)
	mouse.updateMouseCapture(true)
	mouse.addMouseDelta(4000, -2500)
	mouse.addMouseDelta(12.5, -1.25)
	if x, y := mouse.TakeMouseDelta(); x != 4012.5 || y != -2501.25 {
		t.Fatalf("captured motion was clipped or lost: %g, %g", x, y)
	}
	if x, y := mouse.TakeMouseDelta(); x != 0 || y != 0 {
		t.Fatal("motion was consumed twice")
	}
	mouse.addMouseDelta(100, 200)
	mouse.updateMouseCapture(false)
	mouse.addMouseDelta(300, 400)
	mouse.updateMouseCapture(true)
	if x, y := mouse.TakeMouseDelta(); x != 0 || y != 0 {
		t.Fatal("capture transition retained stale motion")
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		mouse.addMouseDelta(20, -10)
		mouse.TakeMouseDelta()
	}); allocs != 0 {
		t.Fatalf("captured motion allocated: %g", allocs)
	}
}
