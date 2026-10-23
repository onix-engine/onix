package project

import "testing"

func TestNineSliceValidateRequiresStretchableCenter(t *testing.T) {
	valid := NineSlice{Left: 6, Right: 5, Top: 4, Bottom: 3}
	if err := valid.Validate(30, 24); err != nil {
		t.Fatal(err)
	}
	if err := (NineSlice{Left: 15, Right: 15}).Validate(30, 24); err == nil {
		t.Fatal("expected horizontal slice without a center to fail")
	}
	if err := (NineSlice{Left: -1}).Validate(30, 24); err == nil {
		t.Fatal("expected negative inset to fail")
	}
}
