//go:build android || ios || onix_mobile_test

package speech

import (
	"errors"
	"strings"
	"sync"
)

type NativeCommand uint8

const (
	NativeCommandSpeak NativeCommand = iota + 1
	NativeCommandPause
	NativeCommandResume
	NativeCommandStop
)

type NativeEvent uint8

const (
	NativeEventAvailable NativeEvent = iota + 1
	NativeEventStarted
	NativeEventCompleted
	NativeEventPaused
	NativeEventResumed
	NativeEventStopped
	NativeEventError
)

type mobileSynthesizer struct{}

var mobileState struct {
	sync.Mutex
	sequence  uint64
	command   NativeCommand
	text      string
	options   Options
	available bool
	status    Status
}

func newPlatformSynthesizer() platformSynthesizer {
	return mobileSynthesizer{}
}

func (mobileSynthesizer) Available() bool {
	mobileState.Lock()
	defer mobileState.Unlock()
	return mobileState.available
}

func (mobileSynthesizer) Speak(text string, options Options) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("speech: text is empty")
	}
	options = normalizedOptions(options)
	if err := validateOptions(options); err != nil {
		return err
	}
	mobileState.Lock()
	defer mobileState.Unlock()
	if !mobileState.available {
		return ErrUnavailable
	}
	mobileState.sequence++
	mobileState.command = NativeCommandSpeak
	mobileState.text = text
	mobileState.options = options
	mobileState.status = Status{Speaking: true}
	return nil
}

func (mobileSynthesizer) Pause() error {
	return mobileSpeechCommand(NativeCommandPause, func(status Status) bool { return status.Speaking && !status.Paused })
}

func (mobileSynthesizer) Resume() error {
	return mobileSpeechCommand(NativeCommandResume, func(status Status) bool { return status.Speaking && status.Paused })
}

func (mobileSynthesizer) Stop() error {
	mobileState.Lock()
	defer mobileState.Unlock()
	if !mobileState.status.Speaking {
		return nil
	}
	mobileState.sequence++
	mobileState.command = NativeCommandStop
	mobileState.status = Status{}
	return nil
}

func (mobileSynthesizer) Status() Status {
	mobileState.Lock()
	defer mobileState.Unlock()
	if !mobileState.available {
		return Status{Err: ErrUnavailable}
	}
	return mobileState.status
}

func (s mobileSynthesizer) Close() error {
	return s.Stop()
}

func mobileSpeechCommand(command NativeCommand, allowed func(Status) bool) error {
	mobileState.Lock()
	defer mobileState.Unlock()
	if !mobileState.available {
		return ErrUnavailable
	}
	if !allowed(mobileState.status) {
		return errors.New("speech: system speech state does not allow this command")
	}
	mobileState.sequence++
	mobileState.command = command
	mobileState.status.Paused = command == NativeCommandPause
	return nil
}

func NativeCommandValue() uint64 {
	mobileState.Lock()
	defer mobileState.Unlock()
	return mobileState.sequence<<8 | uint64(mobileState.command)
}

func NativeText() string {
	mobileState.Lock()
	defer mobileState.Unlock()
	return mobileState.text
}

func NativeOptions() Options {
	mobileState.Lock()
	defer mobileState.Unlock()
	return mobileState.options
}

func DispatchNativeEvent(event NativeEvent) {
	mobileState.Lock()
	defer mobileState.Unlock()
	switch event {
	case NativeEventAvailable:
		mobileState.available = true
		mobileState.status.Err = nil
	case NativeEventStarted, NativeEventResumed:
		mobileState.status = Status{Speaking: true}
	case NativeEventPaused:
		mobileState.status = Status{Speaking: true, Paused: true}
	case NativeEventCompleted, NativeEventStopped:
		mobileState.status = Status{}
	case NativeEventError:
		mobileState.status = Status{Err: errors.New("speech: native synthesis failed")}
	}
}
