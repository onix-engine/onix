// Package assets contains opt-in project resource implementations. Import it
// from an application setup when that application needs raster images or
// audio resources.
package assets

import (
	"github.com/onix-engine/onix/audio"
	"github.com/onix-engine/onix/font"
	"github.com/onix-engine/onix/graphics"
	"github.com/onix-engine/onix/project"
	projectaudio "github.com/onix-engine/onix/project/audio"
	onixtext "github.com/onix-engine/onix/text"
	"github.com/onix-engine/onix/window"
)

// Feature installs statically selected image, font, and audio decoder
// collections. Its type arguments are compile-time link boundaries.
type Feature[I graphics.ImageDecoder, F font.Decoder, K window.IconDecoder, A audio.Decoder] struct{}

func (Feature[I, F, K, A]) ConfigureResources(context project.ResourceLoadContext, entries project.ResourceEntries) error {
	var imageDecoder I
	var iconDecoder K
	context.SetImageDecoder(imageDecoder)
	context.SetWindowIconDecoder(iconDecoder)
	context.SetFontDecoder(fontDecoder[F]{})
	return projectaudio.Feature[A]{}.ConfigureResources(context, entries)
}

type fontDecoder[D font.Decoder] struct{}

func (fontDecoder[D]) DecodeFont(data []byte, path string) (*onixtext.Font, error) {
	var decoder D
	face, err := decoder.DecodeFont(data, path)
	if err != nil {
		return nil, err
	}
	return onixtext.NewFont(face), nil
}
