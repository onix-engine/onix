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
