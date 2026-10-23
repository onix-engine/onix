//go:build !js

package audio

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

var nativeDevice struct {
	once sync.Once
	mu   sync.Mutex
	ctx  *oto.Context
	err  error
}

type nativeSource struct {
	mu     sync.Mutex
	reader io.ReadSeekCloser
}

func (s *nativeSource) Read(buffer []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reader == nil {
		return 0, io.EOF
	}
	return s.reader.Read(buffer)
}

func (s *nativeSource) Seek(offset int64, whence int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reader == nil {
		return 0, io.ErrClosedPipe
	}
	return s.reader.Seek(offset, whence)
}

func (s *nativeSource) offset() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reader == nil {
		return 0
	}
	position, _ := s.reader.Seek(0, io.SeekCurrent)
	return position
}

func (s *nativeSource) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reader == nil {
		return nil
	}
	err := s.reader.Close()
	s.reader = nil
	return err
}

type nativePlayer struct {
	clip   *Clip
	source *nativeSource
	player *oto.Player
	volume float64
	closed bool
}

func newPlatformPlayer(clip *Clip) (platformPlayer, error) {
	reader := io.ReadSeekCloser(readSeekNopCloser{ReadSeeker: bytes.NewReader(clip.pcm)})
	if clip.Streaming() {
		stream, err := clip.openStreamingReader()
		if err != nil {
			return nil, err
		}
		reader = stream
	}
	return &nativePlayer{
		clip:   clip,
		source: &nativeSource{reader: reader},
		volume: 1,
	}, nil
}

func nativeContext() (*oto.Context, error) {
	nativeDevice.once.Do(func() {
		ctx, _, err := oto.NewContext(&oto.NewContextOptions{
			SampleRate:      outputSampleRate,
			ChannelCount:    outputChannels,
			Format:          oto.FormatFloat32LE,
			ApplicationName: "Onix",
		})
		nativeDevice.mu.Lock()
		nativeDevice.ctx = ctx
		nativeDevice.err = err
		nativeDevice.mu.Unlock()
	})
	nativeDevice.mu.Lock()
	defer nativeDevice.mu.Unlock()
	return nativeDevice.ctx, nativeDevice.err
}

func (p *nativePlayer) ensurePlayer() error {
	if p.closed {
		return errPlayerClosed
	}
	if p.player != nil {
		return nil
	}
	ctx, err := nativeContext()
	if err != nil {
		return err
	}
	p.player = ctx.NewPlayer(p.source)
	p.player.SetVolume(p.volume)
	return nil
}

func (p *nativePlayer) Play() error {
	if err := p.ensurePlayer(); err != nil {
		return err
	}
	p.player.Play()
	return p.Err()
}

func (p *nativePlayer) Pause() error {
	if p.closed {
		return errPlayerClosed
	}
	if p.player != nil {
		p.player.Pause()
	}
	return p.Err()
}

func (p *nativePlayer) IsPlaying() bool {
	return !p.closed && p.player != nil && p.player.IsPlaying()
}

func (p *nativePlayer) Position() time.Duration {
	if p.closed {
		return 0
	}
	offset := p.source.offset()
	if p.player != nil {
		offset -= int64(p.player.BufferedSize())
	}
	offset = max(int64(0), min(offset, p.clip.length))
	return time.Duration(offset) * time.Second / bytesPerSecond
}

func (p *nativePlayer) Seek(position time.Duration) error {
	if p.closed {
		return errPlayerClosed
	}
	offset := int64(position) * bytesPerSecond / int64(time.Second)
	offset -= offset % bytesPerFrame
	if p.player != nil {
		_, err := p.player.Seek(offset, io.SeekStart)
		return err
	}
	_, err := p.source.Seek(offset, io.SeekStart)
	return err
}

func (p *nativePlayer) Volume() float64 {
	return p.volume
}

func (p *nativePlayer) SetVolume(volume float64) error {
	if p.closed {
		return errPlayerClosed
	}
	p.volume = volume
	if p.player != nil {
		p.player.SetVolume(volume)
	}
	return p.Err()
}

func (p *nativePlayer) Err() error {
	if p.closed {
		return errPlayerClosed
	}
	if p.player != nil {
		if err := p.player.Err(); err != nil {
			return err
		}
	}
	nativeDevice.mu.Lock()
	ctx := nativeDevice.ctx
	nativeDevice.mu.Unlock()
	if ctx != nil {
		return ctx.Err()
	}
	return nil
}

func (p *nativePlayer) Close() error {
	if p.closed {
		return nil
	}
	var playerErr error
	if p.player != nil {
		p.player.Pause()
		playerErr = p.player.Close()
		p.player = nil
	}
	var sourceErr error
	if p.source != nil {
		sourceErr = p.source.close()
	}
	p.closed = true
	p.clip = nil
	p.source = nil
	return errors.Join(playerErr, sourceErr)
}

func suspendPlatform() error {
	nativeDevice.mu.Lock()
	ctx := nativeDevice.ctx
	nativeDevice.mu.Unlock()
	if ctx == nil {
		return nil
	}
	return ctx.Suspend()
}

func resumePlatform() error {
	nativeDevice.mu.Lock()
	ctx := nativeDevice.ctx
	nativeDevice.mu.Unlock()
	if ctx == nil {
		return nil
	}
	return ctx.Resume()
}
