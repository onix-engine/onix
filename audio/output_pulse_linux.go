//go:build linux && !android

package audio

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

type pulseSampleSpec struct {
	format   int32
	rate     uint32
	channels uint8
}

type pulseBufferAttr struct {
	maxLength, targetLength, prebuffer, minRequest, fragmentSize uint32
}

var pulse struct {
	open     func(uintptr, string, int32, uintptr, string, *pulseSampleSpec, uintptr, *pulseBufferAttr, *int32) uintptr
	drain    func(uintptr, *int32) int32
	free     func(uintptr)
	strerror func(int32) string
	write    uintptr
}

var loadPulse = sync.OnceValue(func() error {
	lib, err := purego.Dlopen("libpulse-simple.so.0", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return err
	}
	purego.RegisterLibFunc(&pulse.open, lib, "pa_simple_new")
	purego.RegisterLibFunc(&pulse.drain, lib, "pa_simple_drain")
	purego.RegisterLibFunc(&pulse.free, lib, "pa_simple_free")
	purego.RegisterLibFunc(&pulse.strerror, lib, "pa_strerror")
	pulse.write, err = purego.Dlsym(lib, "pa_simple_write")
	return err
})

type pulseOutput struct {
	handle uintptr
	code   int32
}

func openPulse() (*pulseOutput, error) {
	if err := loadPulse(); err != nil {
		return nil, fmt.Errorf("audio: load PulseAudio: %w", err)
	}
	const playback, float32LE, float32BE = 1, 5, 6
	spec := pulseSampleSpec{format: float32LE, rate: outputSampleRate, channels: outputChannels}
	if binary.NativeEndian.Uint16([]byte{0, 1}) == 1 {
		spec.format = float32BE
	}
	const auto = math.MaxUint32
	attr := pulseBufferAttr{auto, 3 * outputBlockFrames * bytesPerFrame, auto, auto, auto}
	o := &pulseOutput{}
	o.handle = pulse.open(0, "Onix", playback, 0, "Playback", &spec, 0, &attr, &o.code)
	if o.handle == 0 {
		return nil, o.err("open")
	}
	return o, nil
}

func (o *pulseOutput) write(samples []float32) error {
	result, _, _ := purego.SyscallN(pulse.write, o.handle, uintptr(unsafe.Pointer(&samples[0])), uintptr(len(samples)*bytesPerSample), uintptr(unsafe.Pointer(&o.code)))
	if int32(result) < 0 {
		return o.err("write")
	}
	return nil
}

func (o *pulseOutput) idle() error {
	if pulse.drain(o.handle, &o.code) < 0 {
		return o.err("drain")
	}
	return nil
}

func (o *pulseOutput) close(drain bool) error {
	var err error
	if drain {
		err = o.idle()
	}
	pulse.free(o.handle)
	return err
}

func (o *pulseOutput) err(op string) error {
	return fmt.Errorf("audio: PulseAudio %s: %s", op, pulse.strerror(o.code))
}
