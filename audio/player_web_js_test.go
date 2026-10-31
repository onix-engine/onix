//go:build js && wasm && !wechat

package audio

import (
	"io"
	"runtime"
	"slices"
	"syscall/js"
	"testing"
	"time"
)

func TestWebClipKeepsOwnedFloatSamples(t *testing.T) {
	source := []float32{0.25, -0.5, 0.75, -1}
	clip, err := NewClip(FormatWAV, DecodedAudio{Samples: source, SampleRate: OutputSampleRate, Channels: OutputChannels})
	if err != nil {
		t.Fatal(err)
	}
	if len(clip.pcm) != 0 || !slices.Equal(clip.samples, source) {
		t.Fatal("Web clip was converted to PCM bytes")
	}
	if clip.length != int64(len(source)*BytesPerSample) {
		t.Fatalf("Web clip length = %d, want %d", clip.length, len(source)*BytesPerSample)
	}
	source[0] = 0
	if clip.samples[0] != 0.25 {
		t.Fatal("Web clip retained caller-owned sample buffer")
	}
}

func TestWebPlayerDefersAudioContextUntilUserGesture(t *testing.T) {
	js.Global().Call("eval", `
		globalThis.__onixAudioTest = {
			constructors: 0,
			resumes: 0,
			starts: 0,
			mediaPlays: 0,
			channels: [],
			listeners: {},
			dispatch(type) {
				const listener = this.listeners[type];
				if (listener) listener({type});
			}
		};
		Object.defineProperty(globalThis, "navigator", {
			configurable: true,
			value: {userActivation: {hasBeenActive: true, isActive: false}}
		});
		globalThis.document = {
			addEventListener(type, listener) {
				globalThis.__onixAudioTest.listeners[type] = listener;
			},
			removeEventListener(type, listener) {
				if (globalThis.__onixAudioTest.listeners[type] === listener) {
					delete globalThis.__onixAudioTest.listeners[type];
				}
			}
		};
		globalThis.AudioContext = class {
			constructor(options) {
				globalThis.__onixAudioTest.constructors++;
				this.options = options;
				this.currentTime = 0;
				this.destination = {};
			}
			resume() {
				globalThis.__onixAudioTest.resumes++;
				return Promise.resolve();
			}
			createBuffer() {
				return {copyToChannel(values, channel) {
					globalThis.__onixAudioTest.channels[channel] = Array.from(values);
				}};
			}
			createGain() {
				const gain = {gain: {value: 1}, connect() {}, disconnect() {}};
				globalThis.__onixAudioTest.gain = gain;
				return gain;
			}
			createBufferSource() {
				return {
					connect() {},
					disconnect() {},
					stop() {},
					start() { globalThis.__onixAudioTest.starts++; }
				};
			}
			createMediaElementSource() {
				return {connect() {}, disconnect() {}};
			}
		};
		globalThis.Audio = class {
			constructor() {
				this.currentTime = 0;
			}
			addEventListener() {}
			removeEventListener() {}
			pause() {}
			play() {
				globalThis.__onixAudioTest.mediaPlays++;
				return Promise.resolve();
			}
		};
	`)
	t.Cleanup(func() {
		webDevice.mu.Lock()
		unlock := webDevice.unlock
		installed := webDevice.unlockInstalled
		webDevice.ctx = js.Undefined()
		webDevice.err = nil
		clear(webDevice.waiting)
		webDevice.unlock = js.Func{}
		webDevice.unlockInstalled = false
		webDevice.mu.Unlock()
		if installed {
			unlock.Release()
		}
	})

	clip, err := NewClip(FormatWAV, DecodedAudio{Samples: []float32{0.25, -0.5, 0.75, -1}, SampleRate: OutputSampleRate, Channels: OutputChannels})
	if err != nil {
		t.Fatalf("decode web clip: %v", err)
	}
	playback, err := clip.NewPlayer()
	if err != nil {
		t.Fatalf("create web player: %v", err)
	}
	player := playback.platform.(*webPlayer)
	if err := player.SetVolume(.25); err != nil {
		t.Fatalf("set deferred volume: %v", err)
	}
	if err := player.Play(); err != nil {
		t.Fatalf("defer playback: %v", err)
	}
	state := js.Global().Get("__onixAudioTest")
	if got := state.Get("constructors").Int(); got != 0 {
		t.Fatalf("AudioContext constructors before gesture = %d, want 0", got)
	}
	if !player.IsPlaying() {
		t.Fatal("deferred player did not retain its playback request")
	}
	streamClip, err := NewStreamingClip(FormatMP3, NewMemorySource([]byte{1}), outputSampleRate, outputSampleRate,
		func(source Source) (io.ReadSeekCloser, error) { return source.Open() })
	if err != nil {
		t.Fatalf("create streaming clip: %v", err)
	}
	streamPlatform, err := newStreamingWebPlayer(streamClip)
	if err != nil {
		t.Fatalf("create streaming web player: %v", err)
	}
	streamPlayer := streamPlatform.(*webStreamPlayer)
	if err := streamPlayer.Play(); err != nil {
		t.Fatalf("defer streaming playback: %v", err)
	}

	state.Call("dispatch", "pointerdown")
	if got := state.Get("constructors").Int(); got != 0 {
		t.Fatalf("AudioContext constructors after pointerdown = %d, want 0", got)
	}
	if got := state.Get("mediaPlays").Int(); got != 0 {
		t.Fatalf("stream plays after pointerdown = %d, want 0", got)
	}

	js.Global().Get("navigator").Get("userActivation").Set("isActive", true)
	state.Call("dispatch", "click")
	deadline := time.Now().Add(time.Second)
	for state.Get("starts").Int() == 0 && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if got := state.Get("constructors").Int(); got != 1 {
		t.Fatalf("AudioContext constructors after gesture = %d, want 1", got)
	}
	if got := state.Get("resumes").Int(); got != 1 {
		t.Fatalf("AudioContext resumes after gesture = %d, want 1", got)
	}
	if got := state.Get("starts").Int(); got != 1 {
		t.Fatalf("audio source starts after gesture = %d, want 1", got)
	}
	if channels := state.Get("channels"); channels.Index(0).Index(0).Float() != 0.25 || channels.Index(0).Index(1).Float() != 0.75 || channels.Index(1).Index(0).Float() != -0.5 || channels.Index(1).Index(1).Float() != -1 {
		t.Fatalf("WebAudio channels were not deinterleaved from float samples")
	}
	if got := state.Get("mediaPlays").Int(); got != 1 {
		t.Fatalf("stream plays after gesture = %d, want 1", got)
	}
	if got := player.gain.Get("gain").Get("value").Float(); got != .25 {
		t.Fatalf("deferred gain = %v, want .25", got)
	}
	if state.Get("listeners").Get("click").Truthy() {
		t.Fatal("click unlock listener remained installed")
	}
	if err := player.Close(); err != nil {
		t.Fatalf("close web player: %v", err)
	}
	if err := streamPlayer.Close(); err != nil {
		t.Fatalf("close streaming web player: %v", err)
	}
}
