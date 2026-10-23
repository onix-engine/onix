//go:build (windows || linux || android) && cgo && (!(linux && onix_drm) || onix_drm_vulkan)

package render

import "testing"

// The packer and the Vulkan uploader each compute the block layout of a mip
// chain, in Go and in C respectively. Nothing links them, so this pins them
// together: a disagreement would upload every level after the first at the
// wrong offset, which corrupts the chain progressively instead of failing.
func TestVulkanNativeChainSizeMatchesGoLayout(t *testing.T) {
	sizes := [...]struct{ width, height int }{
		{1, 1}, {1, 4}, {2, 3}, {3, 1}, {4, 4}, {5, 5}, {13, 7},
		{16, 16}, {64, 32}, {128, 128}, {300, 200}, {1024, 512},
	}
	for _, size := range sizes {
		full := MipLevels(size.width, size.height)
		if got := nativeTextureMipLevels(size.width, size.height); got != full {
			t.Fatalf("native mip levels for %dx%d = %d, want %d", size.width, size.height, got, full)
		}
		for format := TextureFormatRGBA8; format <= TextureFormatASTC4x4SRGB; format++ {
			for levels := 1; levels <= full; levels++ {
				got := nativeTextureChainSize(format, size.width, size.height, levels)
				if want := format.ChainSize(size.width, size.height, levels); got != want {
					t.Fatalf("native chain size for %s %dx%d with %d levels = %d, want %d",
						format.Name(), size.width, size.height, levels, got, want)
				}
			}
		}
	}
}

// An oversized level count is clamped rather than trusted, because the count
// comes from a descriptor that callers build.
func TestVulkanNativeChainSizeClampsExcessLevels(t *testing.T) {
	full := TextureFormatBC3.ChainSize(32, 32, MipLevels(32, 32))
	for _, levels := range []int{0, 1, MipLevels(32, 32), 99} {
		got := nativeTextureChainSize(TextureFormatBC3, 32, 32, levels)
		if want := TextureFormatBC3.ChainSize(32, 32, max(levels, 1)); got != want {
			t.Fatalf("native chain size for bc3 32x32 with %d levels = %d, want %d", levels, got, want)
		}
	}
	if got := nativeTextureChainSize(TextureFormatBC3, 32, 32, 99); got != full {
		t.Fatalf("99 levels covered %d bytes, want the full chain of %d", got, full)
	}
}
