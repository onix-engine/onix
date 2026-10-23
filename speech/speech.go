package speech

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

var ErrUnavailable = errors.New("speech: system text-to-speech is unavailable")

type Options struct {
	Language string
	Voice    string
	Rate     float64
	Pitch    float64
	Volume   float64
}

type Status struct {
	Speaking bool
	Paused   bool
	Err      error
}

type platformSynthesizer interface {
	Available() bool
	Speak(string, Options) error
	Pause() error
	Resume() error
	Stop() error
	Status() Status
	Close() error
}

type Synthesizer struct {
	platform platformSynthesizer
}

func New() *Synthesizer {
	return &Synthesizer{platform: newPlatformSynthesizer()}
}

func (s *Synthesizer) Available() bool {
	return s != nil && s.platform != nil && s.platform.Available()
}

func (s *Synthesizer) Speak(text string, options Options) error {
	if !s.Available() {
		return ErrUnavailable
	}
	return s.platform.Speak(text, options)
}

func (s *Synthesizer) Pause() error {
	if !s.Available() {
		return ErrUnavailable
	}
	return s.platform.Pause()
}

func (s *Synthesizer) Resume() error {
	if !s.Available() {
		return ErrUnavailable
	}
	return s.platform.Resume()
}

func (s *Synthesizer) Stop() error {
	if !s.Available() {
		return ErrUnavailable
	}
	return s.platform.Stop()
}

func (s *Synthesizer) Status() Status {
	if !s.Available() {
		return Status{Err: ErrUnavailable}
	}
	return s.platform.Status()
}

func (s *Synthesizer) Close() error {
	if s == nil || s.platform == nil {
		return nil
	}
	err := s.platform.Close()
	s.platform = nil
	return err
}

func normalizedOptions(options Options) Options {
	options.Language = strings.TrimSpace(options.Language)
	options.Voice = strings.TrimSpace(options.Voice)
	if options.Rate == 0 {
		options.Rate = 1
	}
	if options.Pitch == 0 {
		options.Pitch = 1
	}
	if options.Volume == 0 {
		options.Volume = 1
	}
	return options
}

func validateOptions(options Options) error {
	for name, value := range map[string]float64{"rate": options.Rate, "pitch": options.Pitch, "volume": options.Volume} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("speech: %s must be finite", name)
		}
	}
	if options.Rate < 0.1 || options.Rate > 10 {
		return errors.New("speech: rate must be between 0.1 and 10")
	}
	if options.Pitch < 0 || options.Pitch > 2 {
		return errors.New("speech: pitch must be between 0 and 2")
	}
	if options.Volume < 0 || options.Volume > 1 {
		return errors.New("speech: volume must be between 0 and 1")
	}
	return nil
}
