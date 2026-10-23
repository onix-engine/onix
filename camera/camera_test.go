package camera

import "testing"

func TestCameraUnavailablePlatform(t *testing.T) {
	if Available() {
		t.Skip("camera is available on this platform")
	}
	if _, err := Open(Options{}); err != ErrUnavailable {
		t.Fatalf("Open error = %v, want %v", err, ErrUnavailable)
	}
	var camera *Camera
	if err := camera.Close(); err != nil {
		t.Fatal(err)
	}
}
