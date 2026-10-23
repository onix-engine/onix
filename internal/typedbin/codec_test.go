package typedbin

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

type recursiveValue struct {
	Name     string
	Count    int
	Enabled  bool
	Ratio    float32
	Optional *string
	Values   []uint16
	Empty    []byte
	Lookup   map[string][]int
	Children []recursiveValue
}

func TestRoundTrip(t *testing.T) {
	optional := "present"
	want := recursiveValue{
		Name: "root", Count: -42, Enabled: true, Ratio: 0.75, Optional: &optional,
		Values: []uint16{1, 255, 65535}, Empty: []byte{},
		Lookup:   map[string][]int{"second": nil, "first": {1, -2, 3}},
		Children: []recursiveValue{{Name: "child"}},
	}
	data, err := Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Unmarshal[recursiveValue](data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded value = %#v, want %#v", got, want)
	}
	if got.Empty == nil {
		t.Fatal("non-nil empty slice became nil")
	}
}

func TestMarshalIsDeterministic(t *testing.T) {
	first := recursiveValue{Lookup: map[string][]int{"z": {3}, "a": {1}, "m": {2}}}
	second := recursiveValue{Lookup: map[string][]int{"m": {2}, "z": {3}, "a": {1}}}
	left, err := Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	right, err := Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(left, right) {
		t.Fatal("equivalent maps produced different encodings")
	}
}

func TestUnmarshalRejectsWrongSchemaAndCorruption(t *testing.T) {
	data, err := Marshal(recursiveValue{Name: "value"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Unmarshal[string](data); err == nil {
		t.Fatal("wrong target schema was accepted")
	}
	for _, corrupt := range [][]byte{nil, data[:len(data)-1], append(append([]byte(nil), data...), 1)} {
		if _, err := Unmarshal[recursiveValue](corrupt); err == nil {
			t.Fatalf("corrupt data of length %d was accepted", len(corrupt))
		}
	}
}

func TestMarshalRejectsUnsupportedAndCyclicTypes(t *testing.T) {
	type unsupported struct{ Value complex64 }
	if _, err := Marshal(unsupported{}); err == nil {
		t.Fatal("unsupported field type was accepted")
	}
	type node struct{ Next *node }
	value := &node{}
	value.Next = value
	if _, err := Marshal(value); err == nil {
		t.Fatal("cyclic value was accepted")
	}
}

func TestMapScratchKeepsSparseEntriesIndependent(t *testing.T) {
	type key struct {
		Name string
		Rank int
	}
	optional := "present"
	want := map[key]recursiveValue{
		{Name: "named"}: {Name: "populated", Optional: &optional, Values: []uint16{1, 2}, Lookup: map[string][]int{"one": {1}}},
		{Rank: 3}:       {},
		{}:              {Empty: []byte{}, Lookup: map[string][]int{}},
	}
	for range 50 {
		data, err := Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Unmarshal[map[key]recursiveValue](data)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("map scratch leaked a preceding entry: %#v", got)
		}
	}
}

func TestMapScratchReusePreservesResults(t *testing.T) {
	want := map[string]map[string]int{
		strings.Repeat("a", 8192): {"one": 1, "two": 2},
		"b":                       {"three": 3},
	}
	for worker := range 8 {
		t.Run(string(rune('A'+worker)), func(t *testing.T) {
			t.Parallel()
			data, err := Marshal(want)
			if err != nil {
				t.Fatal(err)
			}
			snapshot := bytes.Clone(data)
			for range 20 {
				other, err := Marshal(map[string]string{strings.Repeat("z", 16384): "other"})
				if err != nil {
					t.Fatal(err)
				}
				clear(other)
				again, err := Marshal(want)
				if err != nil || !bytes.Equal(again, snapshot) || !bytes.Equal(data, snapshot) {
					t.Fatalf("encoding changed across scratch reuse: %v", err)
				}
			}
			got, err := Unmarshal[map[string]map[string]int](data)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("nested map round trip failed: %v", err)
			}
		})
	}
}

func TestMapKeyErrorDoesNotPoisonScratch(t *testing.T) {
	type key struct {
		Next *key
	}
	cyclic := new(key)
	cyclic.Next = cyclic
	if _, err := Marshal(map[*key]int{cyclic: 1}); err == nil {
		t.Fatal("cyclic map key was accepted")
	}
	leaf := new(key)
	data, err := Marshal(map[*key]int{leaf: 2})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Unmarshal[map[*key]int](data)
	if err != nil || len(got) != 1 {
		t.Fatalf("scratch reuse after map key failure: %v", err)
	}
	for item, value := range got {
		if item == nil || item.Next != nil || value != 2 {
			t.Fatalf("unexpected decoded map entry: %+v = %d", item, value)
		}
	}
}

func FuzzUnmarshal(f *testing.F) {
	seed, err := Marshal(recursiveValue{Name: "seed", Values: []uint16{1, 2, 3}})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seed)
	f.Add([]byte("not typed binary"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = Unmarshal[recursiveValue](data)
	})
}
