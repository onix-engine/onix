//go:build js && !wechat

package audio

import (
	"errors"
	"syscall/js"
)

func createWebAudioContext() (js.Value, error) {
	class, err := webAudioContextClass()
	if err != nil {
		return js.Undefined(), err
	}
	options := js.Global().Get("Object").New()
	options.Set("sampleRate", outputSampleRate)
	return class.New(options), nil
}

func validateWebAudioContext() error {
	_, err := webAudioContextClass()
	return err
}

func webAudioContextClass() (js.Value, error) {
	class := js.Global().Get("AudioContext")
	if !class.Truthy() {
		class = js.Global().Get("webkitAudioContext")
	}
	if !class.Truthy() {
		return js.Undefined(), errors.New("audio: WebAudio is unavailable")
	}
	return class, nil
}

func shouldDeferWebAudioContext() bool {
	return !js.Global().Get("navigator").Get("userActivation").Get("isActive").Bool()
}

func installWebAudioUnlock(callback js.Func) {
	document := js.Global().Get("document")
	document.Call("addEventListener", "click", callback)
	document.Call("addEventListener", "keydown", callback)
}

func uninstallWebAudioUnlock(callback js.Func) {
	document := js.Global().Get("document")
	document.Call("removeEventListener", "click", callback)
	document.Call("removeEventListener", "keydown", callback)
}
