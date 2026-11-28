//go:build !js

package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"slices"
	"testing"
	"testing/synctest"
	"time"
)

func TestPlayerStateBeforeDeviceInitialization(t *testing.T) {
	clip := &Clip{
		format:   FormatWAV,
		samples:  make([]float32, outputSampleRate*outputChannels),
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

func TestNativeMixStaticAndStreaming(t *testing.T) {
	static := &nativePlayer{samples: []float32{1, -1, 0.5, -0.5}, frames: 2, volume: 0.5}
	var data []byte
	for _, v := range []float32{0.75, -0.75, 0.5, -0.5, 0.125, -0.125} {
		data = binary.LittleEndian.AppendUint32(data, math.Float32bits(v))
	}
	source := newTrackedAudioSource(data)
	clip, err := NewStreamingClip(FormatMP3, source, outputSampleRate, 3, Source.Open)
	if err != nil {
		t.Fatal(err)
	}
	player, err := clip.NewPlayer()
	if err != nil {
		t.Fatal(err)
	}
	stream := player.platform.(*nativePlayer)
	stream.stream.start(stream.frames)
	waitForNativeDecode(stream.stream)
	if err := clip.Close(); err != nil {
		t.Fatal(err)
	}
	mixer := nativeMixer{
		players: map[*nativePlayer]struct{}{static: {}, stream: {}},
	}
	var out [4]float32
	if n := mixer.mix(out[:]); n != 4 || out != [4]float32{1, -1, 0.75, -0.75} {
		t.Fatalf("mixed %d samples: %v", n, out)
	}
	if n := mixer.mix(out[:]); n != 2 || out != [4]float32{0.125, -0.125, 0, 0} {
		t.Fatalf("mixed tail of %d samples: %v", n, out)
	}
	if len(mixer.players) != 0 || player.Position() != clip.Duration() || stream.err != nil {
		t.Fatalf("EOF: active=%d position=%v error=%v", len(mixer.players), player.Position(), stream.err)
	}
	if err := player.Stop(); err != nil || player.Position() != 0 {
		t.Fatalf("rewind: position=%v error=%v", player.Position(), err)
	}
	if err := player.Close(); err != nil || source.closes.Load() != 1 {
		t.Fatalf("close: source closed %d times, error=%v", source.closes.Load(), err)
	}
}

func TestNativeMixTruncatedStream(t *testing.T) {
	// One full stereo frame followed by only part of the second frame.
	data := make([]byte, bytesPerFrame+2)
	p := &nativePlayer{stream: newNativeStream(&clipStreamReader{ReadSeekCloser: readSeekNopCloser{bytes.NewReader(data)}}), frames: 2, volume: 1}
	p.stream.start(p.frames)
	waitForNativeDecode(p.stream)
	var out [4]float32
	if got := p.mix(out[:]); got != outputChannels || p.frame != 1 || !errors.Is(p.err, io.ErrUnexpectedEOF) {
		t.Fatalf("mixed %d samples, position=%d error=%v", got, p.frame, p.err)
	}
}

func TestNativeStaticPlayerRetainsSamples(t *testing.T) {
	samples := []float32{0.25, -0.5}
	clip, err := NewClip(FormatWAV, DecodedAudio{samples, outputSampleRate, outputChannels})
	if err != nil {
		t.Fatal(err)
	}
	player, err := clip.NewPlayer()
	if err != nil {
		t.Fatal(err)
	}
	defer player.Close()
	if err := clip.Close(); err != nil {
		t.Fatal(err)
	}
	clear(samples)
	var out [2]float32
	player.platform.(*nativePlayer).mix(out[:])
	if !slices.Equal(out[:], []float32{0.25, -0.5}) {
		t.Fatalf("samples after clip closed = %v", out)
	}
}

func TestNativeMixWhileStreamIsWaiting(t *testing.T) {
	input, output := io.Pipe()
	reader := newNativeStream(&clipStreamReader{ReadSeekCloser: &pipePCMReader{PipeReader: input}})
	reader.start(1)
	t.Cleanup(func() {
		reader.close()
		output.Close()
	})
	stream := &nativePlayer{stream: reader, frames: 1, volume: 1}
	other := &nativePlayer{samples: []float32{0.25, -0.25}, frames: 1, volume: 1}
	m := nativeMixer{players: map[*nativePlayer]struct{}{stream: {}, other: {}}}
	var out [2]float32
	if n := m.mix(out[:]); n != 2 || out != [2]float32{0.25, -0.25} || stream.frame != 0 {
		t.Fatalf("mix with pending stream = %v, samples=%d, stream frame=%d", out, n, stream.frame)
	}
}

type pipePCMReader struct{ *io.PipeReader }

func (*pipePCMReader) Seek(int64, int) (int64, error) { return 0, io.ErrClosedPipe }

func waitForNativeDecode(s *nativeStream) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.remaining > 0 || s.reading {
		s.changed.Wait()
	}
}

type delayedPCMReader struct {
	pipePCMReader
	unwinding, release chan struct{}
}

func (r *delayedPCMReader) Read(dst []byte) (int, error) {
	n, err := r.PipeReader.Read(dst)
	close(r.unwinding)
	<-r.release
	return n, err
}

func TestNativeCloseInterruptsReadDuringSeek(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		if err := Suspend(); err != nil {
			t.Fatal(err)
		}
		defer Resume()
		input, writer := io.Pipe()
		defer writer.Close()
		r := &delayedPCMReader{pipePCMReader{input}, make(chan struct{}), make(chan struct{})}
		source := newTrackedAudioSource([]byte{1})
		clip, err := NewStreamingClip(FormatWAV, source, outputSampleRate, outputSampleRate,
			func(Source) (io.ReadSeekCloser, error) { return r, nil })
		if err != nil {
			t.Fatal(err)
		}
		p, err := clip.NewPlayer()
		if err != nil {
			t.Fatal(err)
		}
		if err := p.Play(); err != nil {
			t.Fatal(err)
		}
		if err := clip.Close(); err != nil {
			t.Fatal(err)
		}
		synctest.Wait() // the producer is in Read
		sought := make(chan error, 1)
		go func() { sought <- p.Seek(time.Millisecond) }()
		synctest.Wait() // Seek holds the player lock and awaits the producer
		closed := make(chan error, 1)
		go func() { closed <- p.Close() }()
		<-r.unwinding
		if source.closes.Load() != 0 {
			t.Fatal("released the source before the read returned")
		}
		close(r.release)
		if err := <-sought; !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("seek returned %v", err)
		}
		if err := <-closed; err != nil || source.closes.Load() != 1 {
			t.Fatalf("close returned %v, source closed %d times", err, source.closes.Load())
		}
	})
}

