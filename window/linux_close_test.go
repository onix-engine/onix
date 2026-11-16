//go:build linux && cgo && !android && !onix_drm

package window

import "testing"

func TestLinuxCloseVetoClearsPolledFlag(t *testing.T) {
	c := &Context{nativeShouldClose: 1}
	event := &c.events[0]
	event.kind = 1 // ONIX_EVENT_CLOSE
	calls := 0
	c.OnCloseRequested(func() bool {
		calls++
		return calls > 1
	})
	c.handleEvent(event)
	if calls != 1 || c.shouldClose || c.nativeShouldClose != 0 {
		t.Fatal("vetoed close would terminate the loop before asynchronous confirmation")
	}
	c.nativeShouldClose = 1
	c.handleEvent(event)
	if calls != 2 || !c.shouldClose {
		t.Fatal("subsequent approved close did not terminate the loop")
	}
}

func TestLinuxCloseWithoutHandlerStillExits(t *testing.T) {
	c := &Context{nativeShouldClose: 1}
	event := &c.events[0]
	event.kind = 1 // ONIX_EVENT_CLOSE
	c.handleEvent(event)
	if !c.shouldClose {
		t.Fatal("closing without a handler must terminate the loop")
	}
}
