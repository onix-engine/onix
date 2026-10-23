//go:build onix_mobile_test

package camera

import (
	"runtime"
	"slices"
	"testing"
	"time"
)

func TestMobileCameraBridgeLifecycle(t *testing.T) {
	mobileState.Lock()
	mobileState.sequence = 0
	mobileState.command = 0
	mobileState.options = Options{}
	mobileState.opening = false
	mobileState.opened = false
	mobileState.result = nil
	mobileState.frame = nil
	mobileState.err = nil
	mobileState.Unlock()
	opened := make(chan *Camera, 1)
	failed := make(chan error, 1)
	go func() {
		device, err := Open(Options{Width: 640, Height: 480, FacingMode: "user"})
		if err != nil {
			failed <- err
			return
		}
		opened <- device
	}()
	waitForCameraCommand(t, NativeCommandOpen)
	if options := NativeOptions(); options.Width != 640 || options.Height != 480 || NativeFacingMode() != 2 {
		t.Fatalf("native camera options = %+v facing=%d", options, NativeFacingMode())
	}
	DispatchNativeEvent(NativeEventOpened)
	var device *Camera
	select {
	case device = <-opened:
	case err := <-failed:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("camera open did not complete")
	}
	pixels := []byte{255, 0, 0, 255, 0, 255, 0, 128}
	DispatchNativeFrame(pixels, 2, 1)
	frame, err := device.Frame()
	if err != nil {
		t.Fatal(err)
	}
	if frame.Bounds().Dx() != 2 || frame.Bounds().Dy() != 1 || !slices.Equal(frame.Pix, pixels) {
		t.Fatalf("camera frame bounds=%v pixels=%v", frame.Bounds(), frame.Pix)
	}
	if err := device.Close(); err != nil {
		t.Fatal(err)
	}
	waitForCameraCommand(t, NativeCommandClose)
}

func waitForCameraCommand(t *testing.T, command NativeCommand) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if NativeCommand(NativeCommandValue()&0xff) == command {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("camera command %d was not issued", command)
}
