// Package config reads WebP headers without registering a pixel decoder.
package config

import (
	"errors"
	"image"
	"image/color"
	"io"

	"github.com/diiyw/onix/images/webp/internal/riff"
	"github.com/diiyw/onix/images/webp/internal/vp8"
	"github.com/diiyw/onix/images/webp/internal/vp8l"
)

var errInvalidFormat = errors.New("webp: invalid format")

// Decode returns the color model and dimensions without decoding pixel data.
// Keeping this entry in its own package lets dimension queries omit the full
// WebP decoder and its image.RegisterFormat initialization.
func Decode(reader io.Reader) (image.Config, error) {
	form, chunks, err := riff.NewReader(reader)
	if err != nil {
		return image.Config{}, err
	}
	if form != (riff.FourCC{'W', 'E', 'B', 'P'}) {
		return image.Config{}, errInvalidFormat
	}
	for {
		id, size, data, err := chunks.Next()
		if err == io.EOF {
			err = errInvalidFormat
		}
		if err != nil {
			return image.Config{}, err
		}
		switch id {
		case riff.FourCC{'A', 'L', 'P', 'H'}, riff.FourCC{'A', 'N', 'I', 'M'}, riff.FourCC{'A', 'N', 'M', 'F'}:
			// Alpha and animation chunks require an earlier VP8X header, whose
			// canvas dimensions would already have completed this query.
			return image.Config{}, errInvalidFormat
		case riff.FourCC{'V', 'P', '8', ' '}:
			if size > 1<<31-1 {
				return image.Config{}, errInvalidFormat
			}
			decoder := vp8.NewDecoder()
			decoder.Init(data, int(size))
			header, err := decoder.DecodeFrameHeader()
			if err != nil {
				return image.Config{}, err
			}
			return image.Config{ColorModel: color.YCbCrModel, Width: header.Width, Height: header.Height}, nil
		case riff.FourCC{'V', 'P', '8', 'L'}:
			return vp8l.DecodeConfig(data)
		case riff.FourCC{'V', 'P', '8', 'X'}:
			if size != 10 {
				return image.Config{}, errInvalidFormat
			}
			var header [10]byte
			if _, err := io.ReadFull(data, header[:]); err != nil {
				return image.Config{}, err
			}
			var model color.Model = color.YCbCrModel
			if header[0]&0x10 != 0 {
				model = color.NYCbCrAModel
			}
			return image.Config{
				ColorModel: model,
				Width:      1 + int(uint32(header[4])|uint32(header[5])<<8|uint32(header[6])<<16),
				Height:     1 + int(uint32(header[7])|uint32(header[8])<<8|uint32(header[9])<<16),
			}, nil
		}
	}
}
