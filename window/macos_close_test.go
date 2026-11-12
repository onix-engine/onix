//go:build darwin && cgo && !ios

package window

import "testing"

// Polling snapshots the native close flag before the queued close event is
// dispatched. Vetoing that event must also clear the value LoopGame reads.
func TestMacOSCloseVetoClearsPolledFlag(t *testing.T) {
	c := &Context{events: new(macEventBuffer), nativeShouldClose: 1}
	event := &c.events[0]
	event.kind = 1 // ONIX_MAC_EVENT_CLOSE
	calls := 0
	c.OnCloseRequested(func() bool {
		calls++
		return calls > 1
	})
	c.handleEvent(event)
	if calls != 1 {
		t.Fatal("close event did not reach the application callback")
	}
	if c.shouldClose || c.nativeShouldClose != 0 {
		t.Fatal("vetoed close would still terminate LoopGame")
	}
	c.nativeShouldClose = 1
	c.handleEvent(event)
	if calls != 2 || !c.shouldClose {
		t.Fatal("a subsequent accepted close did not terminate the loop")
	}
}

func TestMacOSCloseWithoutHandlerStillExits(t *testing.T) {
	c := &Context{events: new(macEventBuffer), nativeShouldClose: 1}
	event := &c.events[0]
	event.kind = 1 // ONIX_MAC_EVENT_CLOSE
	c.handleEvent(event)
	if !c.shouldClose {
		t.Fatal("closing without a handler must terminate the loop")
	}
}
