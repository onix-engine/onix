package texcompress

import (
	"bytes"
	"testing"

	"github.com/diiyw/onix/render"
)

func TestEncodeASTCLevelWritesStandardConstantColorBlock(t *testing.T) {
	pixels := make([]byte, 4*4*4)
	for offset := 0; offset < len(pixels); offset += 4 {
		pixels[offset], pixels[offset+1], pixels[offset+2], pixels[offset+3] = 12, 34, 56, 78
	}
	encoded, err := EncodeLevel(render.TextureFormatASTC4x4, pixels, 4, 4)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0xFC, 0xFD, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0x0C, 0x0C, 0x22, 0x22, 0x38, 0x38, 0x4E, 0x4E,
	}
	if !bytes.Equal(encoded, want) {
		t.Fatalf("ASTC block = %x, want %x", encoded, want)
	}
}

func TestEncodeASTCChainMatchesGenericEncoder(t *testing.T) {
	pixels := gradientPixels(7, 5)
	chain, levels, err := EncodeASTCChain(pixels, 7, 5, true)
	if err != nil {
		t.Fatal(err)
	}
	generic, genericLevels, err := EncodeChain(render.TextureFormatASTC4x4SRGB, pixels, 7, 5)
	if err != nil {
		t.Fatal(err)
	}
	if levels != genericLevels || !bytes.Equal(chain, generic) {
		t.Fatal("ASTC specialized and generic chain encoders differ")
	}
}

func TestDownsampleIntoSupportsMipBufferReuse(t *testing.T) {
	for _, size := range [][2]int{{7, 5}, {5, 3}, {3, 7}, {2, 9}, {1, 8}} {
		width, height := size[0], size[1]
		source := gradientPixels(width, height)
		buffer := make([]byte, max(1, width/2)*max(1, height/2)*4)
		for width > 1 || height > 1 {
			nextWidth, nextHeight := max(1, width/2), max(1, height/2)
			want := downsample(source, width, height, true)
			downsampleInto(buffer[:nextWidth*nextHeight*4], source, width, height, true)
			if !bytes.Equal(buffer[:len(want)], want) {
				t.Fatalf("downsample %dx%d mismatch", width, height)
			}
			source = buffer[:len(want)]
			width, height = nextWidth, nextHeight
		}
	}
}
