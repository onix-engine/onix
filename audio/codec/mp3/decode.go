package mp3

import (
	"errors"
	"fmt"
	"io"

	"github.com/onix-engine/onix/audio"
	"github.com/onix-engine/onix/audio/codec/mp3/internal/layer3"
)

type Decoder struct{}

func (Decoder) SupportsAudioFormat(format audio.Format) bool { return format == audio.FormatMP3 }

func (Decoder) DecodeAudio(source audio.Source, format audio.Format, stream bool) (*audio.Clip, error) {
	if source == nil {
		return nil, errors.New("audio: missing MP3 source")
	}
	if format != audio.FormatMP3 {
		return nil, errors.Join(fmt.Errorf("audio: unsupported format %q", format), source.Close())
	}
	if !stream {
		return audio.DecodeClip(format, source, decode)
	}
	reader, err := source.Open()
	if err != nil {
		return nil, errors.Join(err, source.Close())
	}
	track, scanErr := layer3.Inspect(reader)
	if err := errors.Join(scanErr, reader.Close()); err != nil {
		return nil, errors.Join(err, source.Close())
	}
	return audio.NewStreamingClip(format, source, track.SampleRate(), track.Frames(), func(source audio.Source) (io.ReadSeekCloser, error) {
		reader, err := source.Open()
		if err != nil {
			return nil, err
		}
		return newStreamReader(track, reader), nil
	})
}

func decode(data []byte) (audio.DecodedAudio, error) {
	samples, rate, err := layer3.Decode(data)
	if err != nil {
		return audio.DecodedAudio{}, err
	}
	return audio.DecodedAudio{Samples: samples, SampleRate: rate, Channels: 2}, nil
}
