package typedbin

import (
	"bytes"
	"reflect"
	"testing"
)

func TestSkinNumericStructKeepsONIXBIN1Layout(t *testing.T) {
	type skin struct {
		Joints  [4]uint32
		Weights [4]float32
		Active  bool
	}
	values := []skin{{Joints: [4]uint32{0, 128, 65535, 0}, Weights: [4]float32{.25, .5, .25}, Active: true}, {Active: true}, {}}
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
	if !bytes.Equal(encoded[headerSize+1:], legacyPayload.data) {
		t.Fatal("skin numeric struct encoding changed the existing resource format")
	}
	decoded, err := Unmarshal[[]skin](encoded)
	if err != nil || !reflect.DeepEqual(decoded, values) {
		t.Fatalf("skin resource round trip: %v, %v", decoded, err)
	}
	active, err := Marshal([]skin{{Active: true}})
	if err != nil {
		t.Fatal(err)
	}
	active[len(active)-1] = 2
	if _, err := Unmarshal[[]skin](active); err == nil {
		t.Fatal("invalid skin boolean decoded successfully")
	}
}
