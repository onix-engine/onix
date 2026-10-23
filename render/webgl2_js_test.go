//go:build js && wasm && wechat

package render

import (
	"bytes"
	"syscall/js"
	"testing"
)

func TestCopyTextureToJSReusesUploadBuffer(t *testing.T) {
	var storage js.Value
	firstData := []byte{1, 2, 3, 4}
	first := copyBytesToJS(&storage, firstData)
	if first.Get("byteLength").Int() < len(firstData) {
		t.Fatalf("first upload buffer is too small: %d", first.Get("byteLength").Int())
	}
	copied := make([]byte, len(firstData))
	js.CopyBytesToGo(copied, first)
	if !bytes.Equal(copied, firstData) {
		t.Fatalf("first upload data = %v, want %v", copied, firstData)
	}

	secondData := []byte{9, 8}
	second := copyBytesToJS(&storage, secondData)
	if !second.Equal(first) {
		t.Fatal("upload buffer was replaced without growing")
	}
	js.CopyBytesToGo(copied[:len(secondData)], second)
	if !bytes.Equal(copied[:len(secondData)], secondData) {
		t.Fatalf("second upload data = %v, want %v", copied[:len(secondData)], secondData)
	}

	larger := make([]byte, first.Get("byteLength").Int()+1)
	third := copyBytesToJS(&storage, larger)
	if third.Equal(first) {
		t.Fatal("upload buffer was not replaced when growing")
	}
}

func TestWebGLMeshHelperBindsChangedSlotsAndPreservesInstanceOffsets(t *testing.T) {
	binds, draws, attributes := 0, 0, 0
	lastOffset := 0
	active := js.FuncOf(func(_ js.Value, args []js.Value) any { return nil })
	defer active.Release()
	bind := js.FuncOf(func(_ js.Value, args []js.Value) any { binds++; return nil })
	defer bind.Release()
	attribute := js.FuncOf(func(_ js.Value, args []js.Value) any { attributes++; lastOffset = args[5].Int(); return nil })
	defer attribute.Release()
	draw := js.FuncOf(func(_ js.Value, args []js.Value) any {
		draws++
		if args[1].Int() != 3 || args[3].Int() != 8 || args[4].Int() != 2 {
			t.Fatal("draw range changed")
		}
		return nil
	})
	defer draw.Release()
	d := webGL2Driver{gl: webGLObject("activeTexture", active, "bindTexture", bind, "vertexAttribPointer", attribute, "drawElementsInstanced", draw)}
	texture := d.insertTexture(webGLTexture{value: webGLObject()})
	other := d.insertTexture(webGLTexture{value: webGLObject()})
	d.ensureMeshDrawHelper()
	d.meshDrawReset.Invoke()
	for slot := range 16 {
		d.meshCommand[slot] = texture.ID()
	}
	d.meshCommand[16], d.meshCommand[17], d.meshCommand[18], d.meshCommand[19] = 4, 2, 3, 2
	for repeat := range 3 {
		if repeat == 2 {
			d.meshCommand[7] = other.ID()
		}
		js.CopyBytesToJS(d.meshCommandBytes, sliceBytes(d.meshCommand[:]))
		d.meshDrawHelper.Invoke(d.meshCommandData)
	}
	if binds != 17 || draws != 3 || attributes != 36 || lastOffset != 4*192+11*16 {
		t.Fatalf("binds=%d draws=%d attributes=%d lastOffset=%d", binds, draws, attributes, lastOffset)
	}
	d.meshDrawReset.Invoke()
	js.CopyBytesToJS(d.meshCommandBytes, sliceBytes(d.meshCommand[:]))
	d.meshDrawHelper.Invoke(d.meshCommandData)
	if binds != 33 {
		t.Fatal("new batch did not invalidate texture binding state")
	}
}

func webGLObject(values ...any) js.Value {
	object := js.Global().Get("Object").New()
	for index := 0; index < len(values); index += 2 {
		object.Set(values[index].(string), values[index+1])
	}
	return object
}
