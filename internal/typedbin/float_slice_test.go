package typedbin

import (
	"bytes"
	"math"
	"reflect"
	"testing"
)

func TestFloat32SliceRetainsExactBits(t *testing.T) {
	values := []float32{0, math.Float32frombits(0x80000000), 1.25, math.Float32frombits(0x7fc00001), float32(math.Inf(1))}
	data, err := Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Unmarshal[[]float32](data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(values) {
		t.Fatalf("decoded %d values, want %d", len(got), len(values))
	}
	for i := range got {
		if math.Float32bits(got[i]) != math.Float32bits(values[i]) {
			t.Fatalf("value %d changed bits: %08x → %08x", i, math.Float32bits(values[i]), math.Float32bits(got[i]))
		}
	}
	if _, err := Unmarshal[[]float32](data[:len(data)-1]); err == nil {
		t.Fatal("truncated float slice decoded successfully")
	}
}

func TestFloat32ArrayResourceRetainsExactBits(t *testing.T) {
	type vertex struct {
		Position [3]float32
		Tangent  [4]float32
	}
	value := []vertex{{Position: [3]float32{1, math.Float32frombits(0x80000000), math.Float32frombits(0x7fc00001)}, Tangent: [4]float32{0, 0, 1, -1}}}
	data, err := Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Unmarshal[[]vertex](data)
	if err != nil {
		t.Fatal(err)
	}
	for i := range value[0].Position {
		if math.Float32bits(got[0].Position[i]) != math.Float32bits(value[0].Position[i]) {
			t.Fatalf("position element %d changed bits", i)
		}
	}
	if got[0].Tangent != value[0].Tangent {
		t.Fatal("tangent changed")
	}
	if _, err := Unmarshal[[]vertex](data[:len(data)-1]); err == nil {
		t.Fatal("truncated float array decoded successfully")
	}
}

func TestFloatArrayStructSliceKeepsONIXBIN1Layout(t *testing.T) {
	type vertex struct {
		Position [3]float32
		Normal   [3]float32
		UV       [2]float32
		Tangent  [4]float32
	}
	values := []vertex{{Position: [3]float32{1, 2, 3}, UV: [2]float32{.5, 1}}, {Normal: [3]float32{0, 0, 1}, Tangent: [4]float32{1, 0, 0, -1}}, {Position: [3]float32{math.Float32frombits(0x80000000)}}}
	encoded, err := Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	legacyPayload := valueEncoder{}
	for _, value := range values {
		if err := legacyPayload.encode(reflect.ValueOf(value), reflect.TypeOf(value)); err != nil {
			t.Fatal(err)
		}
	}
	// The slice length occupies one byte after the header.
	if !bytes.Equal(encoded[headerSize+1:], legacyPayload.data) {
		t.Fatal("fixed float struct encoding changed the existing resource format")
	}
	decoded, err := Unmarshal[[]vertex](encoded)
	if err != nil || !reflect.DeepEqual(decoded, values) {
		t.Fatalf("vertex resource round trip: %v, %v", decoded, err)
	}
	encoded[headerSize+1] |= 0x80
	if _, err := Unmarshal[[]vertex](encoded); err == nil {
		t.Fatal("invalid vertex bitmap decoded successfully")
	}
}

func BenchmarkFloatStructResourceSlice(b *testing.B) {
	type vertex struct {
		Position [3]float32
		Normal   [3]float32
		UV       [2]float32
		Tangent  [4]float32
	}
	vertices := make([]vertex, 1<<16)
	for i := range vertices {
		vertices[i] = vertex{Position: [3]float32{float32(i), 1, 2}, Normal: [3]float32{0, 0, 1}, UV: [2]float32{.5, .5}, Tangent: [4]float32{1, 0, 0, 1}}
	}
	data, err := Marshal(vertices)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := Unmarshal[[]vertex](data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFloat32ResourceSlice(b *testing.B) {
	values := make([]float32, 1<<16)
	for i := range values {
		values[i] = float32(i) * .125
	}
	data, err := Marshal(values)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("Marshal", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := Marshal(values); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("Unmarshal", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := Unmarshal[[]float32](data); err != nil {
				b.Fatal(err)
			}
		}
	})
}
