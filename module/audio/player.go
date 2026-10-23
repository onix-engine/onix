package moduleaudio

import (
	"time"

	"github.com/diiyw/onix/audio"
	"github.com/diiyw/onix/module"
)

type audioPlayback interface {
	Play() error
	Pause() error
	Stop() error
	IsPlaying() bool
	Position() time.Duration
	Duration() time.Duration
	Seek(time.Duration) error
	Volume() float64
	SetVolume(float64) error
	Err() error
	Close() error
}

// Player owns one playback cursor. Decoded clips may be shared, while
// playback state stays private and follows the owning scene's lifetime.
type Player struct {
	context           *module.Context
	player            audioPlayback
	wanted, suspended bool
}

func NewPlayer(c *module.Context, resource string) (*Player, error) {
	clip, err := module.Resource[*audio.Clip](c, "audio", resource)
	if err != nil {
		return nil, err
	}
	player, err := clip.NewPlayer()
	if err != nil {
		return nil, err
	}
	p := &Player{context: c, player: player, suspended: !c.Active()}
	if err := module.Own(c, p); err != nil {
		_ = player.Close()
		return nil, err
	}
	return p, nil
}

func (p *Player) usable() bool { return p != nil && p.context.Alive() && p.player != nil }
func (p *Player) Play() error {
	if !p.usable() {
		return module.ErrDisposed
	}
	p.wanted = true
	if p.suspended {
		return nil
	}
	return p.player.Play()
}
func (p *Player) Pause() error {
	if !p.usable() {
		return module.ErrDisposed
	}
	p.wanted = false
	return p.player.Pause()
}
func (p *Player) Stop() error {
	if !p.usable() {
		return module.ErrDisposed
	}
	p.wanted = false
	return p.player.Stop()
}
func (p *Player) IsPlaying() bool { return p.usable() && !p.suspended && p.player.IsPlaying() }
func (p *Player) Position() time.Duration {
	if !p.usable() {
		return 0
	}
	return p.player.Position()
}
func (p *Player) Duration() time.Duration {
	if !p.usable() {
		return 0
	}
	return p.player.Duration()
}
func (p *Player) Seek(position time.Duration) error {
	if !p.usable() {
		return module.ErrDisposed
	}
	return p.player.Seek(position)
}
func (p *Player) Volume() float64 {
	if !p.usable() {
		return 0
	}
	return p.player.Volume()
}
func (p *Player) SetVolume(volume float64) error {
	if !p.usable() {
		return module.ErrDisposed
	}
	return p.player.SetVolume(volume)
}
func (p *Player) Err() error {
	if !p.usable() {
		return module.ErrDisposed
	}
	return p.player.Err()
}
func (p *Player) Close() error {
	if p == nil || p.player == nil {
		return nil
	}
	err := p.player.Close()
	p.player, p.wanted = nil, false
	module.Disown(p.context, p)
	return err
}
func (p *Player) Activate(active bool) error {
	if p.player == nil || active == !p.suspended {
		return nil
	}
	if !active {
		p.wanted = p.player.IsPlaying()
		p.suspended = true
		return p.player.Pause()
	}
	p.suspended = false
	if p.wanted {
		return p.player.Play()
	}
	return nil
}
