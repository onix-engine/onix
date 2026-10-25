package render

import (
	"bytes"
	"fmt"
	"testing"
)

var texturePixelBenchmarkByte byte

func TestTexturePixelPackingAllocatesOnceAndReusesStorage(t *testing.T) {
	updates := make([]TextureUpdate, 64)
	var want []byte
	for index := range updates {
		updates[index].Pixels = bytes.Repeat([]byte{byte(index)}, 1024+index)
		want = append(want, updates[index].Pixels...)
	}
	var buffer []byte
	if allocations := testing.AllocsPerRun(10, func() {
		buffer = packTexturePixels(nil, updates, len(want))
	}); allocations != 1 {
		t.Fatalf("cold packing allocated %g times, want 1", allocations)
	}
	if cap(buffer) != len(want) || !bytes.Equal(buffer, want) {
		t.Fatal("packing changed payload or over-reserved storage")
	}
	pointer := &buffer[0]
	if allocations := testing.AllocsPerRun(100, func() {
		buffer = packTexturePixels(buffer, updates, len(want))
	}); allocations != 0 {
		t.Fatalf("warm packing allocated %g times", allocations)
	}
	updates = updates[:2]
	updates[0].Pixels = []byte{31, 17, 9}
	updates[1].Pixels = []byte{5}
	buffer = packTexturePixels(buffer, updates, 4)
	if &buffer[0] != pointer || !bytes.Equal(buffer, []byte{31, 17, 9, 5}) {
		t.Fatal("smaller batch lost storage or copied stale data")
	}
	updates[0].Pixels[0] = 99
	if buffer[0] != 31 {
		t.Fatal("packed pixels alias the caller's data")
	}
	previousCapacity := cap(buffer)
	updates = updates[:1]
	updates[0].Pixels = make([]byte, previousCapacity+2)
	buffer = packTexturePixels(buffer, updates, previousCapacity+2)
	pointer = &buffer[0]
	updates[0].Pixels = append(updates[0].Pixels, 71)
	buffer = packTexturePixels(buffer, updates, previousCapacity+3)
	if &buffer[0] != pointer || buffer[len(buffer)-1] != 71 {
		t.Fatal("slightly larger batches failed to reuse growth headroom")
	}
}

func BenchmarkTexturePixelPacking(b *testing.B) {
	for _, count := range []int{4, 64} {
		updates := make([]TextureUpdate, count)
		for i := range updates {
			updates[i].Pixels = make([]byte, 64<<10)
		}
		for _, warm := range []bool{false, true} {
			b.Run(fmt.Sprintf("updates=%d/warm=%t", count, warm), func(b *testing.B) {
				var buffer []byte
				buffer = packTexturePixels(buffer, updates, count*(64<<10))
				b.ReportAllocs()
				b.SetBytes(int64(len(buffer)))
				b.ResetTimer()
				for range b.N {
					if !warm {
						buffer = nil
					}
					buffer = packTexturePixels(buffer, updates, count*(64<<10))
				}
				texturePixelBenchmarkByte = buffer[len(buffer)-1]
			})
		}
	}
}
