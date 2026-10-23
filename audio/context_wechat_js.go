//go:build js && wechat

package audio

import (
	"errors"
	"syscall/js"
)

func createWebAudioContext() (js.Value, error) {
	if err := validateWebAudioContext(); err != nil {
		return js.Undefined(), err
	}
	context := js.Global().Get("wx").Call("createWebAudioContext")
	if !context.Truthy() {
		return js.Undefined(), errors.New("audio: WeChat WebAudio is unavailable")
	}
	return context, nil
}

func validateWebAudioContext() error {
	wx := js.Global().Get("wx")
	if !wx.Truthy() {
		return errors.New("audio: WeChat API is unavailable")
	}
	if !wx.Get("createWebAudioContext").Truthy() {
		return errors.New("audio: WeChat WebAudio is unavailable")
	}
	return nil
}

func shouldDeferWebAudioContext() bool { return false }

func installWebAudioUnlock(js.Func) {}

func uninstallWebAudioUnlock(js.Func) {}
