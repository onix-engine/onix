package mp3

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"testing"

	"github.com/diiyw/onix/audio"
)

func TestPCM16StereoStreamResamplesAndSeeks(t *testing.T) {
	var source bytes.Buffer
	for _, samples := range [][2]int16{{-32768, 0}, {0, 16384}, {32767, -16384}} {
		for _, sample := range samples {
			if err := binary.Write(&source, binary.LittleEndian, sample); err != nil {
				t.Fatal(err)
			}
		}
	}
	stream := newPCM16StereoStream(bytes.NewReader(source.Bytes()), 24000, 3)
	output, err := io.ReadAll(stream)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(output), 6*audio.BytesPerFrame; got != want {
		t.Fatalf("output size = %d, want %d", got, want)
	}
	if got := streamSample(output, 0, 0); got != -1 {
		t.Fatalf("first sample = %f, want -1", got)
	}
	if got := streamSample(output, 1, 0); math.Abs(float64(got+0.5)) > .0001 {
		t.Fatalf("interpolated sample = %f, want -0.5", got)
	}
	if _, err := stream.Seek(2*audio.BytesPerFrame, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	frame := make([]byte, audio.BytesPerFrame)
	if _, err := io.ReadFull(stream, frame); err != nil {
		t.Fatal(err)
	}
	if got := streamSample(frame, 0, 1); got != .5 {
		t.Fatalf("sample after seek = %f, want 0.5", got)
	}
}

func TestDecoderRejectsStreamingWAV(t *testing.T) {
	if _, err := (Decoder{}).DecodeAudio(audio.NewMemorySource([]byte("wav")), audio.FormatWAV, true); err == nil {
		t.Fatal("MP3 decoder accepted a WAV stream")
	}
}

func TestPCM16StereoStreamReadDoesNotAllocate(t *testing.T) {
	source := make([]byte, 64*4)
	stream := newPCM16StereoStream(bytes.NewReader(source), audio.OutputSampleRate, 64)
	output := make([]byte, 64*audio.BytesPerFrame)
	allocations := testing.AllocsPerRun(100, func() {
		if _, err := stream.Seek(0, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		if count, err := stream.Read(output); err != nil || count != len(output) {
			t.Fatalf("Read = %d, %v", count, err)
		}
	})
	if allocations != 0 {
		t.Fatalf("stream read allocated %.1f objects", allocations)
	}
}

func streamSample(pcm []byte, frame, channel int) float32 {
	offset := (frame*audio.OutputChannels + channel) * audio.BytesPerSample
	return math.Float32frombits(binary.LittleEndian.Uint32(pcm[offset:]))
}
