package mp3

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/diiyw/onix/audio"
	"github.com/hajimehoshi/go-mp3"
)

type Decoder struct{}

func (Decoder) SupportsAudioFormat(format audio.Format) bool { return format == audio.FormatMP3 }

func (Decoder) DecodeAudio(source audio.Source, format audio.Format, stream bool) (*audio.Clip, error) {
	if format != audio.FormatMP3 {
		return nil, errors.Join(fmt.Errorf("audio: unsupported format %q", format), source.Close())
	}
	if stream {
		return decodeStream(source, format)
	}
	return audio.DecodeClip(format, source, decode)
}

func decode(data []byte) (audio.DecodedAudio, error) {
	decoder, err := mp3.NewDecoder(bytes.NewReader(data))
	if err != nil {
		return audio.DecodedAudio{}, err
	}
	pcm, err := io.ReadAll(decoder)
	if err != nil {
		return audio.DecodedAudio{}, err
	}
	if len(pcm) == 0 || len(pcm)%4 != 0 {
		return audio.DecodedAudio{}, fmt.Errorf("invalid stereo PCM byte count %d", len(pcm))
	}
	samples := make([]float32, len(pcm)/2)
	for index := range samples {
		sample := int16(binary.LittleEndian.Uint16(pcm[index*2:]))
		samples[index] = audio.NormalizeSigned(sample, 16)
	}
	return audio.DecodedAudio{Samples: samples, SampleRate: decoder.SampleRate(), Channels: 2}, nil
}

func decodeStream(source audio.Source, format audio.Format) (*audio.Clip, error) {
	rate, frames, err := inspectStream(source)
	if err != nil {
		return nil, errors.Join(err, source.Close())
	}
	return audio.NewStreamingClip(format, source, rate, frames,
		func(source audio.Source) (io.ReadSeekCloser, error) { return newStreamReader(source, rate, frames) },
	)
}
