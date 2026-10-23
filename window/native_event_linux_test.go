//go:build linux && cgo && !android && !onix_drm

package window

import (
	"testing"
	"unsafe"

	"github.com/diiyw/onix/input"
)

func TestNativeMouseEventDoesNotAllocate(t *testing.T) {
	context := &Context{}
	event := &context.events[0]
	event.kind = 6 // ONIX_EVENT_MOUSE_MOVE
	var calls int
	context.onMouseMove = func(x, y float64) { calls++ }
	if allocations := testing.AllocsPerRun(1000, func() { context.handleEvent(event) }); allocations != 0 {
		t.Fatalf("mouse event allocations = %v, want 0", allocations)
	}
	if calls != 1001 {
		t.Fatalf("mouse callbacks = %d, want 1001", calls)
	}
}

func TestNativeEventTextSurvivesBufferReuse(t *testing.T) {
	context := &Context{}
	context.textInputSupport.config = input.TextInputConfig{Revision: 1, Active: true}
	var text string
	context.OnTextInput(func(value string) { text = value })
	event := &context.events[0]
	event.kind = 10 // ONIX_EVENT_TEXT_INPUT
	buffer := unsafe.Slice((*byte)(unsafe.Pointer(&event.text[0])), len(event.text))
	copy(buffer, "你好\x00")
	context.handleEvent(event)
	clear(buffer)
	if text != "你好" {
		t.Fatalf("text after event buffer reuse = %q", text)
	}
}

func TestNativeRepeatedCommandSurvivesBufferReuse(t *testing.T) {
	context := &Context{}
	context.textInputSupport.config = input.TextInputConfig{Revision: 1, Active: true}
	event := &context.events[0]
	event.kind = 12               // ONIX_EVENT_TEXT_COMMAND
	event.arg0, event.arg1 = 1, 3 // Backspace repeated three times.
	var calls int
	context.OnTextCommand(func(command input.TextCommand) {
		if command != input.TextCommandBackspace {
			t.Fatalf("command = %v, want backspace", command)
		}
		calls++
		event.arg0 = 0
	})
	context.handleEvent(event)
	if calls != 3 {
		t.Fatalf("command callbacks = %d, want 3", calls)
	}
}

func BenchmarkNativeMouseEvent(b *testing.B) {
	context := &Context{}
	event := &context.events[0]
	event.kind = 6 // ONIX_EVENT_MOUSE_MOVE
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		context.handleEvent(event)
	}
}
