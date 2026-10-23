package render

import (
	"math"
	"testing"
	"unsafe"
)

func TestShaderDescriptorValidation(t *testing.T) {
	valid := []ShaderDescriptor{
		{VulkanVertexSPIRV: make([]byte, 4), VulkanFragmentSPIRV: make([]byte, 4)},
		{VulkanFragmentSPIRV: make([]byte, 4)},
		{MetalSource: "metal"},
		{WebGL2VertexSource: "vertex", WebGL2FragmentSource: "fragment"},
		{WebGL2FragmentSource: "fragment"},
		{WebGPUVertexSource: "vertex", WebGPUFragmentSource: "fragment"},
		{WebGPUFragmentSource: "fragment"},
	}
	for _, descriptor := range valid {
		if err := descriptor.Validate(); err != nil {
			t.Fatalf("valid descriptor rejected: %v", err)
		}
	}
	invalid := []ShaderDescriptor{
		{},
		{VulkanVertexSPIRV: make([]byte, 3), VulkanFragmentSPIRV: make([]byte, 4)},
	}
	for _, descriptor := range invalid {
		if err := descriptor.Validate(); err == nil {
			t.Fatalf("invalid descriptor accepted: %+v", descriptor)
		}
	}
}

func TestTextureDescriptorValidationSupportsR8(t *testing.T) {
	if err := (TextureDescriptor{Width: 4, Height: 2, Format: TextureFormatR8}).Validate(make([]byte, 8)); err != nil {
		t.Fatalf("R8 texture descriptor rejected: %v", err)
	}
	if err := (TextureDescriptor{Width: 4, Height: 2}).Validate(make([]byte, 32)); err != nil {
		t.Fatalf("RGBA8 texture descriptor rejected: %v", err)
	}
	if err := (TextureDescriptor{Width: 4, Height: 2, Format: TextureFormatR8}).Validate(make([]byte, 32)); err == nil {
		t.Fatal("R8 descriptor accepted RGBA-sized pixel data")
	}
}

func TestInstanceABIWidths(t *testing.T) {
	if got := unsafe.Sizeof(Instance2D{}); got != 132 {
		t.Fatalf("sprite instance size = %d, want 132", got)
	}
	if got := unsafe.Offsetof(Instance2D{}.TextureSlot); got != 128 {
		t.Fatalf("sprite texture slot offset = %d, want 128", got)
	}
	if got := unsafe.Sizeof(ShapeInstance2D{}); got != 100 {
		t.Fatalf("shape instance size = %d, want 100", got)
	}
	if got := unsafe.Sizeof(Vertex3D{}); got != 48 {
		t.Fatalf("3D vertex size = %d, want 48", got)
	}
	if got := unsafe.Sizeof(Instance3D{}); got != 192 {
		t.Fatalf("3D instance size = %d, want 192", got)
	}
}

func TestInstancePickingIDPreservesAllBits(t *testing.T) {
	values := [...]uint32{0, 1, 0x7fffffff, 0x80000000, 0xffffffff}
	for _, value := range values {
		var instance Instance3D
		instance.SetPickingID(value)
		if got := instance.PickingID(); got != value {
			t.Fatalf("picking ID %#x round-tripped as %#x", value, got)
		}
		if math.Float32bits(instance.Custom[3][3]) != value {
			t.Fatalf("stored picking bits %#x, want %#x", math.Float32bits(instance.Custom[3][3]), value)
		}
	}
}

// A backend walks the payload with Level, so the levels have to tile it without
// gaps or overlap. If they do not, a prebuilt chain uploads shifted blocks.
func TestTextureDescriptorLevelsTileThePayload(t *testing.T) {
	for _, testCase := range []struct {
		format TextureFormat
		width  int
		height int
	}{
		{TextureFormatRGBA8, 4, 4},
		{TextureFormatR8, 16, 8},
		{TextureFormatBC1, 8, 8},
		{TextureFormatBC3, 64, 32},
		// Odd sizes force the block rounding at every level.
		{TextureFormatBC1, 5, 5},
		{TextureFormatBC3, 13, 7},
		{TextureFormatBC1, 1, 1},
		{TextureFormatBC7, 3, 1},
	} {
		descriptor := TextureDescriptor{
			Width:  testCase.width,
			Height: testCase.height,
			Format: testCase.format,
			Levels: MipLevels(testCase.width, testCase.height),
		}
		next := 0
		for level, got := range descriptor.MipChain {
			if want := descriptor.Level(level); got != want {
				t.Fatalf("mip iterator level %d = %+v, want %+v", level, got, want)
			}
			if got.Offset != next {
				t.Fatalf("%s %dx%d level %d starts at %d, want %d",
					testCase.format.Name(), testCase.width, testCase.height, level, got.Offset, next)
			}
			next += got.Bytes
		}
		if want := testCase.format.ChainSize(testCase.width, testCase.height, descriptor.MipLevels()); next != want {
			t.Fatalf("%s %dx%d levels cover %d bytes, want %d",
				testCase.format.Name(), testCase.width, testCase.height, next, want)
		}
	}
}

func TestTextureMipChainStopsEarlyWithoutAllocating(t *testing.T) {
	descriptor := TextureDescriptor{Width: 13, Height: 7, Format: TextureFormatASTC4x4, Levels: 4}
	visited := 0
	for level := range descriptor.MipChain {
		visited++
		if level == 1 {
			break
		}
	}
	if visited != 2 {
		t.Fatalf("early stop visited %d levels", visited)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		for _, mip := range descriptor.MipChain {
			visited += mip.Bytes
		}
	}); allocations != 0 {
		t.Fatalf("mip iteration allocates %v times", allocations)
	}
}

func BenchmarkTextureMipChain(b *testing.B) {
	descriptor := TextureDescriptor{Width: 16384, Height: 8192, Format: TextureFormatBC7, Levels: 15}
	b.Run("random-access", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			total := 0
			for level := range descriptor.MipLevels() {
				total += descriptor.Level(level).Bytes
			}
			if total == 0 {
				b.Fatal("empty chain")
			}
		}
	})
	b.Run("sequential", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			total := 0
			for _, mip := range descriptor.MipChain {
				total += mip.Bytes
			}
			if total == 0 {
				b.Fatal("empty chain")
			}
		}
	})
}

func TestTextureDescriptorLevelHalvesDimensionsDownToOne(t *testing.T) {
	descriptor := TextureDescriptor{Width: 5, Height: 5, Format: TextureFormatBC1, Levels: MipLevels(5, 5)}
	want := [...]TextureLevel{
		{Width: 5, Height: 5, Offset: 0, Bytes: 32},
		{Width: 2, Height: 2, Offset: 32, Bytes: 8},
		{Width: 1, Height: 1, Offset: 40, Bytes: 8},
	}
	if descriptor.MipLevels() != len(want) {
		t.Fatalf("5x5 has %d levels, want %d", descriptor.MipLevels(), len(want))
	}
	for level, expected := range want {
		if got := descriptor.Level(level); got != expected {
			t.Fatalf("5x5 bc1 level %d = %+v, want %+v", level, got, expected)
		}
	}
	// Levels past the chain clamp to the base level instead of reading garbage.
	if got := descriptor.Level(-1); got != want[0] {
		t.Fatalf("negative level = %+v, want %+v", got, want[0])
	}
}
