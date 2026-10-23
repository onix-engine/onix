package texcompress

import (
	"bytes"
	"math"
	"slices"
	"testing"

	"github.com/diiyw/onix/render"
)

// decodeBC1Block reconstructs a 4x4 block the way a sampler would, so tests can
// measure the quality the GPU will actually see.
func decodeBC1Block(dst, src []byte) {
	endpoint0 := uint16(src[0]) | uint16(src[1])<<8
	endpoint1 := uint16(src[2]) | uint16(src[3])<<8
	palette := bc1Palette(endpoint0, endpoint1)
	bits := uint32(src[4]) | uint32(src[5])<<8 | uint32(src[6])<<16 | uint32(src[7])<<24
	for index := range blockSize * blockSize {
		copy(dst[index*4:index*4+4], palette[bits>>(2*index)&3][:])
	}
}

func decodeBC4Block(dst, src []byte, channel int) {
	palette := bc4Palette(src[0], src[1])
	bits := uint64(0)
	for index := range 6 {
		bits |= uint64(src[2+index]) << (8 * index)
	}
	for index := range blockSize * blockSize {
		dst[index*4+channel] = palette[bits>>(3*index)&7]
	}
}

func decodeLevel(format render.TextureFormat, data []byte, width, height int) []byte {
	dst := make([]byte, width*height*4)
	blocksX, blocksY := blockCount(width, height)
	blockBytes := format.Block().Bytes
	var block [blockSize * blockSize * 4]byte
	offset := 0
	for blockY := range blocksY {
		for blockX := range blocksX {
			clear(block[:])
			switch format {
			case render.TextureFormatBC1, render.TextureFormatBC1SRGB:
				decodeBC1Block(block[:], data[offset:])
			case render.TextureFormatBC3, render.TextureFormatBC3SRGB:
				// Alpha block first, colour block second. Apply colour first:
				// the colour decoder writes all four channels, so alpha has to
				// be written after it.
				decodeBC1Block(block[:], data[offset+bc4BlockBytes:])
				decodeBC4Block(block[:], data[offset:], 3)
			case render.TextureFormatBC4:
				decodeBC4Block(block[:], data[offset:], 0)
			case render.TextureFormatBC5:
				decodeBC4Block(block[:], data[offset:], 0)
				decodeBC4Block(block[:], data[offset+bc4BlockBytes:], 1)
			}
			for row := range blockSize {
				y := blockY*blockSize + row
				if y >= height {
					break
				}
				for column := range blockSize {
					x := blockX*blockSize + column
					if x >= width {
						break
					}
					// Both sides must be bounded: an unbounded source slice
					// makes copy spill the rest of the block over neighbouring
					// texels.
					target := (y*width + x) * 4
					source := (row*blockSize + column) * 4
					copy(dst[target:target+4], block[source:source+4])
				}
			}
			offset += blockBytes
		}
	}
	return dst
}

// gradientPixels builds a smooth two-axis ramp, the shape a mip level of a real
// colour texture takes.
func gradientPixels(width, height int) []byte {
	pixels := make([]byte, width*height*4)
	spanX, spanY := max(width-1, 1), max(height-1, 1)
	for y := range height {
		for x := range width {
			offset := (y*width + x) * 4
			pixels[offset] = uint8(x * 255 / spanX)
			pixels[offset+1] = uint8(y * 255 / spanY)
			pixels[offset+2] = uint8((x + y) * 255 / (spanX + spanY))
			pixels[offset+3] = 255
		}
	}
	return pixels
}

// psnr reports the peak signal-to-noise ratio over the selected channels.
func psnr(reference, decoded []byte, channels []int) float64 {
	var squared float64
	for offset := 0; offset+3 < len(reference); offset += 4 {
		for _, channel := range channels {
			difference := float64(reference[offset+channel]) - float64(decoded[offset+channel])
			squared += difference * difference
		}
	}
	count := float64(len(reference) / 4 * len(channels))
	if squared == 0 {
		return math.Inf(1)
	}
	return 10 * math.Log10(255*255*count/squared)
}

