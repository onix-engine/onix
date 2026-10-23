//go:build android || ios || onix_mobile_test

package camera

import (
	"errors"
	"image"
	"strings"
	"sync"
)

type NativeCommand uint8

const (
	NativeCommandOpen NativeCommand = iota + 1
	NativeCommandClose
)

type NativeEvent uint8

const (
	NativeEventOpened NativeEvent = iota + 1
	NativeEventClosed
	NativeEventError
)

type mobileCamera struct {
	closed bool
}

var mobileState struct {
	sync.Mutex
	sequence uint64
	command  NativeCommand
	options  Options
	opening  bool
	opened   bool
	result   chan error
	frame    *image.NRGBA
	err      error
}

func platformAvailable() bool {
	return true
}

func open(options Options) (frameSource, error) {
	if options.Width <= 0 {
		options.Width = 960
	}
	if options.Height <= 0 {
		options.Height = 720
	}
	if strings.TrimSpace(options.FacingMode) == "" {
		options.FacingMode = "environment"
	}
	mobileState.Lock()
	if mobileState.opening || mobileState.opened {
		mobileState.Unlock()
		return nil, errors.New("camera: camera capture is already active")
	}
	result := make(chan error, 1)
	mobileState.sequence++
	mobileState.command = NativeCommandOpen
	mobileState.options = options
	mobileState.opening = true
	mobileState.result = result
	mobileState.frame = nil
	mobileState.err = nil
	mobileState.Unlock()
	if err := <-result; err != nil {
		return nil, err
	}
	return &mobileCamera{}, nil
}

func (c *mobileCamera) Frame() (*image.NRGBA, error) {
	if c == nil || c.closed {
		return nil, ErrUnavailable
	}
	mobileState.Lock()
	defer mobileState.Unlock()
	if mobileState.frame == nil {
		if mobileState.err != nil {
			return nil, mobileState.err
		}
		return nil, ErrNotReady
	}
	return mobileState.frame, nil
}

func (c *mobileCamera) Close() error {
	if c == nil || c.closed {
		return nil
	}
	c.closed = true
	mobileState.Lock()
	defer mobileState.Unlock()
	mobileState.sequence++
	mobileState.command = NativeCommandClose
	mobileState.opened = false
	mobileState.frame = nil
	mobileState.err = nil
	return nil
}

func NativeCommandValue() uint64 {
	mobileState.Lock()
	defer mobileState.Unlock()
	return mobileState.sequence<<8 | uint64(mobileState.command)
}

func NativeOptions() Options {
	mobileState.Lock()
	defer mobileState.Unlock()
	return mobileState.options
}

func NativeFacingMode() int {
	if strings.EqualFold(NativeOptions().FacingMode, "user") {
		return 2
	}
	return 1
}

func DispatchNativeEvent(event NativeEvent) {
	mobileState.Lock()
	defer mobileState.Unlock()
	switch event {
	case NativeEventOpened:
		mobileState.opening = false
		mobileState.opened = true
		mobileState.err = nil
		sendCameraResult(nil)
	case NativeEventClosed:
		mobileState.opening = false
		mobileState.opened = false
		mobileState.frame = nil
		mobileState.err = nil
	case NativeEventError:
		mobileState.opening = false
		mobileState.opened = false
		mobileState.frame = nil
		mobileState.err = errors.New("camera: native capture failed")
		sendCameraResult(mobileState.err)
	}
}

func DispatchNativeFrame(pixels []byte, width, height int) {
	if width <= 0 || height <= 0 || width > maxInt()/4 || height > maxInt()/(width*4) || len(pixels) != width*height*4 {
		return
	}
	frame := image.NewNRGBA(image.Rect(0, 0, width, height))
	copy(frame.Pix, pixels)
	mobileState.Lock()
	if mobileState.opened {
		mobileState.frame = frame
	}
	mobileState.Unlock()
}

func sendCameraResult(err error) {
	if mobileState.result == nil {
		return
	}
	mobileState.result <- err
	mobileState.result = nil
}

func maxInt() int {
	return int(^uint(0) >> 1)
}
