package render

import "testing"

func TestTextureFormatSRGBVariantKeepsBlocksAndSwapsSampling(t *testing.T) {
	for _, testCase := range []struct {
		format TextureFormat
		srgb   bool
		want   TextureFormat
	}{
		{TextureFormatBC1, true, TextureFormatBC1SRGB},
		{TextureFormatBC1SRGB, false, TextureFormatBC1},
		{TextureFormatBC3, true, TextureFormatBC3SRGB},
		{TextureFormatBC3SRGB, false, TextureFormatBC3},
		{TextureFormatBC7, true, TextureFormatBC7SRGB},
		{TextureFormatBC7SRGB, false, TextureFormatBC7},
		{TextureFormatASTC4x4, true, TextureFormatASTC4x4SRGB},
		{TextureFormatASTC4x4SRGB, false, TextureFormatASTC4x4},
		{TextureFormatRGBA8, true, TextureFormatSRGBA8},
		{TextureFormatSRGBA8, false, TextureFormatRGBA8},
		// Already the requested interpretation.
		{TextureFormatBC1, false, TextureFormatBC1},
		{TextureFormatBC1SRGB, true, TextureFormatBC1SRGB},
		// Data and single-channel formats have no colour interpretation.
		{TextureFormatR8, true, TextureFormatR8},
		{TextureFormatBC4, true, TextureFormatBC4},
		{TextureFormatBC5, true, TextureFormatBC5},
		{TextureFormat(200), true, TextureFormat(200)},
	} {
		if got := testCase.format.SRGBVariant(testCase.srgb); got != testCase.want {
			t.Fatalf("%s.SRGBVariant(%v) = %s, want %s", testCase.format.Name(), testCase.srgb, got.Name(), testCase.want.Name())
		}
	}
}

// The two variants have to describe the same bytes, otherwise a payload packed
// for one sampler could not be handed to the other.
func TestTextureFormatSRGBVariantKeepsBlockLayout(t *testing.T) {
	for _, format := range []TextureFormat{TextureFormatBC1, TextureFormatBC3, TextureFormatBC7, TextureFormatASTC4x4} {
		variant := format.SRGBVariant(true)
		if variant.Block() != format.Block() {
			t.Fatalf("%s and %s disagree on block layout", format.Name(), variant.Name())
		}
		if format.SRGB() == variant.SRGB() {
			t.Fatalf("%s and %s sample the same way", format.Name(), variant.Name())
		}
	}
}

func TestTextureFormatChainSizeCoversFullMipChain(t *testing.T) {
	// 8x8 BC1: 4 blocks of 8 bytes at level 0, then 1 block per level down to 1x1.
	if got, want := TextureFormatBC1.ChainSize(8, 8, MipLevels(8, 8)), 4*8+8+8+8; got != want {
		t.Fatalf("8x8 bc1 chain = %d, want %d", got, want)
	}
	// A partial block still occupies a whole block.
	if got, want := TextureFormatBC1.LevelSize(1, 1), 8; got != want {
		t.Fatalf("1x1 bc1 level = %d, want %d", got, want)
	}
	// Levels beyond the chain must not inflate the size.
	if got, want := TextureFormatBC1.ChainSize(8, 8, 99), TextureFormatBC1.ChainSize(8, 8, MipLevels(8, 8)); got != want {
		t.Fatalf("clamped chain = %d, want %d", got, want)
	}
}