func TestEncodeLevelSizesAndDeterminism(t *testing.T) {
	pixels := gradientPixels(64, 32)
	for _, format := range Formats() {
		first, err := EncodeLevel(format, pixels, 64, 32)
		if err != nil {
			t.Fatalf("encode %s: %v", format.Name(), err)
		}
		if len(first) != format.LevelSize(64, 32) {
			t.Fatalf("format %s encoded %d bytes, want %d", format.Name(), len(first), format.LevelSize(64, 32))
		}
		second, err := EncodeLevel(format, pixels, 64, 32)
		if err != nil {
			t.Fatalf("encode %s twice: %v", format.Name(), err)
		}
		if !bytes.Equal(first, second) {
			t.Fatalf("format %s is not deterministic", format.Name())
		}
	}
}

func TestEncodeLevelRejectsInvalidInput(t *testing.T) {
	if _, err := EncodeLevel(render.TextureFormatRGBA8, gradientPixels(4, 4), 4, 4); err == nil {
		t.Fatal("expected an error for a format without an encoder")
	}
	if _, err := EncodeLevel(render.TextureFormatBC1, make([]byte, 4), 4, 4); err == nil {
		t.Fatal("expected an error for a short payload")
	}
	if _, err := EncodeLevel(render.TextureFormatBC1, nil, 0, 4); err == nil {
		t.Fatal("expected an error for a zero dimension")
	}
}

func TestEncodeLevelHandlesPartialBlocks(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {5, 3}, {3, 5}, {7, 7}} {
		width, height := size[0], size[1]
		pixels := gradientPixels(width, height)
		for _, format := range Formats() {
			encoded, err := EncodeLevel(format, pixels, width, height)
			if err != nil {
				t.Fatalf("encode %s at %dx%d: %v", format.Name(), width, height, err)
			}
			if len(encoded) != format.LevelSize(width, height) {
				t.Fatalf("format %s at %dx%d encoded %d bytes, want %d",
					format.Name(), width, height, len(encoded), format.LevelSize(width, height))
			}
		}
	}
}

func TestEncodeLevelSolidColorRoundTrips(t *testing.T) {
	pixels := make([]byte, 4*4*4)
	for offset := 0; offset < len(pixels); offset += 4 {
		pixels[offset], pixels[offset+1], pixels[offset+2], pixels[offset+3] = 40, 90, 200, 255
	}
	for _, format := range []render.TextureFormat{
		render.TextureFormatBC1, render.TextureFormatBC1SRGB,
		render.TextureFormatBC3, render.TextureFormatBC3SRGB,
	} {
		encoded, err := EncodeLevel(format, pixels, 4, 4)
		if err != nil {
			t.Fatalf("encode %s: %v", format.Name(), err)
		}
		decoded := decodeLevel(format, encoded, 4, 4)
		// BC1 endpoints are 5-6-5, so a solid block lands within one
		// quantization step of the source.
		for _, channel := range []int{0, 1, 2, 3} {
			if difference := absDifference(pixels[channel], decoded[channel]); difference > 8 {
				t.Fatalf("format %s channel %d decoded %d, want about %d",
					format.Name(), channel, decoded[channel], pixels[channel])
			}
		}
	}
}

func TestEncodeLevelAlphaRoundTrips(t *testing.T) {
	pixels := make([]byte, 4*4*4)
	for index := range 16 {
		offset := index * 4
		pixels[offset], pixels[offset+1], pixels[offset+2] = 200, 40, 60
		pixels[offset+3] = uint8(index * 255 / 15)
	}
	encoded, err := EncodeLevel(render.TextureFormatBC3, pixels, 4, 4)
	if err != nil {
		t.Fatalf("encode bc3: %v", err)
	}
	decoded := decodeLevel(render.TextureFormatBC3, encoded, 4, 4)
	// Sixteen distinct alpha values over an eight-entry palette cannot be
	// represented exactly; 29.5 dB is the best this input allows, so the
	// threshold guards against regressions rather than asserting perfection.
	if measured := psnr(pixels, decoded, []int{3}); measured < 29 {
		t.Fatalf("bc3 alpha scored %.1f dB, want at least 29 dB", measured)
	}
}

