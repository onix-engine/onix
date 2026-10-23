package audio_test

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"

	"github.com/diiyw/onix/audio"
	"github.com/diiyw/onix/audio/codec"
	flaccodec "github.com/mewkiz/flac"
	"github.com/mewkiz/flac/frame"
	"github.com/mewkiz/flac/meta"
)

func TestFormatFromPath(t *testing.T) {
	for path, want := range map[string]audio.Format{
		"music.FLAC": audio.FormatFLAC,
		"music.Mp3":  audio.FormatMP3,
		"music.Ogg":  audio.FormatOGG,
		"music.Wav":  audio.FormatWAV,
	} {
		got, err := audio.FormatFromPath(path)
		if err != nil {
			t.Fatalf("FormatFromPath(%q): %v", path, err)
		}
		if got != want {
			t.Fatalf("FormatFromPath(%q) = %q, want %q", path, got, want)
		}
	}
	if _, err := audio.FormatFromPath("music.aac"); err == nil {
		t.Fatal("FormatFromPath accepted unsupported AAC audio")
	}
}

func TestDecodeWAVResamplesMono(t *testing.T) {
	data := encodeTestWAV(t, 24000, []int16{-32768, -16384, 0, 32767})
	clip, err := codec.DefaultDecoder{}.DecodeAudio(audio.NewMemorySource(data), audio.FormatWAV, false)
	if err != nil {
		t.Fatalf("Decode WAV: %v", err)
	}
	if clip.Format() != audio.FormatWAV {
		t.Fatalf("format = %q, want %q", clip.Format(), audio.FormatWAV)
	}
	if clip.Duration() <= 0 {
		t.Fatal("WAV clip has no duration")
	}
}

func TestDecodeFloatWAV(t *testing.T) {
	var output bytes.Buffer
	output.WriteString("RIFF")
	writeLE(t, &output, uint32(36+8))
	output.WriteString("WAVEfmt ")
	writeLE(t, &output, uint32(16))
	writeLE(t, &output, uint16(3))
	writeLE(t, &output, uint16(1))
	writeLE(t, &output, uint32(audio.OutputSampleRate))
	writeLE(t, &output, uint32(audio.OutputSampleRate*4))
	writeLE(t, &output, uint16(4))
	writeLE(t, &output, uint16(32))
	output.WriteString("data")
	writeLE(t, &output, uint32(8))
	writeLE(t, &output, float32(-0.5))
	writeLE(t, &output, float32(0.5))

	clip, err := codec.DefaultDecoder{}.DecodeAudio(audio.NewMemorySource(output.Bytes()), audio.FormatWAV, false)
	if err != nil {
		t.Fatalf("Decode float WAV: %v", err)
	}
	if clip.Duration() <= 0 {
		t.Fatal("float WAV clip has no duration")
	}
}

func TestDecodeFLAC(t *testing.T) {
	data := encodeTestFLAC(t)
	clip, err := codec.DefaultDecoder{}.DecodeAudio(audio.NewMemorySource(data), audio.FormatFLAC, false)
	if err != nil {
		t.Fatalf("Decode FLAC: %v", err)
	}
	if clip.Duration() <= 0 {
		t.Fatal("FLAC clip has no duration")
	}
}

func TestDecodeVorbis(t *testing.T) {
	data, err := os.ReadFile("testdata/test.ogg")
	if err != nil {
		t.Fatal(err)
	}
	clip, err := codec.DefaultDecoder{}.DecodeAudio(audio.NewMemorySource(data), audio.FormatOGG, false)
	if err != nil {
		t.Fatalf("Decode OGG/Vorbis: %v", err)
	}
	if clip.Format() != audio.FormatOGG || clip.Duration() <= 0 {
		t.Fatalf("invalid OGG/Vorbis clip: format=%q duration=%s", clip.Format(), clip.Duration())
	}
}

func TestDecodeRejectsUnsupportedChannels(t *testing.T) {
	_, err := audio.NewClip(audio.FormatWAV, audio.DecodedAudio{
		Samples:    []float32{0, 0, 0},
		SampleRate: audio.OutputSampleRate,
		Channels:   3,
	})
	if err == nil {
		t.Fatal("prepareSamples accepted three channels")
	}
}

func encodeTestWAV(t *testing.T, sampleRate int, samples []int16) []byte {
	t.Helper()
	var output bytes.Buffer
	dataSize := len(samples) * 2
	output.WriteString("RIFF")
	writeLE(t, &output, uint32(36+dataSize))
	output.WriteString("WAVEfmt ")
	writeLE(t, &output, uint32(16))
	writeLE(t, &output, uint16(1))
	writeLE(t, &output, uint16(1))
	writeLE(t, &output, uint32(sampleRate))
	writeLE(t, &output, uint32(sampleRate*2))
	writeLE(t, &output, uint16(2))
	writeLE(t, &output, uint16(16))
	output.WriteString("data")
	writeLE(t, &output, uint32(dataSize))
	for _, sample := range samples {
		writeLE(t, &output, sample)
	}
	return output.Bytes()
}

func encodeTestFLAC(t *testing.T) []byte {
	t.Helper()
	const frames = 32
	var output bytes.Buffer
	encoder, err := flaccodec.NewEncoder(&output, &meta.StreamInfo{
		BlockSizeMin:  frames,
		BlockSizeMax:  frames,
		SampleRate:    24000,
		NChannels:     1,
		BitsPerSample: 16,
		NSamples:      frames,
	})
	if err != nil {
		t.Fatal(err)
	}
	encoder.EnablePredictionAnalysis(false)
	samples := make([]int32, frames)
	for index := range samples {
		samples[index] = int32(-32768 + index*2048)
	}
	err = encoder.WriteFrame(&frame.Frame{
		Header: frame.Header{
			HasFixedBlockSize: true,
			BlockSize:         frames,
			SampleRate:        24000,
			Channels:          frame.ChannelsMono,
			BitsPerSample:     16,
		},
		Subframes: []*frame.Subframe{{
			SubHeader: frame.SubHeader{Pred: frame.PredVerbatim},
			Samples:   samples,
			NSamples:  frames,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func writeLE(t *testing.T, output *bytes.Buffer, value any) {
	t.Helper()
	if err := binary.Write(output, binary.LittleEndian, value); err != nil {
		t.Fatal(err)
	}
}
