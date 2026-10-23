//go:build !wechat

package speech

import (
	"errors"
	"strings"
	"sync"
	"syscall/js"
)

type webSynthesizer struct {
	mu        sync.Mutex
	engine    js.Value
	utterance js.Value
	end       js.Func
	failure   js.Func
	status    Status
}

func newPlatformSynthesizer() platformSynthesizer {
	return &webSynthesizer{engine: js.Global().Get("speechSynthesis")}
}

func (s *webSynthesizer) Available() bool {
	return s != nil && s.engine.Truthy() && js.Global().Get("SpeechSynthesisUtterance").Type() == js.TypeFunction
}

func (s *webSynthesizer) Speak(text string, options Options) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("speech: text is empty")
	}
	options = normalizedOptions(options)
	if err := validateOptions(options); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
	utterance := js.Global().Get("SpeechSynthesisUtterance").New(text)
	utterance.Set("lang", options.Language)
	utterance.Set("rate", options.Rate)
	utterance.Set("pitch", options.Pitch)
	utterance.Set("volume", options.Volume)
	if voice := s.voice(options.Voice, options.Language); voice.Truthy() {
		utterance.Set("voice", voice)
	}
	s.end = js.FuncOf(func(js.Value, []js.Value) any {
		s.finish(nil)
		return nil
	})
	s.failure = js.FuncOf(func(_ js.Value, args []js.Value) any {
		message := "system speech failed"
		if len(args) > 0 && args[0].Get("error").Truthy() {
			message = args[0].Get("error").String()
		}
		s.finish(errors.New("speech: " + message))
		return nil
	})
	utterance.Set("onend", s.end)
	utterance.Set("onerror", s.failure)
	s.utterance = utterance
	s.status = Status{Speaking: true}
	s.engine.Call("speak", utterance)
	return nil
}

func (s *webSynthesizer) Pause() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.status.Speaking {
		return errors.New("speech: system speech is not active")
	}
	if !s.status.Paused {
		s.engine.Call("pause")
		s.status.Paused = true
	}
	return nil
}

func (s *webSynthesizer) Resume() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.status.Speaking {
		return errors.New("speech: system speech is not active")
	}
	if s.status.Paused {
		s.engine.Call("resume")
		s.status.Paused = false
	}
	return nil
}

func (s *webSynthesizer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
	return nil
}

func (s *webSynthesizer) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *webSynthesizer) Close() error {
	return s.Stop()
}

func (s *webSynthesizer) finish(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = Status{Err: err}
	if s.utterance.Truthy() {
		s.utterance.Set("onend", js.Null())
		s.utterance.Set("onerror", js.Null())
		s.utterance = js.Undefined()
	}
}

func (s *webSynthesizer) stopLocked() {
	active := s.status.Speaking || s.engine.Get("speaking").Bool() || s.engine.Get("pending").Bool()
	s.status = Status{}
	s.detachLocked()
	if active {
		s.engine.Call("cancel")
	}
}

func (s *webSynthesizer) detachLocked() {
	if s.utterance.Truthy() {
		s.utterance.Set("onend", js.Null())
		s.utterance.Set("onerror", js.Null())
	}
	if s.end.Value.Type() == js.TypeFunction {
		s.end.Release()
		s.end = js.Func{}
	}
	if s.failure.Value.Type() == js.TypeFunction {
		s.failure.Release()
		s.failure = js.Func{}
	}
	s.utterance = js.Undefined()
}

func (s *webSynthesizer) voice(name, language string) js.Value {
	voices := s.engine.Call("getVoices")
	var languageMatch js.Value
	for index := 0; index < voices.Length(); index++ {
		voice := voices.Index(index)
		if name != "" && (strings.EqualFold(voice.Get("name").String(), name) || strings.EqualFold(voice.Get("voiceURI").String(), name)) {
			return voice
		}
		if !languageMatch.Truthy() && language != "" && strings.EqualFold(voice.Get("lang").String(), language) {
			languageMatch = voice
		}
	}
	return languageMatch
}