func TestEncodeLevelAlphaIsExactForSolidBlocks(t *testing.T) {
	for _, alpha := range []uint8{0, 255} {
		pixels := make([]byte, 4*4*4)
		for offset := 0; offset < len(pixels); offset += 4 {
			pixels[offset], pixels[offset+1], pixels[offset+2], pixels[offset+3] = 90, 120, 160, alpha
		}
		encoded, err := EncodeLevel(render.TextureFormatBC3, pixels, 4, 4)
		if err != nil {
			t.Fatalf("encode bc3 at alpha %d: %v", alpha, err)
		}
		decoded := decodeLevel(render.TextureFormatBC3, encoded, 4, 4)
		if decoded[3] != alpha {
			t.Fatalf("bc3 solid alpha %d decoded as %d", alpha, decoded[3])
		}
	}
}

// The hardware reads a BC3 block as an alpha block followed by a colour block.
// Writing them the other way round still round-trips through a matching
// reference decoder, so assert the layout against the block encoders instead.
func TestEncodeLevelBC3PlacesAlphaBlockFirst(t *testing.T) {
	pixels := make([]byte, 4*4*4)
	for offset := 0; offset < len(pixels); offset += 4 {
		pixels[offset], pixels[offset+1], pixels[offset+2], pixels[offset+3] = 255, 0, 0, 96
	}
	encoded, err := EncodeLevel(render.TextureFormatBC3, pixels, 4, 4)
	if err != nil {
		t.Fatalf("encode bc3: %v", err)
	}
	if len(encoded) != bc3BlockBytes {
		t.Fatalf("bc3 block is %d bytes, want %d", len(encoded), bc3BlockBytes)
	}
	var scratch blockScratch
	gatherBlock(&scratch, pixels, 4*4, 0, 0, 4, 4)
	var alpha [bc4BlockBytes]byte
	encodeBC4(alpha[:], &scratch, 3)
	if !slices.Equal(encoded[:bc4BlockBytes], alpha[:]) {
		t.Fatalf("bc3 leading block is not the alpha block: %v, want %v", encoded[:bc4BlockBytes], alpha[:])
	}
	var colour [bc1BlockBytes]byte
	encodeBC1(colour[:], &scratch, true)
	if !slices.Equal(encoded[bc4BlockBytes:bc3BlockBytes], colour[:]) {
		t.Fatalf("bc3 trailing block is not the colour block: %v, want %v", encoded[bc4BlockBytes:], colour[:])
	}
}

func TestEncodeLevelBC5PlacesRedBlockFirst(t *testing.T) {
	pixels := make([]byte, 4*4*4)
	for offset := 0; offset < len(pixels); offset += 4 {
		pixels[offset], pixels[offset+1], pixels[offset+2], pixels[offset+3] = 255, 0, 0, 255
	}
	encoded, err := EncodeLevel(render.TextureFormatBC5, pixels, 4, 4)
	if err != nil {
		t.Fatalf("encode bc5: %v", err)
	}
	if len(encoded) != bc5BlockBytes {
		t.Fatalf("bc5 block is %d bytes, want %d", len(encoded), bc5BlockBytes)
	}
	var scratch blockScratch
	gatherBlock(&scratch, pixels, 4*4, 0, 0, 4, 4)
	var red, green [bc4BlockBytes]byte
	encodeBC4(red[:], &scratch, 0)
	encodeBC4(green[:], &scratch, 1)
	if !slices.Equal(encoded[:bc4BlockBytes], red[:]) {
		t.Fatalf("bc5 leading block is not the red plane: %v, want %v", encoded[:bc4BlockBytes], red[:])
	}
	if !slices.Equal(encoded[bc4BlockBytes:bc5BlockBytes], green[:]) {
		t.Fatalf("bc5 trailing block is not the green plane: %v, want %v", encoded[bc4BlockBytes:], green[:])
	}
}

