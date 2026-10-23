package texcontainer

import (
	"encoding/binary"
	"testing"
)

func TestDimensionsReadsPackedHeader(t *testing.T) {
	header := make([]byte, HeaderSize+4)
	copy(header, Magic[:])
	binary.LittleEndian.PutUint32(header[12:], 320)
	binary.LittleEndian.PutUint32(header[16:], 180)

	if !Encoded(header) {
		t.Fatal("container signature was not recognized")
	}
	width, height, ok := Dimensions(header)
	if !ok || width != 320 || height != 180 {
		t.Fatalf("dimensions = %dx%d ok=%v, want 320x180", width, height, ok)
	}
}

func TestDimensionsRejectsShortOrForeignPayloads(t *testing.T) {
	if _, _, ok := Dimensions(nil); ok {
		t.Fatal("empty payload was read as a container")
	}
	if _, _, ok := Dimensions([]byte("ONIXTEX1")); ok {
		t.Fatal("signature without a header was read as a container")
	}
	foreign := make([]byte, HeaderSize)
	copy(foreign, []byte("ONIXTEX2"))
	if _, _, ok := Dimensions(foreign); ok {
		t.Fatal("a different magic was read as a container")
	}
}
