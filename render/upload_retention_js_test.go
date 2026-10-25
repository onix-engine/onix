//go:build js && wasm

package render

import (
	"bytes"
	"syscall/js"
	"testing"
)

func TestJSUploadRetentionPeakShrinkIdleAndRegrow(t *testing.T) {
	var storage js.Value
	var retention scratchRetention
	large := bytes.Repeat([]byte{37}, 1<<20)
	small := []byte{9, 7, 5, 3}
	copyBytesToJS(&storage, &retention, large)
	original := storage
	for range bufferTrimDelay * 2 {
		copyBytesToJS(&storage, &retention, large)
		copyBytesToJS(&storage, &retention, small)
		trimJSUpload(&storage, &retention)
		if !storage.Equal(original) {
			t.Fatal("last small upload hid the frame peak")
		}
	}
	for range bufferTrimDelay {
		copyBytesToJS(&storage, &retention, small)
		trimJSUpload(&storage, &retention)
	}
	if got := storage.Get("byteLength").Int(); got != 64<<10 {
		t.Fatalf("low-load capacity = %d", got)
	}
	warm := storage
	for range bufferTrimDelay * 2 {
		copyBytesToJS(&storage, &retention, small)
		trimJSUpload(&storage, &retention)
		if !storage.Equal(warm) {
			t.Fatal("stable load recreated the JS array")
		}
	}
	copyBytesToJS(&storage, &retention, large)
	actual := make([]byte, len(large))
	js.CopyBytesToGo(actual, storage)
	if !bytes.Equal(actual, large) {
		t.Fatal("regrowth changed the upload payload")
	}
	trimJSUpload(&storage, &retention)
	for range bufferTrimDelay {
		trimJSUpload(&storage, &retention)
	}
	if storage.Truthy() {
		t.Fatal("idle upload retained its peak")
	}
	copyBytesToJS(&storage, &retention, small)
	js.CopyBytesToGo(actual[:len(small)], storage)
	if !bytes.Equal(actual[:len(small)], small) {
		t.Fatal("upload after release changed the payload")
	}
}
