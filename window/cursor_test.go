package window

import (
	"testing"

	"github.com/onix-engine/onix/input"
)

func TestCursorOnlyCallsPlatformOnChange(t *testing.T) {
	calls := 0
	var applied input.Cursor
	support := cursorSupport{applyCursor: func(cursor input.Cursor) { calls++; applied = cursor }}
	support.SetCursor(input.CursorDefault)
	support.SetCursor(input.CursorText)
	if calls != 1 || applied != input.CursorText {
		t.Fatal("text cursor was not applied exactly once")
	}
	if allocs := testing.AllocsPerRun(100, func() { support.SetCursor(input.CursorText) }); allocs != 0 {
		t.Fatalf("idle cursor allocated %g times", allocs)
	}
	if calls != 1 {
		t.Fatal("idle cursor called platform")
	}
	support.SetCursor(input.CursorDefault)
	if calls != 2 || applied != input.CursorDefault {
		t.Fatal("default cursor was not restored")
	}
}