func TestNativePlayingSeekReusesQueue(t *testing.T) {
	if err := Suspend(); err != nil {
		t.Fatal(err)
	}
	defer Resume()
	data := make([]byte, outputSampleRate*bytesPerFrame)
	lastBlock := len(data) - outputBlockFrames*bytesPerFrame
	binary.LittleEndian.PutUint32(data[lastBlock:], math.Float32bits(0.75))
	src := NewMemorySource(data)
	clip, err := NewStreamingClip(FormatWAV, src, outputSampleRate, outputSampleRate, Source.Open)
	if err != nil {
		t.Fatal(err)
	}
	defer clip.Close()
	p, err := clip.NewPlayer()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	if allocs := testing.AllocsPerRun(100, func() {
		if err := p.Seek(time.Millisecond); err != nil {
			t.Fatal(err)
		}
		if err := p.Seek(time.Second - 10*time.Millisecond); err != nil {
			t.Fatal(err)
		}
		waitForNativeDecode(p.platform.(*nativePlayer).stream)
	}); allocs != 0 {
		t.Fatalf("warm seek allocated %g objects", allocs)
	}
	native := p.platform.(*nativePlayer)
	var frame [2]float32
	if got := native.mix(frame[:]); got != 2 || frame != [2]float32{0.75, 0} {
		t.Fatalf("first frame after seek = %v, samples = %d", frame, got)
	}
}

func TestNativeMixerAllocs(t *testing.T) {
	p := &nativePlayer{samples: make([]float32, 2*outputBlockFrames*outputChannels), frames: 2 * outputBlockFrames, volume: 0.5}
	m := nativeMixer{players: map[*nativePlayer]struct{}{p: {}}}
	var out [outputBlockFrames * outputChannels]float32
	allocs := testing.AllocsPerRun(100, func() {
		p.frame = 0
		m.mix(out[:])
	})
	if allocs != 0 {
		t.Fatalf("allocations per mixed block = %v", allocs)
	}
}