func TestEncodeLevelQuality(t *testing.T) {
	pixels := gradientPixels(128, 128)
	thresholds := map[string]float64{
		"bc1":      38,
		"bc1-srgb": 38,
		"bc3":      38,
	}
	for _, format := range Formats() {
		if format == render.TextureFormatBC4 || format == render.TextureFormatBC5 {
			continue
		}
		threshold, ok := thresholds[format.Name()]
		if !ok {
			continue
		}
		encoded, err := EncodeLevel(format, pixels, 128, 128)
		if err != nil {
			t.Fatalf("encode %s: %v", format.Name(), err)
		}
		decoded := decodeLevel(format, encoded, 128, 128)
		measured := psnr(pixels, decoded, []int{0, 1, 2})
		t.Logf("format %s scored %.1f dB at 128x128", format.Name(), measured)
		if measured < threshold {
			t.Fatalf("format %s scored %.1f dB, want at least %.1f dB", format.Name(), measured, threshold)
		}
	}
}

func TestEncodeLevelSingleChannelQuality(t *testing.T) {
	width, height := 64, 64
	pixels := gradientPixels(width, height)
	for _, format := range []render.TextureFormat{render.TextureFormatBC4, render.TextureFormatBC5} {
		encoded, err := EncodeLevel(format, pixels, width, height)
		if err != nil {
			t.Fatalf("encode %s: %v", format.Name(), err)
		}
		decoded := decodeLevel(format, encoded, width, height)
		channels := []int{0}
		if format == render.TextureFormatBC5 {
			channels = []int{0, 1}
		}
		if measured := psnr(pixels, decoded, channels); measured < 38 {
			t.Fatalf("format %s scored %.1f dB, want at least 38 dB", format.Name(), measured)
		}
	}
}

func TestEncodeChainMatchesDeclaredSize(t *testing.T) {
	width, height := 32, 16
	pixels := gradientPixels(width, height)
	for _, format := range Formats() {
		data, levels, err := EncodeChain(format, pixels, width, height)
		if err != nil {
			t.Fatalf("encode chain %s: %v", format.Name(), err)
		}
		if want := render.MipLevels(width, height); levels != want {
			t.Fatalf("format %s produced %d levels, want %d", format.Name(), levels, want)
		}
		if len(data) != format.ChainSize(width, height, levels) {
			t.Fatalf("format %s produced %d bytes, want %d",
				format.Name(), len(data), format.ChainSize(width, height, levels))
		}
	}
}

func TestDownsampleHalvesAndPreservesConstants(t *testing.T) {
	width, height := 8, 4
	pixels := make([]byte, width*height*4)
	for offset := 0; offset < len(pixels); offset += 4 {
		pixels[offset], pixels[offset+1], pixels[offset+2], pixels[offset+3] = 17, 200, 99, 128
	}
	reduced := downsample(pixels, width, height, false)
	if len(reduced) != 4*2*4 {
		t.Fatalf("downsample produced %d bytes, want %d", len(reduced), 4*2*4)
	}
	for offset := 0; offset < len(reduced); offset += 4 {
		if reduced[offset] != 17 || reduced[offset+1] != 200 || reduced[offset+2] != 99 || reduced[offset+3] != 128 {
			t.Fatalf("constant level changed at offset %d: %v", offset, reduced[offset:offset+4])
		}
	}
}

func TestDownsampleFiltersSRGBInLinearLight(t *testing.T) {
	pixels := []byte{
		0, 0, 0, 255,
		0, 0, 0, 255,
		255, 255, 255, 255,
		255, 255, 255, 255,
	}
	linear := downsample(pixels, 2, 2, false)
	srgb := downsample(pixels, 2, 2, true)
	// Averaging encoded values would land on 127; averaging in linear light is
	// the whole point of filtering an sRGB level correctly.
	if linear[0] < 125 || linear[0] > 130 {
		t.Fatalf("linear average was %d, want about 127", linear[0])
	}
	if srgb[0] < 180 || srgb[0] > 195 {
		t.Fatalf("sRGB average was %d, want about 188", srgb[0])
	}
	if srgb[3] != 255 {
		t.Fatalf("alpha must stay linear, got %d", srgb[3])
	}
}

func TestOpaque(t *testing.T) {
	opaque := gradientPixels(4, 4)
	if !Opaque(opaque) {
		t.Fatal("gradient with alpha 255 reported as non-opaque")
	}
	opaque[7] = 254
	if Opaque(opaque) {
		t.Fatal("pixels with alpha 254 reported as opaque")
	}
}

func absDifference(left, right uint8) int {
	if left > right {
		return int(left) - int(right)
	}
	return int(right) - int(left)
}
