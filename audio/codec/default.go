// Package codec provides the full built-in audio decoder collection.
package codec

import (
	"github.com/diiyw/onix/audio"
	"github.com/diiyw/onix/audio/codec/flac"
	"github.com/diiyw/onix/audio/codec/mp3"
	"github.com/diiyw/onix/audio/codec/vorbis"
	"github.com/diiyw/onix/audio/codec/wav"
)

type DefaultDecoder struct{}

func (DefaultDecoder) DecodeAudio(source audio.Source, format audio.Format, stream bool) (*audio.Clip, error) {
	return audio.DecoderSet[flac.Decoder, mp3.Decoder, vorbis.Decoder, wav.Decoder]{}.DecodeAudio(source, format, stream)
}
