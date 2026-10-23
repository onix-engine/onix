package editor

import (
	"bytes"
	"testing"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/graphics/codec/ontex"
	"github.com/diiyw/onix/render"
)

func TestEncodeASTCChainPureGo(t *testing.T) {
	const width, height = 7, 5
	pixels := make([]byte, width*height*4)
	for offset := 0; offset < len(pixels); offset += 4 {
		pixels[offset], pixels[offset+1], pixels[offset+2], pixels[offset+3] = byte(offset), byte(offset/2), 180, 255
	}
	payload, levels, err := encodeASTCChain(pixels, width, height, BuildModeDebug, false)
	if err != nil {
		t.Fatal(err)
	}
	if levels != render.MipLevels(width, height) {
		t.Fatalf("ASTC levels = %d, want %d", levels, render.MipLevels(width, height))
	}
	if want := render.TextureFormatASTC4x4.ChainSize(width, height, levels); len(payload) != want {
		t.Fatalf("ASTC chain = %d bytes, want %d", len(payload), want)
	}
	if !bytes.Equal(payload[:8], []byte{0xFC, 0xFD, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}) {
		t.Fatalf("ASTC block does not use the standard void-extent header: %x", payload[:8])
	}
}

func TestPackEmbeddedASTCSRGBProfile(t *testing.T) {
	original := encodeASTCTexture
	defer func() { encodeASTCTexture = original }()
	var gotSRGB bool
	encodeASTCTexture = func(_ []byte, width, height int, _ BuildMode, srgb bool) ([]byte, int, error) {
		gotSRGB = srgb
		levels := render.MipLevels(width, height)
		return make([]byte, render.TextureFormatASTC4x4.ChainSize(width, height, levels)), levels, nil
	}
	data := textureSourcePNG(t, 2, 2)
	packed, err := packEmbeddedTextureSource(data, "image/png", TextureTargetASTC, BuildModeRelease, graphics.TextureOptions{SRGB: true})
	if err != nil {
		t.Fatal(err)
	}
	image, err := ontex.Unmarshal(packed)
	if err != nil {
		t.Fatal(err)
	}
	if !gotSRGB || image.Format != render.TextureFormatASTC4x4SRGB {
		t.Fatalf("ASTC sRGB profile = %v, format = %s", gotSRGB, image.Format.Name())
	}
}

func TestTextureTargetForBuildPlatform(t *testing.T) {
	for _, testCase := range []struct {
		platform BuildPlatform
		want     TextureTarget
	}{
		{BuildPlatformWindows, TextureTargetBC},
		{BuildPlatformLinux, TextureTargetBC},
		{BuildPlatformMacOS, TextureTargetBC},
		{BuildPlatformAndroid, TextureTargetASTC},
		{BuildPlatformIOS, TextureTargetASTC},
		{BuildPlatformWeb, TextureTargetASTC},
		{BuildPlatformWeChat, TextureTargetASTC},
	} {
		if got := textureTargetForBuildPlatform(testCase.platform); got != testCase.want {
			t.Errorf("texture target for %s = %s, want %s", testCase.platform, got, testCase.want)
		}
	}
}
