// Package ontex reads and writes the ONIXTEX1 container: a block-compressed
// texture together with the mip chain the build already generated.
//
// The container exists because compressed textures cannot be uploaded the way
// RGBA8 images are. A sampler cannot generate mips for block data, and a 4x4
// block has no meaningful partial update, so the build has to ship the whole
// chain and the runtime has to hand it to the device untouched.
package ontex

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/diiyw/onix/internal/texcontainer"
	"github.com/diiyw/onix/render"
)

const (
	// Extension is the suffix the build pipeline writes.
	Extension = ".ontex"
	// Version is the container revision carried by the magic.
	Version = 1

	magicSize  = texcontainer.MagicSize
	headerSize = texcontainer.HeaderSize
)

// Flags recorded alongside the compressed payload.
const (
	// FlagPremultipliedAlpha marks colour channels already scaled by alpha.
	// Compressed payloads bake this in, so the runtime cannot infer it.
	FlagPremultipliedAlpha = 1 << 0
)

var magic = texcontainer.Magic

// Image is a container payload. Data holds every mip level concatenated from the
// largest down to 1x1, which is the order the device uploads them in.
type Image struct {
	Format        render.TextureFormat
	Width, Height int
	Levels        int
	Premultiplied bool
	Data          []byte
}

// Marshal encodes a container. It rejects payloads whose length does not match
// the mip chain the header describes, so a corrupt build fails loudly instead of
// uploading garbage.
func Marshal(image Image) ([]byte, error) {
	if err := image.validate(); err != nil {
		return nil, err
	}
	data := make([]byte, headerSize+len(image.Data))
	copy(data, magic[:])
	data[8] = byte(image.Format)
	if image.Premultiplied {
		data[9] = FlagPremultipliedAlpha
	}
	data[10] = byte(image.Levels)
	binary.LittleEndian.PutUint32(data[12:], uint32(image.Width))
	binary.LittleEndian.PutUint32(data[16:], uint32(image.Height))
	copy(data[headerSize:], image.Data)
	return data, nil
}

// Unmarshal reads a container header and returns the payload as a subslice of
// data, so loading a texture from a resource package never copies the blocks.
func Unmarshal(data []byte) (Image, error) {
	if len(data) < headerSize {
		return Image{}, errors.New("ontex: truncated header")
	}
	if [magicSize]byte(data[:magicSize]) != magic {
		return Image{}, errors.New("ontex: not an ONIXTEX1 container")
	}
	image := Image{
		Format:        render.TextureFormat(data[8]),
		Premultiplied: data[9]&FlagPremultipliedAlpha != 0,
		Levels:        int(data[10]),
		Width:         int(binary.LittleEndian.Uint32(data[12:])),
		Height:        int(binary.LittleEndian.Uint32(data[16:])),
	}
	if data[11] != 0 {
		return Image{}, fmt.Errorf("ontex: unsupported header flags 0x%02x", data[11])
	}
	// Validate the header before sizing the payload, so a corrupt depth cannot
	// be silently clamped into a valid-looking chain.
	if err := image.validate(); err != nil {
		return Image{}, err
	}
	image.Data = data[headerSize:]
	if err := image.validatePayload(); err != nil {
		return Image{}, err
	}
	return image, nil
}

func (image Image) validate() error {
	if !image.Format.Valid() || !image.Format.Compressed() {
		return fmt.Errorf("ontex: %s is not a block-compressed format", image.Format.Name())
	}
	if image.Width <= 0 || image.Height <= 0 {
		return fmt.Errorf("ontex: invalid dimensions %dx%d", image.Width, image.Height)
	}
	if depth := render.MipLevels(image.Width, image.Height); image.Levels <= 0 || image.Levels > depth {
		return fmt.Errorf("ontex: %dx%d has %d mip levels, got %d", image.Width, image.Height, depth, image.Levels)
	}
	return nil
}

func (image Image) validatePayload() error {
	expected := image.Format.ChainSize(image.Width, image.Height, image.Levels)
	if len(image.Data) != expected {
		return fmt.Errorf("ontex: %s %dx%d with %d levels needs %d bytes, got %d",
			image.Format.Name(), image.Width, image.Height, image.Levels, expected, len(image.Data))
	}
	return nil
}
