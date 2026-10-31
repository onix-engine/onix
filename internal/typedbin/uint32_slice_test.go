package typedbin

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"
)

func TestUint32ResourceBlocksKeepVarintFormat(t *testing.T) {
	values := []uint32{0, 127, 128, 65535, ^uint32(0)}
	encoded, err := Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	want := binary.AppendUvarint(nil, uint64(len(values)+1))
	for _, value := range values {
		want = binary.AppendUvarint(want, uint64(value))
	}
	if !bytes.Equal(encoded[headerSize:], want) {
		t.Fatal("uint32 slice changed the existing varint format")
	}
	decoded, err := Unmarshal[[]uint32](encoded)
	if err != nil || !reflect.DeepEqual(decoded, values) {
		t.Fatalf("uint32 slice round trip: %v, %v", decoded, err)
	}
	array := [4]uint32{0, 127, 128, ^uint32(0)}
	arrayData, err := Marshal(array)
	if err != nil {
		t.Fatal(err)
	}
	decodedArray, err := Unmarshal[[4]uint32](arrayData)
	if err != nil || decodedArray != array {
		t.Fatalf("uint32 array round trip: %v, %v", decodedArray, err)
	}
	if _, err := Unmarshal[[]uint32](encoded[:len(encoded)-1]); err == nil {
		t.Fatal("truncated uint32 slice decoded successfully")
	}
	overflow, err := Marshal([]uint32{1})
	if err != nil {
		t.Fatal(err)
	}
	overflow = append(overflow[:headerSize+1], binary.AppendUvarint(nil, 1<<32)...)
	if _, err := Unmarshal[[]uint32](overflow); err == nil {
		t.Fatal("overflowing uint32 slice decoded successfully")
	}
}

func BenchmarkUint32ResourceSlice(b *testing.B) {
	values := make([]uint32, 1<<16)
	for i := range values {
		values[i] = uint32(i)
	}
	data, err := Marshal(values)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := Unmarshal[[]uint32](data); err != nil {
			b.Fatal(err)
		}
	}
}
