//go:build js && wechat

package audio

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"sync"
	"sync/atomic"
	"syscall/js"
	"time"
)

var wechatStreamFiles struct {
	sync.Mutex
	manager   js.Value
	directory string
}

var wechatStreamPlayers struct {
	sync.Mutex
	values    map[*wechatStreamPlayer]struct{}
	suspended atomic.Bool
}

type wechatStreamPlayer struct {
	mu        sync.Mutex
	media     js.Value
	ended     js.Func
	failed    js.Func
	duration  float64
	offset    float64
	volume    float64
	playing   bool
	suspended bool
	closed    bool
	err       error
}

func newStreamingWebPlayer(clip *Clip) (platformPlayer, error) {
	source, err := wechatStreamSource(clip)
	if err != nil {
		return nil, err
	}
	media := js.Global().Get("wx").Call("createInnerAudioContext")
	if !media.Truthy() {
		return nil, fmt.Errorf("audio: WeChat InnerAudioContext is unavailable")
	}
	player := &wechatStreamPlayer{
		media:    media,
		duration: clip.duration.Seconds(),
		volume:   1,
	}
	player.ended = js.FuncOf(func(js.Value, []js.Value) any {
		player.mu.Lock()
		if !player.closed {
			player.playing = false
			player.suspended = false
			player.offset = player.duration
		}
		player.mu.Unlock()
		return nil
	})
	player.failed = js.FuncOf(func(_ js.Value, arguments []js.Value) any {
		message := "unknown error"
		if len(arguments) != 0 {
			if value := arguments[0].Get("errMsg"); value.Type() == js.TypeString {
				message = value.String()
			}
		}
		player.mu.Lock()
		if !player.closed {
			player.playing = false
			player.suspended = false
			player.err = fmt.Errorf("audio: WeChat stream: %s", message)
		}
		player.mu.Unlock()
		return nil
	})
	media.Call("onEnded", player.ended)
	media.Call("onError", player.failed)
	media.Set("autoplay", false)
	media.Set("loop", false)
	media.Set("volume", player.volume)
	media.Set("src", source)
	registerWechatStreamPlayer(player)
	return player, nil
}

func (p *wechatStreamPlayer) Play() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errPlayerClosed
	}
	if p.playing {
		return nil
	}
	if p.offset >= p.duration {
		p.offset = 0
		p.media.Call("seek", 0)
	}
	p.playing = true
	if wechatStreamingSuspended() {
		p.suspended = true
		return nil
	}
	p.media.Call("play")
	return nil
}

func (p *wechatStreamPlayer) Pause() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errPlayerClosed
	}
	if p.playing {
		if !p.suspended {
			p.offset = p.positionLocked()
			p.media.Call("pause")
		}
		p.playing = false
		p.suspended = false
	}
	return nil
}

func (p *wechatStreamPlayer) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.closed && p.playing
}

func (p *wechatStreamPlayer) Position() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0
	}
	return time.Duration(p.positionLocked() * float64(time.Second))
}

func (p *wechatStreamPlayer) Seek(position time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errPlayerClosed
	}
	p.offset = position.Seconds()
	p.media.Call("seek", p.offset)
	return nil
}

func (p *wechatStreamPlayer) Volume() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0
	}
	return p.volume
}

func (p *wechatStreamPlayer) SetVolume(volume float64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errPlayerClosed
	}
	p.volume = volume
	p.media.Set("volume", volume)
	return nil
}

func (p *wechatStreamPlayer) Err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errPlayerClosed
	}
	return p.err
}

func (p *wechatStreamPlayer) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.media.Call("offEnded", p.ended)
	p.media.Call("offError", p.failed)
	p.media.Call("destroy")
	p.playing = false
	p.suspended = false
	p.closed = true
	p.media = js.Undefined()
	p.mu.Unlock()
	unregisterWechatStreamPlayer(p)
	p.ended.Release()
	p.failed.Release()
	return nil
}

func (p *wechatStreamPlayer) positionLocked() float64 {
	if !p.suspended && p.media.Truthy() {
		if current := p.media.Get("currentTime"); current.Type() == js.TypeNumber {
			p.offset = current.Float()
		}
	}
	return max(0, min(p.offset, p.duration))
}

