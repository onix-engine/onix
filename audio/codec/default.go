// Package codec provides the full built-in audio decoder collection.
package codec

import (
	"github.com/onix-engine/onix/audio"
	"github.com/onix-engine/onix/audio/codec/flac"
	"github.com/onix-engine/onix/audio/codec/mp3"
	"github.com/onix-engine/onix/audio/codec/vorbis"
	"github.com/onix-engine/onix/audio/codec/wav"
)

type DefaultDecoder struct{}

func (DefaultDecoder) DecodeAudio(source audio.Source, format audio.Format, stream bool) (*audio.Clip, error) {
	return audio.DecoderSet[flac.Decoder, mp3.Decoder, vorbis.Decoder, wav.Decoder]{}.DecodeAudio(source, format, stream)
}
