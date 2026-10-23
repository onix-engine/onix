//go:build !js

package audio

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"
)

func TestNativeSourceAfterClose(t *testing.T) {
	source := nativeSource{
		reader: readSeekNopCloser{ReadSeeker: bytes.NewReader([]byte{1, 2, 3})},
	}
	if err := source.close(); err != nil {
		t.Fatal(err)
	}
	if count, err := source.Read(make([]byte, 1)); count != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("Read after close = (%d, %v), want (0, EOF)", count, err)
	}
	if position, err := source.Seek(0, io.SeekStart); position != 0 || !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Seek after close = (%d, %v), want (0, %v)", position, err, io.ErrClosedPipe)
	}
	if position := source.offset(); position != 0 {
		t.Fatalf("offset after close = %d, want 0", position)
	}
}

func TestPlayerStateBeforeDeviceInitialization(t *testing.T) {
	clip := &Clip{
		format:   FormatWAV,
		pcm:      make([]byte, outputSampleRate*bytesPerFrame),
		length:   outputSampleRate * bytesPerFrame,
		duration: time.Second,
	}
	player, err := clip.NewPlayer()
	if err != nil {
		t.Fatal(err)
	}
	if err := player.SetVolume(2); err != nil {
		t.Fatal(err)
	}
	if got := player.Volume(); got != 1 {
		t.Fatalf("volume = %f, want 1", got)
	}
	if err := player.Seek(250 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if got := player.Position(); got != 250*time.Millisecond {
		t.Fatalf("position = %s, want 250ms", got)
	}
	if err := player.Stop(); err != nil {
		t.Fatal(err)
	}
	if got := player.Position(); got != 0 {
		t.Fatalf("position after Stop = %s, want 0", got)
	}
	if err := player.Close(); err != nil {
		t.Fatal(err)
	}
	if err := player.Play(); !errors.Is(err, errPlayerClosed) {
		t.Fatalf("Play after Close error = %v, want %v", err, errPlayerClosed)
	}
}
