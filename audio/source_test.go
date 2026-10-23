package audio

import (
	"errors"
	"io"
	"sync/atomic"
	"testing"
)

type trackedAudioSource struct {
	*MemorySource
	closes atomic.Int32
}

func newTrackedAudioSource(data []byte) *trackedAudioSource {
	return &trackedAudioSource{MemorySource: NewMemorySource(data)}
}

func (s *trackedAudioSource) Close() error {
	s.closes.Add(1)
	return s.MemorySource.Close()
}

func TestDecodeClipReleasesStaticSource(t *testing.T) {
	source := newTrackedAudioSource([]byte{1})
	clip, err := DecodeClip(FormatWAV, source, func([]byte) (DecodedAudio, error) {
		return DecodedAudio{Samples: []float32{0, 0}, SampleRate: OutputSampleRate, Channels: 2}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if source.closes.Load() != 1 {
		t.Fatalf("source closes = %d, want 1", source.closes.Load())
	}
	if err := clip.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeClipReleasesSourceAfterError(t *testing.T) {
	source := newTrackedAudioSource([]byte{1})
	want := errors.New("decode failed")
	if _, err := DecodeClip(FormatWAV, source, func([]byte) (DecodedAudio, error) {
		return DecodedAudio{}, want
	}); !errors.Is(err, want) {
		t.Fatalf("DecodeClip error = %v, want %v", err, want)
	}
	if source.closes.Load() != 1 {
		t.Fatalf("source closes = %d, want 1", source.closes.Load())
	}
}

func TestStreamingReaderRetainsSourcePastClipClose(t *testing.T) {
	source := newTrackedAudioSource([]byte{1, 2, 3, 4})
	clip, err := NewStreamingClip(FormatMP3, source, OutputSampleRate, 1,
		func(source Source) (io.ReadSeekCloser, error) {
			return source.Open()
		})
	if err != nil {
		t.Fatal(err)
	}
	reader, err := clip.openStreamingReader()
	if err != nil {
		t.Fatal(err)
	}
	if err := clip.Close(); err != nil {
		t.Fatal(err)
	}
	if source.closes.Load() != 0 {
		t.Fatal("Clip.Close released a source with an active reader")
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if source.closes.Load() != 1 {
		t.Fatalf("source closes = %d, want 1", source.closes.Load())
	}
	if _, err := clip.NewPlayer(); err == nil {
		t.Fatal("closed clip created a player")
	}
}
