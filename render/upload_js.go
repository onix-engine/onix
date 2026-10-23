//go:build js && wasm

package render

import (
	"syscall/js"
	"unsafe"
)

func sliceBytes[T any](values []T) []byte {
	if len(values) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&values[0])), len(values)*int(unsafe.Sizeof(values[0])))
}

func nextBufferCapacity(required int) int {
	capacity := 1024
	for capacity < required {
		capacity *= 2
	}
	return capacity
}

// Return the retained array itself; uploads supply their exact source length
// so smaller payloads need neither a new array nor a subarray view.
func copyBytesToJS(storage *js.Value, values []byte) js.Value {
	if !storage.Truthy() || storage.Get("byteLength").Int() < len(values) {
		*storage = js.Global().Get("Uint8Array").New(nextBufferCapacity(len(values)))
	}
	if len(values) != 0 {
		js.CopyBytesToJS(*storage, values)
	}
	return *storage
}
