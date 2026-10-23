package resource

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	stdpng "image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diiyw/onix/internal/texcontainer"
)

func TestSizeReadsAbsoluteImageWithoutDefaultSource(t *testing.T) {
	name := filepath.Join(t.TempDir(), "image.png")
	file, err := os.Create(name)
	if err != nil {
		t.Fatalf("create image: %v", err)
	}
	asset := image.NewNRGBA(image.Rect(0, 0, 7, 5))
	asset.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	if err := stdpng.Encode(file, asset); err != nil {
		_ = file.Close()
		t.Fatalf("encode image: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close image: %v", err)
	}

	width, height, err := Size(name)
	if err != nil {
		t.Fatalf("read image size: %v", err)
	}
	if width != 7 || height != 5 {
		t.Fatalf("image size = %vx%v, want 7x5", width, height)
	}
}

func TestDecodeImageConfigUsesEngineSVGDecoder(t *testing.T) {
	config, err := decodeImageConfig(strings.NewReader(`<svg viewBox="0 0 32 18"></svg>`), ".SVG")
	if err != nil {
		t.Fatalf("decode SVG config: %v", err)
	}
	if config.Width != 32 || config.Height != 18 {
		t.Fatalf("SVG size = %dx%d, want 32x18", config.Width, config.Height)
	}
}

func TestDecodeImageConfigReadsCompressedSVG(t *testing.T) {
	var encoded bytes.Buffer
	compressed := gzip.NewWriter(&encoded)
	if _, err := compressed.Write([]byte(`<svg width="21" height="13"></svg>`)); err != nil {
		t.Fatalf("compress SVG: %v", err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatalf("close compressed SVG: %v", err)
	}

	config, err := decodeImageConfig(bytes.NewReader(encoded.Bytes()), ".svgz")
	if err != nil {
		t.Fatalf("decode compressed SVG config: %v", err)
	}
	if config.Width != 21 || config.Height != 13 {
		t.Fatalf("compressed SVG size = %dx%d, want 21x13", config.Width, config.Height)
	}
}

// A packed project keeps the source path for a compressed texture, so layout
// has to read the dimensions out of the container header instead of the PNG.
func TestDecodeImageConfigReadsPackedTextureContainer(t *testing.T) {
	container := packedContainerHeader(t, 40, 24)
	config, err := decodeImageConfig(bytes.NewReader(container), ".png")
	if err != nil {
		t.Fatalf("decode packed texture config: %v", err)
	}
	if config.Width != 40 || config.Height != 24 {
		t.Fatalf("packed texture size = %dx%d, want 40x24", config.Width, config.Height)
	}
	// A payload that only looks like a container from its prefix must still be
	// handed to the source decoder.
	if _, err := decodeImageConfig(bytes.NewReader(container[:texcontainer.MagicSize]), ".png"); err == nil {
		t.Fatal("truncated container header was accepted")
	}
}

func packedContainerHeader(t *testing.T, width, height int) []byte {
	t.Helper()
	header := make([]byte, texcontainer.HeaderSize)
	copy(header, texcontainer.Magic[:])
	binary.LittleEndian.PutUint32(header[12:], uint32(width))
	binary.LittleEndian.PutUint32(header[16:], uint32(height))
	return header
}

func TestDecodeImageConfigSniffsRasterHeaders(t *testing.T) {
	pixels := image.NewRGBA(image.Rect(0, 0, 17, 9))
	for _, test := range []struct {
		name   string
		encode func(*bytes.Buffer) error
	}{
		{"png", func(out *bytes.Buffer) error { return stdpng.Encode(out, pixels) }},
		{"gif", func(out *bytes.Buffer) error { return gif.Encode(out, pixels, nil) }},
		{"jpeg", func(out *bytes.Buffer) error { return jpeg.Encode(out, pixels, nil) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := test.encode(&out); err != nil {
				t.Fatal(err)
			}
			data := out.Bytes()
			if test.name == "jpeg" {
				// An odd-sized, large comment must be skipped in one streaming pass.
				comment := make([]byte, 60005)
				comment[0], comment[1] = 0xff, 0xfe
				binary.BigEndian.PutUint16(comment[2:4], uint16(len(comment)-2))
				data = append(append(bytes.Clone(data[:2]), comment...), data[2:]...)
			}
			for _, extension := range []string{".unknown", "." + test.name} {
				got, err := decodeImageConfig(bytes.NewReader(data), extension)
				if err != nil || got.Width != 17 || got.Height != 9 {
					t.Fatalf("%s config = %#v, error = %v", extension, got, err)
				}
			}
			for _, length := range []int{0, 6, 10} {
				if _, err := decodeImageConfig(bytes.NewReader(data[:length]), ".unknown"); err == nil {
					t.Fatalf("accepted truncated %d-byte header", length)
				}
			}
		})
	}
}

func TestDecodeImageConfigSniffsWebP(t *testing.T) {
	data, err := os.ReadFile("../images/webp/testdata/vp8x-alpha.webp")
	if err != nil {
		t.Fatal(err)
	}
	for _, extension := range []string{".webp", ".unknown"} {
		got, err := decodeImageConfig(bytes.NewReader(data), extension)
		if err != nil || got.Width != 400 || got.Height != 301 {
			t.Fatalf("%s config = %#v, error = %v", extension, got, err)
		}
	}
}

func TestDecodeImageConfigRejectsMalformedRasterHeaders(t *testing.T) {
	for _, data := range [][]byte{
		[]byte("GIF89a\x01\x00\x01\x00\x80\x00\x00"), // Missing palette.
		{0xff, 0xd8, 0xff, 0xc0, 0, 1},               // Invalid segment length.
		{0xff, 0xd8, 0xff, 0xc0, 0, 11, 8},           // Truncated frame header.
		{0xff, 0xd8, 0xff, 0xc2, 0, 8},               // Invalid frame length.
		{0xff, 0xd8, 0xff, 0xda},                     // Scan without frame header.
		[]byte("not an image"),
	} {
		if _, err := decodeImageConfig(bytes.NewReader(data), ".unknown"); err == nil {
			t.Fatalf("accepted malformed image header %x", data)
		}
	}
}
