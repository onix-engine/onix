//go:build windows && cgo

package window

import "time"

type eventWaiter struct{}

func (*eventWaiter) wait(_ []loopWindow, timeout time.Duration) {
	msgWaitForMultipleObjectsEx(0, 0, uint32(idleTimeoutMillis(timeout)), 0x04ff, 0x0004)
}
