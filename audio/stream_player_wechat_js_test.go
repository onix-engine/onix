//go:build js && wasm && wechat

package audio

import (
	"errors"
	"io"
	"strings"
	"syscall/js"
	"testing"
	"time"
)

func TestWechatStreamingPlayerUsesCompressedFileCache(t *testing.T) {
	js.Global().Call("eval", `
		globalThis.__onixWechatAudioTest = {
			files: new Map(),
			writes: 0,
			plays: 0,
			pauses: 0,
			seeks: [],
			destroys: 0,
			last: null
		};
		globalThis.wx = {
			env: {USER_DATA_PATH: "/user"},
			createWebAudioContext() { return {}; },
			getFileSystemManager() {
				return {
					mkdirSync(name) {
						globalThis.__onixWechatAudioTest.files.set(name, null);
					},
					statSync(name) {
						if (!globalThis.__onixWechatAudioTest.files.has(name)) throw new Error("ENOENT");
						const data = globalThis.__onixWechatAudioTest.files.get(name);
						return {size: data === null ? 0 : data.byteLength};
					},
					writeFileSync(name, data) {
						globalThis.__onixWechatAudioTest.writes++;
						globalThis.__onixWechatAudioTest.files.set(name, new Uint8Array(data));
					}
				};
			},
			createInnerAudioContext() {
				const media = {
					currentTime: 0,
					volume: 1,
					onEnded(callback) { this.ended = callback; },
					offEnded(callback) { if (this.ended === callback) this.ended = null; },
					onError(callback) { this.failed = callback; },
					offError(callback) { if (this.failed === callback) this.failed = null; },
					play() { globalThis.__onixWechatAudioTest.plays++; },
					pause() { globalThis.__onixWechatAudioTest.pauses++; },
					seek(value) {
						this.currentTime = value;
						globalThis.__onixWechatAudioTest.seeks.push(value);
					},
					destroy() { globalThis.__onixWechatAudioTest.destroys++; }
				};
				globalThis.__onixWechatAudioTest.last = media;
				return media;
			}
		};
	`)
	resetWechatAudioTestState(t)

	clip, err := NewStreamingClip(FormatMP3, NewMemorySource([]byte{1, 2, 3, 4}), outputSampleRate, outputSampleRate,
		func(Source) (io.ReadSeekCloser, error) { return nil, errors.New("stream reader must not be opened") },
	)
	if err != nil {
		t.Fatal(err)
	}
	first, err := clip.NewPlayer()
	if err != nil {
		t.Fatalf("create WeChat stream player: %v", err)
	}
	state := js.Global().Get("__onixWechatAudioTest")
	if got := state.Get("writes").Int(); got != 1 {
		t.Fatalf("cache writes = %d, want 1", got)
	}
	if !strings.HasSuffix(clip.streamSource, ".mp3") {
		t.Fatalf("stream cache path = %q, want .mp3 suffix", clip.streamSource)
	}

	second, err := clip.NewPlayer()
	if err != nil {
		t.Fatalf("create cached WeChat stream player: %v", err)
	}
	if got := state.Get("writes").Int(); got != 1 {
		t.Fatalf("cache writes after reuse = %d, want 1", got)
	}
	cachedClip, err := NewStreamingClip(FormatMP3, NewMemorySource([]byte{1, 2, 3, 4}), outputSampleRate, outputSampleRate,
		func(Source) (io.ReadSeekCloser, error) { return nil, errors.New("stream reader must not be opened") },
	)
	if err != nil {
		t.Fatal(err)
	}
	cached, err := cachedClip.NewPlayer()
	if err != nil {
		t.Fatalf("create persisted-cache WeChat stream player: %v", err)
	}
	if got := state.Get("writes").Int(); got != 1 {
		t.Fatalf("cache writes for identical clip = %d, want 1", got)
	}
	if err := first.SetVolume(.25); err != nil {
		t.Fatal(err)
	}
	if got := first.Volume(); got != .25 {
		t.Fatalf("volume = %v, want .25", got)
	}
	if err := first.Play(); err != nil {
		t.Fatal(err)
	}
	if got := state.Get("plays").Int(); got != 1 {
		t.Fatalf("plays = %d, want 1", got)
	}
	if err := first.Seek(250 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if got := first.Position(); got != 250*time.Millisecond {
		t.Fatalf("position = %v, want 250ms", got)
	}

	suspendStreamingPlatform()
	if got := state.Get("pauses").Int(); got != 1 {
		t.Fatalf("pauses after suspend = %d, want 1", got)
	}
	if !first.IsPlaying() {
		t.Fatal("suspended stream lost its playing state")
	}
	resumeStreamingPlatform()
	if got := state.Get("plays").Int(); got != 2 {
		t.Fatalf("plays after resume = %d, want 2", got)
	}

	media := state.Get("last")
	if err := cached.Play(); err != nil {
		t.Fatal(err)
	}
	media.Get("ended").Invoke()
	if cached.IsPlaying() {
		t.Fatal("ended stream remained playing")
	}
	media.Get("failed").Invoke(map[string]any{"errMsg": "decode failed"})
	if err := cached.Err(); err == nil || !strings.Contains(err.Error(), "decode failed") {
		t.Fatalf("stream error = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cached.Close(); err != nil {
		t.Fatal(err)
	}
	if got := state.Get("destroys").Int(); got != 3 {
		t.Fatalf("destroy calls = %d, want 3", got)
	}
}

func resetWechatAudioTestState(t *testing.T) {
	t.Helper()
	wechatStreamFiles.Lock()
	wechatStreamFiles.manager = js.Undefined()
	wechatStreamFiles.directory = ""
	wechatStreamFiles.Unlock()
	wechatStreamPlayers.Lock()
	clear(wechatStreamPlayers.values)
	wechatStreamPlayers.Unlock()
	wechatStreamPlayers.suspended.Store(false)
	t.Cleanup(func() {
		wechatStreamFiles.Lock()
		wechatStreamFiles.manager = js.Undefined()
		wechatStreamFiles.directory = ""
		wechatStreamFiles.Unlock()
		wechatStreamPlayers.Lock()
		clear(wechatStreamPlayers.values)
		wechatStreamPlayers.Unlock()
		wechatStreamPlayers.suspended.Store(false)
		js.Global().Delete("wx")
		js.Global().Delete("__onixWechatAudioTest")
	})
}
