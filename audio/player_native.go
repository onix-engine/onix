//go:build !js

package audio

import "time"

// The device mutex protects playback state. A streaming reader is detached
// from the mixer before a seek or close can block on its decoder.
type nativePlayer struct {
	err     error
	output  *outputRun
	stream  *nativeStream
	samples []float32
	frame   int64
	frames  int64
	volume  float64
}

func (owner *Player) newPlatformPlayer(samples []float32) (platformPlayer, error) {
	clip := owner.clip
	p := &nativePlayer{samples: samples, frames: clip.length / bytesPerFrame, volume: 1}
	if samples == nil {
		reader, err := clip.openStreamingReader()
		if err != nil {
			return nil, err
		}
		p.stream = newNativeStream(reader)
		owner.input.Store(reader)
	}
	return p, nil
}

func (p *nativePlayer) Play() error {
	m := &nativeDevice
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, playing := m.players[p]; playing || p.err != nil || p.frame == p.frames {
		return p.errLocked()
	}
	p.output = m.startLocked()
	if err := p.errLocked(); err != nil {
		return err
	}
	if p.stream != nil {
		p.stream.start(p.frames - p.frame)
	}
	m.players[p] = struct{}{}
	if p.output != nil {
		p.output.wake.Signal()
	}
	return nil
}

func (p *nativePlayer) Pause() error {
	nativeDevice.mu.Lock()
	defer nativeDevice.mu.Unlock()
	delete(nativeDevice.players, p)
	return p.errLocked()
}

func (p *nativePlayer) IsPlaying() bool {
	nativeDevice.mu.Lock()
	defer nativeDevice.mu.Unlock()
	_, playing := nativeDevice.players[p]
	return playing
}

func (p *nativePlayer) Position() time.Duration {
	nativeDevice.mu.Lock()
	defer nativeDevice.mu.Unlock()
	return framesDuration(p.frame)
}

func (p *nativePlayer) Seek(position time.Duration) error {
	frame := p.frames
	if position < framesDuration(frame) {
		frame = int64(position/time.Second)*outputSampleRate + int64(position%time.Second)*outputSampleRate/int64(time.Second)
	}
	var playing bool
	m := &nativeDevice
	m.mu.Lock()
	if s := p.stream; s != nil {
		_, playing = m.players[p]
		delete(m.players, p)
		m.mu.Unlock()
		offset, err := s.seek(frame, p.frames, playing)
		m.mu.Lock()
		if err != nil {
			p.err = err
			m.mu.Unlock()
			return err
		}
		frame = offset
	}
	p.frame, p.err = frame, nil
	m.mu.Unlock()
	if playing {
		return p.Play()
	}
	return nil
}

func (p *nativePlayer) Volume() float64 {
	nativeDevice.mu.Lock()
	defer nativeDevice.mu.Unlock()
	return p.volume
}

func (p *nativePlayer) SetVolume(volume float64) error {
	nativeDevice.mu.Lock()
	defer nativeDevice.mu.Unlock()
	p.volume = volume
	return p.errLocked()
}

func (p *nativePlayer) Err() error {
	nativeDevice.mu.Lock()
	defer nativeDevice.mu.Unlock()
	return p.errLocked()
}

func (p *nativePlayer) errLocked() error {
	if p.err != nil {
		return p.err
	}
	if p.output != nil {
		return p.output.err
	}
	return nil
}

func (p *nativePlayer) Close() error {
	nativeDevice.mu.Lock()
	delete(nativeDevice.players, p)
	s := p.stream
	p.samples, p.stream = nil, nil
	nativeDevice.mu.Unlock()
	if s != nil {
		return s.close()
	}
	return nil
}

func (p *nativePlayer) mix(out []float32) int {
	count := int(min(p.frames-p.frame, int64(len(out)/outputChannels))) * outputChannels
	gain := float32(p.volume)
	if p.stream == nil {
		start := p.frame * outputChannels
		for i, sample := range p.samples[start : start+int64(count)] {
			out[i] += gain * sample
		}
	} else {
		count, p.err = p.stream.mix(out[:count], gain)
	}
	p.frame += int64(count / outputChannels)
	return count
}
