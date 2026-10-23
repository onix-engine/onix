package editor

import (
	"bytes"
	"fmt"
	"image"
	stdgif "image/gif"
	stdjpeg "image/jpeg"
	"path"
	"strings"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/graphics/codec/ontex"
	onixpng "github.com/diiyw/onix/images/png"
	onixsvg "github.com/diiyw/onix/images/svg"
	"github.com/diiyw/onix/images/webp"
	"github.com/diiyw/onix/render"
)

// TextureTarget selects the block-compressed format family a build encodes
// project textures into. It mirrors ShaderTarget: the project stays
// platform-neutral and the build resolves one native format per platform, so a
// single manifest ships to every store.
type TextureTarget uint8

var encodeASTCTexture = func(pixels []byte, width, height int, mode BuildMode, srgb bool) ([]byte, int, error) {
	return encodeASTCChain(pixels, width, height, mode, srgb)
}

const (
	// TextureTargetSource keeps source images uncompressed.
	TextureTargetSource TextureTarget = iota
	// TextureTargetBC targets desktop GPUs, where D3D, Vulkan, and Metal all
	// sample the BC family.
	TextureTargetBC
	// TextureTargetASTC targets mobile GPUs that expose ASTC.
	TextureTargetASTC
)

func (target TextureTarget) String() string {
	switch target {
	case TextureTargetSource:
		return "source"
	case TextureTargetBC:
		return "BC"
	case TextureTargetASTC:
		return "ASTC"
	default:
		return fmt.Sprintf("TextureTarget(%d)", target)
	}
}

// Encodable reports whether this build can actually encode the target.
func (target TextureTarget) Encodable() bool {
	return target == TextureTargetBC || target == TextureTargetASTC
}

// textureTargetForBuildPlatform maps a build platform to the block format its
// GPU samples natively.
func textureTargetForBuildPlatform(platform BuildPlatform) TextureTarget {
	switch platform {
	case BuildPlatformWindows, BuildPlatformLinux, BuildPlatformMacOS:
		return TextureTargetBC
	case BuildPlatformAndroid, BuildPlatformIOS:
		return TextureTargetASTC
	case BuildPlatformWeb, BuildPlatformWeChat:
		return TextureTargetASTC
	default:
		return TextureTargetSource
	}
}

// compressibleTextureSource reports whether a collected resource is an image
// the build can decode and re-encode. Atlas metadata and existing containers
// are not sources.
func compressibleTextureSource(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".gif", ".jpg", ".jpeg", ".png", ".svg", ".webp":
		return true
	default:
		return false
	}
}

// packTextureSource compresses one source image for a target. The optional sRGB
// flag selects the encoder profile and mipmap transfer function used by colour
// textures; the runtime can still select the matching sampling variant without
// changing the block payload.
func packTextureSource(data []byte, name string, target TextureTarget, mode BuildMode, srgb bool) ([]byte, error) {
	if !target.Encodable() {
		return nil, nil
	}
	source, err := decodeTextureSource(data, name)
	if err != nil {
		return nil, err
	}
	if target == TextureTargetASTC {
		payload, levels, err := encodeASTCTexture(source.Pix, source.Rect.Dx(), source.Rect.Dy(), mode, srgb)
		if err != nil {
			return nil, err
		}
		format := render.TextureFormatASTC4x4
		if srgb {
			format = render.TextureFormatASTC4x4SRGB
		}
		return ontex.Marshal(ontex.Image{
			Format: format,
			Width:  source.Rect.Dx(), Height: source.Rect.Dy(), Levels: levels, Data: payload,
		})
	}
	return ontex.Encode(source.Pix, source.Rect.Dx(), source.Rect.Dy(), ontex.EncodeOptions{Mipmaps: true, SRGB: srgb})
}

func packEmbeddedTextureSource(data []byte, mime string, target TextureTarget, mode BuildMode, options graphics.TextureOptions) ([]byte, error) {
	if _, err := ontex.Unmarshal(data); err == nil {
		return data, nil
	}
	mime = strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
	extension := ""
	switch mime {
	case "image/gif":
		extension = ".gif"
	case "image/jpeg", "image/jpg":
		extension = ".jpg"
	case "image/png":
		extension = ".png"
	case "image/svg+xml":
		extension = ".svg"
	case "image/webp":
		extension = ".webp"
	}
	if extension == "" {
		trimmed := bytes.TrimSpace(data)
		switch {
		case len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}):
			extension = ".png"
		case len(data) >= 3 && bytes.Equal(data[:3], []byte{0xff, 0xd8, 0xff}):
			extension = ".jpg"
		case len(data) >= 6 && (bytes.Equal(data[:6], []byte("GIF87a")) || bytes.Equal(data[:6], []byte("GIF89a"))):
			extension = ".gif"
		case len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
			extension = ".webp"
		case bytes.HasPrefix(trimmed, []byte("<svg")) || bytes.HasPrefix(trimmed, []byte("<?xml")):
			extension = ".svg"
		default:
			return nil, fmt.Errorf("unsupported embedded texture MIME %q", mime)
		}
	}
	if target != TextureTargetASTC {
		return packTextureSource(data, "embedded"+extension, target, mode, options.SRGB)
	}
	source, err := decodeTextureSource(data, "embedded"+extension)
	if err != nil {
		return nil, err
	}
	payload, levels, err := encodeASTCTexture(source.Pix, source.Rect.Dx(), source.Rect.Dy(), mode, options.SRGB)
	if err != nil {
		return nil, err
	}
	format := render.TextureFormatASTC4x4
	if options.SRGB {
		format = render.TextureFormatASTC4x4SRGB
	}
	return ontex.Marshal(ontex.Image{Format: format, Width: source.Rect.Dx(), Height: source.Rect.Dy(), Levels: levels, Data: payload})
}

// decodeTextureSource decodes a source image into straight-alpha RGBA8 for the
// encoders, using the same codecs the runtime samples the source through. The
// runtime samples project textures as linear, non-premultiplied colour, so the
// build must feed the encoder the same byte layout or the shipped texture would
// not match what the project looks like today.
func decodeTextureSource(data []byte, name string) (*image.RGBA, error) {
	reader := bytes.NewReader(data)
	var (
		decoded image.Image
		err     error
	)
	switch strings.ToLower(path.Ext(name)) {
	case ".gif":
		decoded, err = stdgif.Decode(reader)
	case ".jpg", ".jpeg":
		decoded, err = stdjpeg.Decode(reader)
	case ".svg":
		decoded, err = onixsvg.Render(reader, onixsvg.Options{})
	case ".webp":
		decoded, err = webp.Decode(reader)
	case ".png":
		decoded, err = onixpng.Decode(reader)
	default:
		return nil, fmt.Errorf("unsupported source image %s", name)
	}
	if err != nil {
		return nil, err
	}
	return graphics.StraightRGBA8(decoded), nil
}
