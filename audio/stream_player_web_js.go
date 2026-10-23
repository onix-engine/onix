//go:build js && !wechat

package audio

import (
	"sync"
	"syscall/js"
	"time"
)

type webStreamPlayer struct {
	mu       sync.Mutex
	media    js.Value
	source   js.Value
	gain     js.Value
	ended    js.Func
	url      string
	duration float64
	offset   float64
	volume   float64
	playing  bool
	closed   bool
	err      error
}

func newStreamingWebPlayer(clip *Clip) (platformPlayer, error) {
	encoded, release, err := clip.borrowEncoded()
	if err != nil {
		return nil, err
	}
	defer release()
	data := js.Global().Get("Uint8Array").New(len(encoded))
	js.CopyBytesToJS(data, encoded)
	parts := js.Global().Get("Array").New()
	parts.Call("push", data)
	options := js.Global().Get("Object").New()
	options.Set("type", "audio/mpeg")
	blob := js.Global().Get("Blob").New(parts, options)
	url := js.Global().Get("URL").Call("createObjectURL", blob).String()
	media := js.Global().Get("Audio").New()
	media.Set("src", url)
	media.Set("preload", "auto")
	player := &webStreamPlayer{media: media, url: url, duration: clip.duration.Seconds(), volume: 1}
	player.ended = js.FuncOf(func(js.Value, []js.Value) any {
		player.mu.Lock()
		player.playing = false
		player.offset = player.duration
		player.mu.Unlock()
		return nil
	})
	media.Call("addEventListener", "ended", player.ended)
	return player, nil
}

func (p *webStreamPlayer) Play() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errPlayerClosed
	}
	if p.playing {
		return nil
	}
	p.playing = true
	context, err := webAudioContextForPlayback(p)
	if err != nil {
		p.playing = false
		p.err = err
		return err
	}
	if context.Truthy() {
		p.startLocked(context)
	}
	return nil
}

func (p *webStreamPlayer) Pause() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errPlayerClosed
	}
	if p.playing {
		p.offset = p.positionLocked()
		p.media.Call("pause")
		p.playing = false
		removeWaitingWebPlayer(p)
	}
	return nil
}

func (p *webStreamPlayer) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.closed && p.playing
}

func (p *webStreamPlayer) Position() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0
	}
	return time.Duration(p.positionLocked() * float64(time.Second))
}

func (p *webStreamPlayer) Seek(position time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errPlayerClosed
	}
	p.offset = position.Seconds()
	p.media.Set("currentTime", p.offset)
	return nil
}

func (p *webStreamPlayer) Volume() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0
	}
	return p.volume
}

func (p *webStreamPlayer) SetVolume(volume float64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errPlayerClosed
	}
	p.volume = volume
	if p.gain.Truthy() {
		p.gain.Get("gain").Set("value", volume)
	}
	return nil
}

func (p *webStreamPlayer) Err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errPlayerClosed
	}
	return p.err
}

func (p *webStreamPlayer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	removeWaitingWebPlayer(p)
	p.media.Call("pause")
	p.media.Call("removeEventListener", "ended", p.ended)
	p.ended.Release()
	if p.source.Truthy() {
		p.source.Call("disconnect")
	}
	if p.gain.Truthy() {
		p.gain.Call("disconnect")
	}
	js.Global().Get("URL").Call("revokeObjectURL", p.url)
	p.playing = false
	p.closed = true
	p.media = js.Undefined()
	p.source = js.Undefined()
	p.gain = js.Undefined()
	return nil
}

func (p *webStreamPlayer) finishDeferredPlayback(context js.Value, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || !p.playing {
		return
	}
	if err != nil {
		p.playing = false
		p.err = err
		return
	}
	p.startLocked(context)
}

func (p *webStreamPlayer) startLocked(context js.Value) {
	if !p.source.Truthy() {
		p.source = context.Call("createMediaElementSource", p.media)
		p.gain = context.Call("createGain")
		p.gain.Get("gain").Set("value", p.volume)
		p.source.Call("connect", p.gain)
		p.gain.Call("connect", context.Get("destination"))
	}
	if p.offset >= p.duration {
		p.offset = 0
	}
	p.media.Set("currentTime", p.offset)
	p.media.Call("play")
}

func (p *webStreamPlayer) positionLocked() float64 {
	if p.media.Truthy() {
		p.offset = p.media.Get("currentTime").Float()
	}
	return max(0, min(p.offset, p.duration))
}

func suspendStreamingPlatform() {}

func resumeStreamingPlatform() {}
