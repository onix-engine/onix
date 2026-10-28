package ontex

import (
	"bytes"
	"testing"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
)

func gradientPixels(width, height int, alpha uint8) []byte {
	pixels := make([]byte, width*height*4)
	spanX, spanY := max(width-1, 1), max(height-1, 1)
	for y := range height {
		for x := range width {
			offset := (y*width + x) * 4
			pixels[offset] = uint8(x * 255 / spanX)
			pixels[offset+1] = uint8(y * 255 / spanY)
			pixels[offset+2] = uint8((x + y) * 255 / (spanX + spanY))
			pixels[offset+3] = alpha
		}
	}
	return pixels
}

func TestEncodeRoundTripsThroughContainer(t *testing.T) {
	width, height := 32, 16
	pixels := gradientPixels(width, height, 255)
	data, err := Encode(pixels, width, height, EncodeOptions{SRGB: true, Mipmaps: true})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	image, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if image.Format != render.TextureFormatBC1SRGB {
		t.Fatalf("opaque source encoded as %s, want bc1-srgb", image.Format.Name())
	}
	if image.Width != width || image.Height != height {
		t.Fatalf("container reports %dx%d, want %dx%d", image.Width, image.Height, width, height)
	}
	if want := render.MipLevels(width, height); image.Levels != want {
		t.Fatalf("container reports %d levels, want %d", image.Levels, want)
	}
	if want := image.Format.ChainSize(width, height, image.Levels); len(image.Data) != want {
		t.Fatalf("payload is %d bytes, want %d", len(image.Data), want)
	}

	// Level offsets must tile the payload exactly, with no gap and no overlap.
	// The layout is read through the descriptor the backend receives, so this
	// checks the packer and the uploader agree on where each level begins.
	descriptor := render.TextureDescriptor{
		Width:  image.Width,
		Height: image.Height,
		Format: image.Format,
		Levels: image.Levels,
	}
	if descriptor.MipLevels() != image.Levels {
		t.Fatalf("descriptor reports %d levels, container carries %d", descriptor.MipLevels(), image.Levels)
	}
	if err := descriptor.Validate(image.Data); err != nil {
		t.Fatalf("packed payload is not uploadable: %v", err)
	}
	offset := 0
	for level := range descriptor.MipLevels() {
		got := descriptor.Level(level)
		if got.Offset != offset {
			t.Fatalf("level %d starts at %d, want %d", level, got.Offset, offset)
		}
		offset += got.Bytes
	}
	if offset != len(image.Data) {
		t.Fatalf("levels cover %d bytes, payload is %d", offset, len(image.Data))
	}
}

func TestEncodeWithoutMipmapsCarriesOneLevel(t *testing.T) {
	width, height := 16, 16
	pixels := gradientPixels(width, height, 255)
	data, err := Encode(pixels, width, height, EncodeOptions{})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	image, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if image.Levels != 1 {
		t.Fatalf("container reports %d levels, want 1", image.Levels)
	}
	if want := image.Format.LevelSize(width, height); len(image.Data) != want {
		t.Fatalf("payload is %d bytes, want %d", len(image.Data), want)
	}
}

func TestEncodeSelectsAlphaCapableFormat(t *testing.T) {
	width, height := 16, 16
	cases := []struct {
		name   string
		alpha  uint8
		srgb   bool
		expect render.TextureFormat
	}{
		{name: "opaque linear", alpha: 255, expect: render.TextureFormatBC1},
		{name: "opaque srgb", alpha: 255, srgb: true, expect: render.TextureFormatBC1SRGB},
		{name: "transparent linear", alpha: 128, expect: render.TextureFormatBC3},
		{name: "transparent srgb", alpha: 128, srgb: true, expect: render.TextureFormatBC3SRGB},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			pixels := gradientPixels(width, height, testCase.alpha)
			data, err := Encode(pixels, width, height, EncodeOptions{SRGB: testCase.srgb})
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			image, err := Unmarshal(data)
			if err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if image.Format != testCase.expect {
				t.Fatalf("encoded as %s, want %s", image.Format.Name(), testCase.expect.Name())
			}
		})
	}
}

func TestEncodeRecordsPremultipliedAlpha(t *testing.T) {
	pixels := gradientPixels(8, 8, 255)
	data, err := Encode(pixels, 8, 8, EncodeOptions{Premultiplied: true})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	image, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !image.Premultiplied {
		t.Fatal("premultiplied flag was not recorded")
	}
}

