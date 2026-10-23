//go:build linux && cgo && !android

package window

import (
	"sync"
	"syscall"
	"testing"
)

func TestNativeWakePipeQueuesAndSurvivesConcurrentClose(t *testing.T) {
	control := loopControl{}
	pipe, err := control.openWakePipe()
	if err != nil {
		t.Fatal(err)
	}
	control.Wake()
	var signal [1]byte
	if count, err := syscall.Read(pipe.read, signal[:]); err != nil || count != 1 {
		t.Fatalf("wake bytes=%d err=%v", count, err)
	}
	if allocations := testing.AllocsPerRun(100, func() { control.Wake(); pipe.drain() }); allocations != 0 {
		t.Fatalf("wake allocations=%v", allocations)
	}
	var group sync.WaitGroup
	group.Go(func() {
		for range 1000 {
			control.Wake()
		}
	})
	control.closeWakePipe(pipe)
	group.Wait()
	control.Wake()
	control.closeWakePipe(pipe)
}
