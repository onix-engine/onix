package typedbin

import (
	"fmt"
	"testing"
)

func BenchmarkCodecScratch(b *testing.B) {
	items := make([]recursiveValue, 256)
	for index := range items {
		items[index] = recursiveValue{Name: "resource", Count: index + 1, Enabled: true, Values: []uint16{1, 255, 65535}}
	}
	lookup := make(map[string]recursiveValue, 128)
	for index := range 128 {
		lookup[fmt.Sprintf("resource-%03d", index)] = recursiveValue{
			Name: "asset", Count: index + 1,
			Lookup: map[string][]int{"second": nil, "first": {1, -2, 3}},
		}
	}
	benchmarkCodec(b, "StructSlice", items)
	benchmarkCodec(b, "NestedMaps", lookup)
}

func benchmarkCodec[T any](b *testing.B, name string, value T) {
	b.Helper()
	data, err := Marshal(value)
	if err != nil {
		b.Fatal(err)
	}
	b.Run(name+"/Marshal", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := Marshal(value); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run(name+"/Unmarshal", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := Unmarshal[T](data); err != nil {
				b.Fatal(err)
			}
		}
	})
}
