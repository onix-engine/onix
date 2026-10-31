//go:build js

package audio

import (
	"sync"
	"syscall/js"
	"time"
	"unsafe"
)

var webDevice struct {
	mu              sync.Mutex
	ctx             js.Value
	err             error
	waiting         map[deferredWebPlayer]struct{}
	unlock          js.Func
	unlockInstalled bool
}

type deferredWebPlayer interface {
	finishDeferredPlayback(js.Value, error)
}

type webPlayer struct {
	mu            sync.Mutex
	samples       []float32
	context       js.Value
	buffer        js.Value
	gain          js.Value
	source        js.Value
	onEnded       js.Func
	duration      float64
	offset        float64
	started       float64
	volume        float64
	playing       bool
	sourceStarted bool
	closed        bool
	err           error
}

func webAudioContext() (js.Value, error) {
	webDevice.mu.Lock()
	defer webDevice.mu.Unlock()
	return webDevice.ctx, webDevice.err
}

func newPlatformPlayer(clip *Clip) (platformPlayer, error) {
	if err := validateWebAudioContext(); err != nil {
		return nil, err
	}
	if clip.Streaming() {
		return newStreamingWebPlayer(clip)
	}
	return &webPlayer{
		samples:  clip.samples,
		duration: clip.duration.Seconds(),
		volume:   1,
	}, nil
}

func (p *webPlayer) Play() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errPlayerClosed
	}
	if p.playing {
		return nil
	}
	p.playing = true
	if err := p.startPlaybackLocked(); err != nil {
		p.playing = false
		p.err = err
		return err
	}
	return nil
}

func (p *webPlayer) Pause() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errPlayerClosed
	}
	if p.playing {
		if p.sourceStarted {
			p.offset = p.positionLocked()
			p.stopSourceLocked()
		}
		p.playing = false
		removeWaitingWebPlayer(p)
	}
	return nil
}

func (p *webPlayer) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.closed && p.playing
}

func (p *webPlayer) Position() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0
	}
	return time.Duration(p.positionLocked() * float64(time.Second))
}

func (p *webPlayer) Seek(position time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errPlayerClosed
	}
	wasPlaying := p.playing
	if p.sourceStarted {
		p.stopSourceLocked()
	}
	p.offset = position.Seconds()
	if wasPlaying {
		if err := p.startPlaybackLocked(); err != nil {
			p.playing = false
			p.err = err
			return err
		}
	}
	return nil
}

func (p *webPlayer) startPlaybackLocked() error {
	if p.sourceStarted {
		return nil
	}
	if p.offset >= p.duration {
		p.offset = 0
	}
	context, err := webAudioContextForPlayback(p)
	if err != nil || !context.Truthy() {
		return err
	}
	p.startSourceLocked(context)
	return nil
}

func (p *webPlayer) startSourceLocked(context js.Value) {
	p.initializeLocked(context)
	source := p.context.Call("createBufferSource")
	source.Set("buffer", p.buffer)
	source.Call("connect", p.gain)
	var ended js.Func
	ended = js.FuncOf(func(js.Value, []js.Value) any {
		p.mu.Lock()
		if p.source.Equal(source) {
			p.playing = false
			p.sourceStarted = false
			p.offset = p.duration
			p.source = js.Undefined()
			p.onEnded = js.Func{}
		}
		p.mu.Unlock()
		ended.Release()
		return nil
	})
	source.Set("onended", ended)
	p.source = source
	p.onEnded = ended
	p.started = p.context.Get("currentTime").Float()
	p.sourceStarted = true
	source.Call("start", 0, p.offset)
}

func (p *webPlayer) initializeLocked(context js.Value) {
	if p.context.Truthy() {
		return
	}
	p.context = context
	frames := len(p.samples) / outputChannels
	p.buffer = context.Call("createBuffer", outputChannels, frames, outputSampleRate)
	channelSamples := make([]float32, frames)
	bytes := js.Global().Get("Uint8Array").New(frames * bytesPerSample)
	array := js.Global().Get("Float32Array").New(bytes.Get("buffer"), bytes.Get("byteOffset"), frames)
	for channel := range outputChannels {
		for frame := range frames {
			channelSamples[frame] = p.samples[frame*outputChannels+channel]
		}
		if frames != 0 {
			js.CopyBytesToJS(bytes, unsafe.Slice((*byte)(unsafe.Pointer(&channelSamples[0])), frames*bytesPerSample))
		}
		p.buffer.Call("copyToChannel", array, channel, 0)
	}
	p.gain = context.Call("createGain")
	p.gain.Get("gain").Set("value", p.volume)
	p.gain.Call("connect", context.Get("destination"))
	p.samples = nil
}

