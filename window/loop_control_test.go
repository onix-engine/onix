package window

import (
	"testing"
	"time"
)

func TestIdleDeadlineAndTimeout(t *testing.T) {
	control := loopControl{}
	if _, idle := control.idleWait(); idle {
		t.Fatal("default loop idles")
	}
	control.SetIdleDeadline(func() (time.Time, bool) { return time.Time{}, true })
	if wait, idle := control.idleWait(); !idle || wait != -1 {
		t.Fatal("indefinite idle missing")
	}
	control.SetIdleDeadline(func() (time.Time, bool) { return time.Now().Add(-time.Second), true })
	if wait, idle := control.idleWait(); !idle || wait != 0 {
		t.Fatal("past deadline blocked")
	}
	for _, entry := range []struct {
		wait         time.Duration
		milliseconds int
	}{{-1, -1}, {0, 0}, {time.Nanosecond, 1}, {time.Millisecond, 1}, {time.Millisecond + 1, 2}, {time.Duration(1<<63 - 1), 2147483647}} {
		if got := idleTimeoutMillis(entry.wait); got != entry.milliseconds {
			t.Fatalf("timeout(%v)=%d", entry.wait, got)
		}
	}
}
