package resource

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"image"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	onixpng "github.com/diiyw/onix/images/png"
	onixsvg "github.com/diiyw/onix/images/svg"
	webpconfig "github.com/diiyw/onix/images/webp/config"
	"github.com/diiyw/onix/internal/texcontainer"
)

// Size returns an image's logical dimensions from the configured resource
// source without decoding its pixel data. The source is opened through the
// engine resource layer, so packaged and directory-backed projects behave the
// same way.
func Size(name string) (float64, float64, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." {
		return 0, 0, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	var file fs.File
	var err error
	if filepath.IsAbs(name) {
		file, err = os.Open(name)
	} else {
		source, _, ok := Default()
		if !ok {
			return 0, 0, fmt.Errorf("resource: default source is not configured")
		}
		if !fs.ValidPath(name) {
			return 0, 0, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
		}
		file, err = source.Open(name)
	}
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()
	config, err := decodeImageConfig(file, path.Ext(name))
	if err != nil {
		return 0, 0, err
	}
	if config.Width <= 0 || config.Height <= 0 {
		return 0, 0, fmt.Errorf("resource: image %q has invalid dimensions %dx%d", name, config.Width, config.Height)
	}
	return float64(config.Width), float64(config.Height), nil
}

// decodeImageConfig reads image dimensions from a payload without decoding it.
//
// A packed project replaces a raster image with a block-compressed container in
// place, so the payload keeps the source extension and only its leading bytes
// identify it. The container header carries the dimensions, which is all this
// needs.
func decodeImageConfig(reader io.Reader, extension string) (image.Config, error) {
	header := make([]byte, texcontainer.HeaderSize)
	read, err := io.ReadFull(reader, header)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return image.Config{}, err
	}
	header = header[:read]
	if width, height, ok := texcontainer.Dimensions(header); ok {
		return image.Config{Width: width, Height: height}, nil
	}
	return decodeSourceImageConfig(io.MultiReader(bytes.NewReader(header), reader), extension, header)
}

func decodeSourceImageConfig(reader io.Reader, extension string, header []byte) (image.Config, error) {
	switch strings.ToLower(extension) {
	case ".png":
		return onixpng.DecodeConfig(reader)
	case ".svg":
		return onixsvg.DecodeConfig(reader)
	case ".svgz":
		compressed, err := gzip.NewReader(reader)
		if err != nil {
			return image.Config{}, err
		}
		config, decodeErr := onixsvg.DecodeConfig(compressed)
		closeErr := compressed.Close()
		if decodeErr != nil {
			return image.Config{}, decodeErr
		}
		return config, closeErr
	case ".webp":
		return webpconfig.Decode(reader)
	}
	// Inspect signatures directly: image.DecodeConfig's global registry retains
	// every registered pixel decoder even when a project only needs dimensions.
	switch {
	case bytes.HasPrefix(header, []byte("\x89PNG\r\n\x1a\n")):
		return onixpng.DecodeConfig(reader)
	case bytes.HasPrefix(header, []byte("GIF87a")), bytes.HasPrefix(header, []byte("GIF89a")):
		return decodeGIFDimensions(reader)
	case bytes.HasPrefix(header, []byte{0xff, 0xd8}):
		return decodeJPEGDimensions(reader)
	case len(header) >= 12 && string(header[:4]) == "RIFF" && string(header[8:12]) == "WEBP":
		return webpconfig.Decode(reader)
	default:
		return image.Config{}, image.ErrFormat
	}
}
