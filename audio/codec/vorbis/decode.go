package vorbis

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/diiyw/onix/audio"
	"github.com/jfreymuth/oggvorbis"
)

type Decoder struct{}

func (Decoder) SupportsAudioFormat(format audio.Format) bool { return format == audio.FormatOGG }

func (Decoder) DecodeAudio(source audio.Source, format audio.Format, stream bool) (*audio.Clip, error) {
	if stream {
		return nil, errors.Join(fmt.Errorf("audio: streaming format %q is not supported", format), source.Close())
	}
	return audio.DecodeClip(format, source, decode)
}

func decode(data []byte) (audio.DecodedAudio, error) {
	samples, format, err := oggvorbis.ReadAll(bytes.NewReader(data))
	if err != nil {
		return audio.DecodedAudio{}, err
	}
	if format == nil {
		return audio.DecodedAudio{}, fmt.Errorf("missing Vorbis format")
	}
	return audio.DecodedAudio{
		Samples:    samples,
		SampleRate: format.SampleRate,
		Channels:   format.Channels,
	}, nil
}
