//go:build linux && !android

package audio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

var alsa struct {
	open      func(*uintptr, string, int32, int32) int32
	setParams func(uintptr, int32, int32, uint32, uint32, int32, uint32) int32
	recover   func(uintptr, int32, int32) int32
	drain     func(uintptr) int32
	prepare   func(uintptr) int32
	drop      func(uintptr) int32
	close     func(uintptr) int32
	strerror  func(int32) string
	writei    uintptr
}

var loadALSA = sync.OnceValue(func() error {
	lib, err := purego.Dlopen("libasound.so.2", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return err
	}
	purego.RegisterLibFunc(&alsa.open, lib, "snd_pcm_open")
	purego.RegisterLibFunc(&alsa.setParams, lib, "snd_pcm_set_params")
	purego.RegisterLibFunc(&alsa.recover, lib, "snd_pcm_recover")
	purego.RegisterLibFunc(&alsa.drain, lib, "snd_pcm_drain")
	purego.RegisterLibFunc(&alsa.prepare, lib, "snd_pcm_prepare")
	purego.RegisterLibFunc(&alsa.drop, lib, "snd_pcm_drop")
	purego.RegisterLibFunc(&alsa.close, lib, "snd_pcm_close")
	purego.RegisterLibFunc(&alsa.strerror, lib, "snd_strerror")
	alsa.writei, err = purego.Dlsym(lib, "snd_pcm_writei")
	return err
})

type alsaOutput uintptr

type linuxOutput interface {
	write([]float32) error
	idle() error
	close(bool) error
}

func openNativeOutput() (linuxOutput, error) {
	pcm, alsaErr := openALSA()
	if alsaErr == nil {
		return pcm, nil
	}
	stream, pulseErr := openPulse()
	if pulseErr != nil {
		return nil, errors.Join(alsaErr, pulseErr)
	}
	return stream, nil
}

func openALSA() (alsaOutput, error) {
	if err := loadALSA(); err != nil {
		return 0, fmt.Errorf("audio: load ALSA: %w", err)
	}
	const (
		playback      = 0
		floatLE       = 14
		floatBE       = 15
		rwInterleaved = 3
	)
	var pcm uintptr
	if code := alsa.open(&pcm, "default", playback, 0); code < 0 {
		return 0, alsaError("open", code)
	}
	format := int32(floatLE)
	if binary.NativeEndian.Uint16([]byte{0, 1}) == 1 {
		format = floatBE
	}
	if code := alsa.setParams(pcm, format, rwInterleaved, outputChannels, outputSampleRate, 1, 30000); code < 0 {
		alsa.close(pcm)
		return 0, alsaError("set params", code)
	}
	return alsaOutput(pcm), nil
}

func (o alsaOutput) write(samples []float32) error {
	for len(samples) > 0 {
		n, _, _ := purego.SyscallN(alsa.writei, uintptr(o), uintptr(unsafe.Pointer(&samples[0])), uintptr(len(samples)/outputChannels))
		if int(n) < 0 {
			if code := alsa.recover(uintptr(o), int32(n), 1); code < 0 {
				return alsaError("write", code)
			}
			continue
		}
		if n == 0 {
			return io.ErrNoProgress
		}
		samples = samples[int(n)*outputChannels:]
	}
	return nil
}

func (o alsaOutput) close(drain bool) error {
	pcm := uintptr(o)
	var code int32
	if drain {
		code = alsa.drain(pcm)
	} else {
		code = alsa.drop(pcm)
	}
	closed := alsa.close(pcm)
	if code < 0 {
		return alsaError("stop", code)
	}
	if closed < 0 {
		return alsaError("close", closed)
	}
	return nil
}

func (o alsaOutput) idle() error {
	if code := alsa.drain(uintptr(o)); code < 0 {
		return alsaError("drain", code)
	}
	if code := alsa.prepare(uintptr(o)); code < 0 {
		return alsaError("prepare", code)
	}
	return nil
}

func alsaError(op string, code int32) error {
	return fmt.Errorf("audio: ALSA %s: %s", op, alsa.strerror(code))
}
