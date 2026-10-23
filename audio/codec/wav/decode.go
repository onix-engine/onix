package wav

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/diiyw/onix/audio"
)

const (
	wavFormatPCM   = 1
	wavFormatFloat = 3
)

type wavHeader struct {
	format     uint16
	channels   int
	sampleRate int
	blockAlign int
	bits       int
}

type Decoder struct{}

func (Decoder) SupportsAudioFormat(format audio.Format) bool { return format == audio.FormatWAV }

func (Decoder) DecodeAudio(source audio.Source, format audio.Format, stream bool) (*audio.Clip, error) {
	if stream {
		return nil, errors.Join(fmt.Errorf("audio: streaming format %q is not supported", format), source.Close())
	}
	return audio.DecodeClip(format, source, decode)
}

func decode(data []byte) (audio.DecodedAudio, error) {
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return audio.DecodedAudio{}, fmt.Errorf("invalid RIFF/WAVE header")
	}
	var header wavHeader
	var pcm []byte
	for offset := 12; offset+8 <= len(data); {
		size := int(binary.LittleEndian.Uint32(data[offset+4:]))
		start := offset + 8
		end := start + size
		if size < 0 || end < start || end > len(data) {
			return audio.DecodedAudio{}, fmt.Errorf("invalid %q chunk size %d", data[offset:offset+4], size)
		}
		switch string(data[offset : offset+4]) {
		case "fmt ":
			if err := header.parse(data[start:end]); err != nil {
				return audio.DecodedAudio{}, err
			}
		case "data":
			pcm = data[start:end]
		}
		offset = end + size%2
	}
	if header.sampleRate == 0 {
		return audio.DecodedAudio{}, fmt.Errorf("missing fmt chunk")
	}
	if len(pcm) == 0 {
		return audio.DecodedAudio{}, fmt.Errorf("missing data chunk")
	}
	if len(pcm)%header.blockAlign != 0 {
		return audio.DecodedAudio{}, fmt.Errorf("PCM size %d is not aligned to %d bytes", len(pcm), header.blockAlign)
	}

	bytesPerChannel := header.bits / 8
	frames := len(pcm) / header.blockAlign
	samples := make([]float32, 0, frames*header.channels)
	for frame := range frames {
		base := frame * header.blockAlign
		for channel := range header.channels {
			offset := base + channel*bytesPerChannel
			sample, err := header.sample(pcm[offset : offset+bytesPerChannel])
			if err != nil {
				return audio.DecodedAudio{}, err
			}
			samples = append(samples, sample)
		}
	}
	return audio.DecodedAudio{
		Samples:    samples,
		SampleRate: header.sampleRate,
		Channels:   header.channels,
	}, nil
}

func (h *wavHeader) parse(data []byte) error {
	if len(data) < 16 {
		return fmt.Errorf("invalid fmt chunk size %d", len(data))
	}
	h.format = binary.LittleEndian.Uint16(data)
	h.channels = int(binary.LittleEndian.Uint16(data[2:]))
	h.sampleRate = int(binary.LittleEndian.Uint32(data[4:]))
	h.blockAlign = int(binary.LittleEndian.Uint16(data[12:]))
	h.bits = int(binary.LittleEndian.Uint16(data[14:]))
	if h.channels < 1 || h.channels > audio.OutputChannels {
		return fmt.Errorf("unsupported channel count %d", h.channels)
	}
	if h.sampleRate <= 0 || h.blockAlign <= 0 || h.bits%8 != 0 {
		return fmt.Errorf("invalid WAV format: rate=%d align=%d bits=%d", h.sampleRate, h.blockAlign, h.bits)
	}
	if h.blockAlign < h.channels*h.bits/8 {
		return fmt.Errorf("invalid block alignment %d", h.blockAlign)
	}
	switch h.format {
	case wavFormatPCM:
		if h.bits != 8 && h.bits != 16 && h.bits != 24 && h.bits != 32 {
			return fmt.Errorf("unsupported PCM bit depth %d", h.bits)
		}
	case wavFormatFloat:
		if h.bits != 32 && h.bits != 64 {
			return fmt.Errorf("unsupported float bit depth %d", h.bits)
		}
	default:
		return fmt.Errorf("unsupported WAV encoding %d", h.format)
	}
	return nil
}

func (h wavHeader) sample(data []byte) (float32, error) {
	if h.format == wavFormatFloat {
		if h.bits == 32 {
			return math.Float32frombits(binary.LittleEndian.Uint32(data)), nil
		}
		return float32(math.Float64frombits(binary.LittleEndian.Uint64(data))), nil
	}
	switch h.bits {
	case 8:
		return float32(int(data[0])-128) / 128, nil
	case 16:
		return audio.NormalizeSigned(int16(binary.LittleEndian.Uint16(data)), 16), nil
	case 24:
		value := int32(data[0]) | int32(data[1])<<8 | int32(data[2])<<16
		if value&0x800000 != 0 {
			value |= ^int32(0xffffff)
		}
		return audio.NormalizeSigned(value, 24), nil
	case 32:
		return audio.NormalizeSigned(int32(binary.LittleEndian.Uint32(data)), 32), nil
	default:
		return 0, fmt.Errorf("unsupported PCM bit depth %d", h.bits)
	}
}