func wechatStreamSource(clip *Clip) (string, error) {
	wechatStreamFiles.Lock()
	defer wechatStreamFiles.Unlock()
	if clip.streamSource != "" {
		return clip.streamSource, nil
	}
	if !wechatStreamFiles.manager.Truthy() {
		wx := js.Global().Get("wx")
		if !wx.Truthy() {
			return "", fmt.Errorf("audio: WeChat API is unavailable")
		}
		wechatStreamFiles.manager = wx.Call("getFileSystemManager")
		if !wechatStreamFiles.manager.Truthy() {
			return "", fmt.Errorf("audio: WeChat filesystem is unavailable")
		}
		wechatStreamFiles.directory = path.Join(wx.Get("env").Get("USER_DATA_PATH").String(), "onix-audio")
		if _, err := callWechatAudio(func() js.Value {
			wechatStreamFiles.manager.Call("mkdirSync", wechatStreamFiles.directory, true)
			return js.Undefined()
		}); err != nil && !wechatAudioPathExists(wechatStreamFiles.manager, wechatStreamFiles.directory, -1) {
			return "", fmt.Errorf("audio: create WeChat stream cache: %w", err)
		}
	}
	encoded, release, err := clip.borrowEncoded()
	if err != nil {
		return "", err
	}
	defer release()
	digest := sha256.Sum256(encoded)
	var name [sha256.Size * 2]byte
	hex.Encode(name[:], digest[:])
	clip.streamSource = path.Join(wechatStreamFiles.directory, string(name[:])+"."+string(clip.format))
	if wechatAudioPathExists(wechatStreamFiles.manager, clip.streamSource, len(encoded)) {
		return clip.streamSource, nil
	}
	data := js.Global().Get("Uint8Array").New(len(encoded))
	js.CopyBytesToJS(data, encoded)
	if _, err := callWechatAudio(func() js.Value {
		wechatStreamFiles.manager.Call("writeFileSync", clip.streamSource, data)
		return js.Undefined()
	}); err != nil {
		clip.streamSource = ""
		return "", fmt.Errorf("audio: cache WeChat stream: %w", err)
	}
	return clip.streamSource, nil
}

func wechatAudioPathExists(manager js.Value, name string, size int) bool {
	exists, err := callWechatAudio(func() bool {
		info := manager.Call("statSync", name)
		return size < 0 || info.Get("size").Int() == size
	})
	return err == nil && exists
}

func callWechatAudio[T any](operation func() T) (value T, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%v", recovered)
		}
	}()
	return operation(), nil
}

func registerWechatStreamPlayer(player *wechatStreamPlayer) {
	wechatStreamPlayers.Lock()
	if wechatStreamPlayers.values == nil {
		wechatStreamPlayers.values = make(map[*wechatStreamPlayer]struct{})
	}
	wechatStreamPlayers.values[player] = struct{}{}
	wechatStreamPlayers.Unlock()
}

func unregisterWechatStreamPlayer(player *wechatStreamPlayer) {
	wechatStreamPlayers.Lock()
	delete(wechatStreamPlayers.values, player)
	wechatStreamPlayers.Unlock()
}

func wechatStreamingSuspended() bool {
	return wechatStreamPlayers.suspended.Load()
}

func suspendStreamingPlatform() {
	wechatStreamPlayers.suspended.Store(true)
	wechatStreamPlayers.Lock()
	for player := range wechatStreamPlayers.values {
		player.mu.Lock()
		if !player.closed && player.playing && !player.suspended {
			player.offset = player.positionLocked()
			player.media.Call("pause")
			player.suspended = true
		}
		player.mu.Unlock()
	}
	wechatStreamPlayers.Unlock()
}

func resumeStreamingPlatform() {
	wechatStreamPlayers.suspended.Store(false)
	wechatStreamPlayers.Lock()
	for player := range wechatStreamPlayers.values {
		player.mu.Lock()
		if !player.closed && player.playing && player.suspended {
			player.media.Call("seek", player.offset)
			player.media.Call("play")
			player.suspended = false
		}
		player.mu.Unlock()
	}
	wechatStreamPlayers.Unlock()
}
