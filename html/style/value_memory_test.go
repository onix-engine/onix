package style

import (
	"testing"
	"unsafe"
)

func TestComputedValueStorageBudget(t *testing.T) {
	if size := unsafe.Sizeof(Value{}); size > 48 {
		t.Fatalf("CSS value stores %d bytes; want at most 48", size)
	}
}

func TestComputedStyleStorageBudget(t *testing.T) {
	if size := unsafe.Sizeof(ComputedStyle{}); size > 5560 {
		t.Fatalf("computed style stores %d bytes; want at most 5560", size)
	}
	computed := defaultStyle(nil)
	if computed.motion != nil || computed.HasMotion() || computed.Animations() != nil || computed.TransitionFor(0) != (TransformTransition{}) {
		t.Fatal("static computed style retains motion data")
	}
}
