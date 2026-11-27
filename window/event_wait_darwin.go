//go:build darwin && cgo && !ios

package window

/*
#include "macos_native.h"
*/
import "C"

import "time"

type eventWaiter struct{}

func (l *EventLoop) runFrameBatch() error {
	// AppKit drains its event pool before Go runs the frame and presents it.
	// Drain render and window callback temporaries before the idle wait.
	defer C.onix_macos_drain_pool(C.onix_macos_new_pool())
	return l.dispatchFrames()
}

func (*eventWaiter) wait(windows []loopWindow, timeout time.Duration) {
	for _, item := range windows {
		if C.onix_macos_events_pending(item.context.nativeWindow) != 0 {
			return
		}
	}
	// All NSWindows share AppKit's application event queue.
	C.onix_macos_wait(windows[0].context.nativeWindow, C.int(idleTimeoutMillis(timeout)))
}
