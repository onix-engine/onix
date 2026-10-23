package render

import (
	"bytes"
	"testing"
	"unsafe"
)

func packSpriteInstances(dst []byte, instances []Instance2D, draws []SpriteDraw) []byte {
	dst = dst[:0]
	for _, draw := range draws {
		dst = appendSpriteDrawInstances(dst, instances, draw)
	}
	return dst
}

func TestSpritePackingPreservesMixedLayoutsAndSource(t *testing.T) {
	instances := make([]Instance2D, 5)
	for index := range instances {
		words := unsafe.Slice((*float32)(unsafe.Pointer(&instances[index])), spriteCustomInstanceBytes/4)
		for component := range words {
			words[component] = float32(index*100 + component)
		}
	}
	original := append([]Instance2D(nil), instances...)
	shader := NewHandle[Material2DShaderResource](1, 1)
	draws := []SpriteDraw{
		{FirstInstance: 1, InstanceCount: 2},
		{Shader: shader, FirstInstance: 4, InstanceCount: 1},
		{FirstInstance: 0, InstanceCount: 1},
		{Shader: shader, FirstInstance: 2, InstanceCount: 1},
	}
	data := packSpriteInstances(nil, instances, draws)
	if len(data) != 892 {
		t.Fatalf("packed length = %d, want 892 including custom index padding", len(data))
	}
	offset := uint64(0)
	for _, draw := range draws {
		offset = spriteInstanceOffset(offset, draw)
		for index := uint32(0); index < draw.InstanceCount; index++ {
			source := &instances[draw.FirstInstance+index]
			raw := unsafe.Slice((*byte)(unsafe.Pointer(source)), spriteCustomInstanceBytes)
			if draw.Shader.Valid() {
				if !bytes.Equal(data[offset:offset+132], raw) {
					t.Fatal("custom shader input changed")
				}
			} else if !bytes.Equal(data[offset:offset+64], raw[:64]) || !bytes.Equal(data[offset+64:offset+100], raw[96:132]) {
				t.Fatal("built-in model, UV, tint, clipping or texture slot changed")
			}
			offset += uint64(spriteInstanceStride(draw.Shader.Valid()))
		}
	}
	for index := range instances {
		if instances[index] != original[index] {
			t.Fatal("packing changed borrowed instances")
		}
	}
	if allocations := testing.AllocsPerRun(100, func() { data = packSpriteInstances(data, instances, draws) }); allocations != 0 {
		t.Fatalf("warm sprite packing allocated %g times", allocations)
	}
}

func TestBuiltinSpritePackingUses100BytesPerInstance(t *testing.T) {
	instances := make([]Instance2D, 1024)
	data := packSpriteInstances(nil, instances, []SpriteDraw{{InstanceCount: uint32(len(instances))}})
	if len(data) != 100*len(instances) || spriteCustomInstanceBytes != 132 {
		t.Fatalf("compact bytes = %d, full stride = %d", len(data), spriteCustomInstanceBytes)
	}
	for _, custom := range []bool{false, true} {
		for location := 2; location <= 11; location++ {
			size := 16
			if location < 6 {
				size = 12
			}
			if location == 11 {
				size = 4
			}
			if spriteAttributeOffset(location, custom)+size > spriteInstanceStride(custom) {
				t.Fatalf("attribute %d exceeds stride (custom=%v)", location, custom)
			}
		}
	}
}

func BenchmarkSpriteInstancePacking(b *testing.B) {
	instances := make([]Instance2D, 4096)
	draws := []SpriteDraw{{InstanceCount: uint32(len(instances))}}
	data := packSpriteInstances(nil, instances, draws)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		data = packSpriteInstances(data, instances, draws)
	}
}