func (p *webPlayer) Volume() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0
	}
	return p.volume
}

func (p *webPlayer) SetVolume(volume float64) error {
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

func (p *webPlayer) Err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errPlayerClosed
	}
	return p.err
}

func (p *webPlayer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	if p.sourceStarted {
		p.stopSourceLocked()
	}
	removeWaitingWebPlayer(p)
	if p.gain.Truthy() {
		p.gain.Call("disconnect")
	}
	p.playing = false
	p.closed = true
	p.samples = nil
	p.buffer = js.Undefined()
	p.gain = js.Undefined()
	return nil
}

func (p *webPlayer) positionLocked() float64 {
	position := p.offset
	if p.sourceStarted {
		position += p.context.Get("currentTime").Float() - p.started
	}
	return max(0, min(position, p.duration))
}

func (p *webPlayer) stopSourceLocked() {
	p.source.Set("onended", js.Null())
	p.source.Call("stop")
	p.source.Call("disconnect")
	p.onEnded.Release()
	p.source = js.Undefined()
	p.onEnded = js.Func{}
	p.sourceStarted = false
}

func webAudioContextForPlayback(player deferredWebPlayer) (js.Value, error) {
	webDevice.mu.Lock()
	defer webDevice.mu.Unlock()
	if webDevice.ctx.Truthy() || webDevice.err != nil {
		return webDevice.ctx, webDevice.err
	}
	if shouldDeferWebAudioContext() {
		if webDevice.waiting == nil {
			webDevice.waiting = make(map[deferredWebPlayer]struct{})
		}
		webDevice.waiting[player] = struct{}{}
		if !webDevice.unlockInstalled {
			webDevice.unlock = js.FuncOf(func(js.Value, []js.Value) any {
				unlockWebAudio()
				return nil
			})
			installWebAudioUnlock(webDevice.unlock)
			webDevice.unlockInstalled = true
		}
		return js.Undefined(), nil
	}
	webDevice.ctx, webDevice.err = createWebAudioContext()
	if webDevice.err == nil {
		webDevice.ctx.Call("resume")
	}
	return webDevice.ctx, webDevice.err
}

func unlockWebAudio() {
	webDevice.mu.Lock()
	if !webDevice.ctx.Truthy() && webDevice.err == nil {
		webDevice.ctx, webDevice.err = createWebAudioContext()
	}
	context, err := webDevice.ctx, webDevice.err
	waiting := make([]deferredWebPlayer, 0, len(webDevice.waiting))
	for player := range webDevice.waiting {
		waiting = append(waiting, player)
	}
	clear(webDevice.waiting)
	webDevice.mu.Unlock()

	if err == nil {
		context.Call("resume")
	}
	uninstallWebAudioUnlock(webDevice.unlock)
	for _, player := range waiting {
		player.finishDeferredPlayback(context, err)
	}
}

func (p *webPlayer) finishDeferredPlayback(context js.Value, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || !p.playing || p.sourceStarted {
		return
	}
	if err != nil {
		p.playing = false
		p.err = err
		return
	}
	p.startSourceLocked(context)
}

func removeWaitingWebPlayer(player deferredWebPlayer) {
	webDevice.mu.Lock()
	delete(webDevice.waiting, player)
	webDevice.mu.Unlock()
}

func suspendPlatform() error {
	suspendStreamingPlatform()
	context, err := webAudioContext()
	if err != nil {
		return err
	}
	if !context.Truthy() {
		return nil
	}
	context.Call("suspend")
	return nil
}

func resumePlatform() error {
	resumeStreamingPlatform()
	context, err := webAudioContext()
	if err != nil {
		return err
	}
	if !context.Truthy() {
		return nil
	}
	context.Call("resume")
	return nil
}