func TestEncodeIsDeterministic(t *testing.T) {
	pixels := gradientPixels(64, 64, 200)
	first, err := Encode(pixels, 64, 64, EncodeOptions{SRGB: true, Mipmaps: true})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	second, err := Encode(pixels, 64, 64, EncodeOptions{SRGB: true, Mipmaps: true})
	if err != nil {
		t.Fatalf("encode twice: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("encoding the same source twice produced different bytes")
	}
}

func TestUnmarshalRejectsMalformedContainers(t *testing.T) {
	valid, err := Encode(gradientPixels(8, 8, 255), 8, 8, EncodeOptions{Mipmaps: true})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	cases := map[string][]byte{
		"truncated header": valid[:headerSize-1],
		"bad magic":        append([]byte("NOTONIX\000\000"), valid[magicSize:]...),
		"short payload":    valid[:len(valid)-1],
		"extra payload":    append(append([]byte{}, valid...), 0),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Unmarshal(data); err == nil {
				t.Fatal("expected an error")
			}
		})
	}

	// An uncompressed format cannot be sampled through this container.
	uncompressed := append([]byte{}, valid...)
	uncompressed[8] = byte(render.TextureFormatRGBA8)
	if _, err := Unmarshal(uncompressed); err == nil {
		t.Fatal("expected an error for an uncompressed format")
	}

	// A header claiming more levels than the dimensions allow must be rejected.
	tooDeep := append([]byte{}, valid...)
	tooDeep[10] = 40
	if _, err := Unmarshal(tooDeep); err == nil {
		t.Fatal("expected an error for an impossible mip depth")
	}
}

func TestEncodeRejectsInvalidSource(t *testing.T) {
	if _, err := Encode(nil, 4, 4, EncodeOptions{}); err == nil {
		t.Fatal("expected an error for a short source")
	}
	if _, err := Encode(nil, 0, 4, EncodeOptions{}); err == nil {
		t.Fatal("expected an error for a zero dimension")
	}
	// Only block-compressed formats have an encoder. RGBA8 is the enum zero
	// value, which means "choose for me", so use a non-zero uncompressed format
	// to exercise the rejection.
	if _, err := Encode(gradientPixels(4, 4, 255), 4, 4, EncodeOptions{Format: render.TextureFormatSRGBA8}); err == nil {
		t.Fatal("expected an error for an uncompressed target format")
	}
}

func TestDecodeProducesSamplableTexture(t *testing.T) {
	width, height := 16, 8
	pixels := gradientPixels(width, height, 255)
	data, err := Encode(pixels, width, height, EncodeOptions{Mipmaps: true})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	texture, err := Decode(data, graphics.TextureOptions{})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !texture.Valid() {
		t.Fatal("decoded texture is not valid")
	}
	if texture.Width != width || texture.Height != height {
		t.Fatalf("texture is %dx%d, want %dx%d", texture.Width, texture.Height, width, height)
	}
	if !texture.Format().Compressed() {
		t.Fatalf("texture format %s is not compressed", texture.Format().Name())
	}
	if want := render.MipLevels(width, height); texture.MipLevels() != want {
		t.Fatalf("texture reports %d mip levels, want %d", texture.MipLevels(), want)
	}
}

// One payload serves both samplers: the build packs linear colour, and a caller
// that samples sRGB reuses the same blocks under the sRGB internal format.
func TestDecodeSelectsSamplingVariantWithoutReencoding(t *testing.T) {
	width, height := 16, 8
	data, err := Encode(gradientPixels(width, height, 255), width, height, EncodeOptions{Mipmaps: true})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	linear, err := Decode(data, graphics.TextureOptions{})
	if err != nil {
		t.Fatalf("decode linear: %v", err)
	}
	defer linear.Dispose()
	srgb, err := Decode(data, graphics.TextureOptions{SRGB: true})
	if err != nil {
		t.Fatalf("decode sRGB: %v", err)
	}
	defer srgb.Dispose()
	if linear.Format() != render.TextureFormatBC1 || srgb.Format() != render.TextureFormatBC1SRGB {
		t.Fatalf("sampling variants = %s and %s, want bc1 and bc1-srgb", linear.Format().Name(), srgb.Format().Name())
	}
}

type deferredUploadDriver struct {
	render.Driver
	pixels []byte
}

func (d *deferredUploadDriver) CreateTexture(_ render.TextureDescriptor, pixels []byte) (render.TextureHandle, error) {
	d.pixels = bytes.Clone(pixels)
	return render.NewHandle[render.TextureResource](1, 1), nil
}
func (*deferredUploadDriver) DestroyTexture(render.TextureHandle) error { return nil }
func (*deferredUploadDriver) Destroy()                                  {}

func TestDecodeOwnsPayloadUntilDeferredUpload(t *testing.T) {
	payload := bytes.Repeat([]byte{0x39}, render.TextureFormatBC1.LevelSize(4, 4))
	data, err := Marshal(Image{Format: render.TextureFormatBC1, Width: 4, Height: 4, Levels: 1, Data: payload})
	if err != nil {
		t.Fatal(err)
	}
	driver := &deferredUploadDriver{}
	device, _ := render.NewDevice(driver)
	defer device.Destroy()
	device.Activate()
	texture, err := Decode(data, graphics.TextureOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer texture.Dispose()
	if driver.pixels != nil || !texture.Valid() {
		t.Fatal("decode or validation invoked the driver")
	}
	clear(data)
	if !texture.RenderHandleFor(device).Valid() || !bytes.Equal(driver.pixels, payload) {
		t.Fatal("deferred upload used released container bytes")
	}
}
