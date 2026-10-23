package flac

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/diiyw/onix/audio"
	"github.com/mewkiz/flac"
)

type Decoder struct{}

func (Decoder) SupportsAudioFormat(format audio.Format) bool { return format == audio.FormatFLAC }

func (Decoder) DecodeAudio(source audio.Source, format audio.Format, stream bool) (*audio.Clip, error) {
	if stream {
		return nil, errors.Join(fmt.Errorf("audio: streaming format %q is not supported", format), source.Close())
	}
	return audio.DecodeClip(format, source, decode)
}

func decode(data []byte) (audio.DecodedAudio, error) {
	stream, err := flac.New(bytes.NewReader(data))
	if err != nil {
		return audio.DecodedAudio{}, err
	}
	channels := int(stream.Info.NChannels)
	if channels < 1 || channels > audio.OutputChannels {
		return audio.DecodedAudio{}, fmt.Errorf("unsupported channel count %d", channels)
	}
	bits := int(stream.Info.BitsPerSample)
	samples := make([]float32, 0, int(stream.Info.NSamples)*channels)
	for {
		frame, err := stream.ParseNext()
		if err == io.EOF {
			break
		}
		if err != nil {
			return audio.DecodedAudio{}, err
		}
		for index := range frame.Subframes[0].Samples {
			for channel := range channels {
				samples = append(samples, audio.NormalizeSigned(frame.Subframes[channel].Samples[index], bits))
			}
		}
	}
	return audio.DecodedAudio{
		Samples:    samples,
		SampleRate: int(stream.Info.SampleRate),
		Channels:   channels,
	}, nil
}
