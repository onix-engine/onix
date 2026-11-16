//go:build linux && cgo && !android && !onix_drm

package window

/*
#include "linux_native.h"
*/
import "C"

import (
	"time"

	"golang.org/x/sys/unix"
)

type eventWaiter struct{ descriptors []unix.PollFd }

func (w *eventWaiter) wait(windows []loopWindow, timeout time.Duration) {
	w.descriptors = w.descriptors[:0]
	for _, item := range windows {
		c := item.context
		fd := int(C.onix_window_event_fd(c.nativeWindow))
		if fd == -2 {
			return
		}
		w.descriptors = append(w.descriptors,
			unix.PollFd{Fd: int32(fd), Events: unix.POLLIN},
			unix.PollFd{Fd: int32(c.wakePipe.read), Events: unix.POLLIN})
	}
	_, _ = unix.Poll(w.descriptors, idleTimeoutMillis(timeout))
	for _, item := range windows {
		item.context.wakePipe.drain()
	}
}
