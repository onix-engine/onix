package audio

import (
	"errors"
	"sync"
	"time"
)

var errPlayerClosed = errors.New("audio: player is closed")

type platformPlayer interface {
	Play() error
	Pause() error
	IsPlaying() bool
	Position() time.Duration
	Seek(time.Duration) error
	Volume() float64
	SetVolume(float64) error
	Err() error
	Close() error
}

type Player struct {
	mu       sync.Mutex
	clip     *Clip
	platform platformPlayer
}

func (c *Clip) NewPlayer() (*Player, error) {
	if c == nil {
		return nil, errors.New("audio: clip is empty")
	}
	c.mu.Lock()
	closed, empty := c.closed, c.length == 0
	c.mu.Unlock()
	if closed {
		return nil, errors.New("audio: clip is closed")
	}
	if empty {
		return nil, errors.New("audio: clip is empty")
	}
	player, err := newPlatformPlayer(c)
	if err != nil {
		return nil, err
	}
	return &Player{clip: c, platform: player}, nil
}

func (p *Player) Play() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.platform == nil {
		return errPlayerClosed
	}
	return p.platform.Play()
}

func (p *Player) Pause() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.platform == nil {
		return errPlayerClosed
	}
	return p.platform.Pause()
}

func (p *Player) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.platform == nil {
		return errPlayerClosed
	}
	if err := p.platform.Pause(); err != nil {
		return err
	}
	return p.platform.Seek(0)
}

func (p *Player) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.platform != nil && p.platform.IsPlaying()
}

func (p *Player) Position() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.platform == nil {
		return 0
	}
	return p.platform.Position()
}

func (p *Player) Seek(position time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.platform == nil {
		return errPlayerClosed
	}
	position = max(time.Duration(0), min(position, p.clip.duration))
	return p.platform.Seek(position)
}

func (p *Player) Duration() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.clip == nil {
		return 0
	}
	return p.clip.duration
}

func (p *Player) Volume() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.platform == nil {
		return 0
	}
	return p.platform.Volume()
}

func (p *Player) SetVolume(volume float64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.platform == nil {
		return errPlayerClosed
	}
	return p.platform.SetVolume(max(0, min(volume, 1)))
}

func (p *Player) Err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.platform == nil {
		return errPlayerClosed
	}
	return p.platform.Err()
}

func (p *Player) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.platform == nil {
		return nil
	}
	err := p.platform.Close()
	p.platform = nil
	p.clip = nil
	return err
}

func Suspend() error {
	return suspendPlatform()
}

func Resume() error {
	return resumePlatform()
}
