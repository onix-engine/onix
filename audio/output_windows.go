package audio

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/ebitengine/purego"
	"golang.org/x/sys/windows"
)

var (
	createInstance        = windows.NewLazySystemDLL("ole32.dll").NewProc("CoCreateInstance")
	deviceEnumeratorClass = windows.GUID{Data1: 0xbcde0395, Data2: 0xe52f, Data3: 0x467c, Data4: [8]byte{0x8e, 0x3d, 0xc4, 0x57, 0x92, 0x91, 0x69, 0x2e}}
	deviceEnumeratorIID   = windows.GUID{Data1: 0xa95664d2, Data2: 0x9614, Data3: 0x4f35, Data4: [8]byte{0xa7, 0x46, 0xde, 0x8d, 0xb6, 0x36, 0x17, 0xe6}}
	audioClientIID        = windows.GUID{Data1: 0x1cb9ad4c, Data2: 0xdbfa, Data3: 0x4c32, Data4: [8]byte{0xb1, 0x78, 0xc2, 0xf5, 0x68, 0xa7, 0x03, 0xb2}}
	renderClientIID       = windows.GUID{Data1: 0xf294acfc, Data2: 0x3146, Data3: 0x4483, Data4: [8]byte{0xa7, 0xbf, 0xad, 0xdc, 0xa7, 0xc2, 0x60, 0xe2}}
)

// Vtable slots from IUnknown, IMMDeviceEnumerator, IMMDevice,
// IAudioClient and IAudioRenderClient in the Windows SDK.
const (
	comRelease              = 2
	getDefaultAudioEndpoint = 4
	activateEndpoint        = 3
	initializeAudioClient   = 3
	getBufferSize           = 4
	getCurrentPadding       = 6
	startAudioClient        = 10
	stopAudioClient         = 11
	setEventHandle          = 13
	getAudioService         = 14
	getRenderBuffer         = 3
	releaseRenderBuffer     = 4
)

type audioCOM struct{ vtable *[15]uintptr }

func (c *audioCOM) release() {
	if c != nil {
		syscall.SyscallN(c.vtable[comRelease], uintptr(unsafe.Pointer(c)))
	}
}

type waveFormat struct {
	tag, channels           uint16
	rate, bytesPerSec       uint32
	blockAlign, bits, extra uint16
}

type wasapiOutput struct {
	client, render    *audioCOM
	buffer            *float32
	event             windows.Handle
	capacity, padding uint32
	started           bool
}

func openNativeOutput() (*wasapiOutput, error) {
	// S_FALSE means this thread already belongs to the requested apartment.
	if err := windows.CoInitializeEx(0, windows.COINIT_MULTITHREADED); err != nil && err != syscall.Errno(1) {
		return nil, fmt.Errorf("audio: initialize COM: %w", err)
	}
	o := &wasapiOutput{}
	if err := o.open(); err != nil {
		o.close(false)
		return nil, err
	}
	return o, nil
}

func (o *wasapiOutput) open() error {
	const inProcServer = 1
	var enumerator, endpoint *audioCOM
	r, _, _ := createInstance.Call(uintptr(unsafe.Pointer(&deviceEnumeratorClass)), 0, inProcServer,
		uintptr(unsafe.Pointer(&deviceEnumeratorIID)), uintptr(unsafe.Pointer(&enumerator)))
	if int32(r) < 0 {
		return wasapiError("create device enumerator", r)
	}
	defer enumerator.release()
	// IMMDeviceEnumerator::GetDefaultAudioEndpoint(eRender, eConsole).
	r, _, _ = syscall.SyscallN(enumerator.vtable[getDefaultAudioEndpoint], uintptr(unsafe.Pointer(enumerator)), 0, 0, uintptr(unsafe.Pointer(&endpoint)))
	if int32(r) < 0 {
		return wasapiError("get default output", r)
	}
	defer endpoint.release()
	r, _, _ = syscall.SyscallN(endpoint.vtable[activateEndpoint], uintptr(unsafe.Pointer(endpoint)), uintptr(unsafe.Pointer(&audioClientIID)),
		inProcServer, 0, uintptr(unsafe.Pointer(&o.client)))
	if int32(r) < 0 {
		return wasapiError("activate audio client", r)
	}

	const (
		eventCallback     = 0x00040000
		autoConvertPCM    = 0x80000000
		srcDefaultQuality = 0x08000000
	)
	format := waveFormat{tag: 3, channels: outputChannels, rate: outputSampleRate, bytesPerSec: bytesPerSecond, blockAlign: bytesPerFrame, bits: 32}
	var initialize func(*audioCOM, uint32, uint32, int64, int64, *waveFormat, *windows.GUID) int32
	purego.RegisterFunc(&initialize, o.client.vtable[initializeAudioClient])
	if hr := initialize(o.client, 0, eventCallback|autoConvertPCM|srcDefaultQuality, 0, 0, &format, nil); hr < 0 {
		return wasapiError("initialize shared output", uintptr(hr))
	}
	client := uintptr(unsafe.Pointer(o.client))
	r, _, _ = syscall.SyscallN(o.client.vtable[getBufferSize], client, uintptr(unsafe.Pointer(&o.capacity)))
	if int32(r) < 0 {
		return wasapiError("get buffer size", r)
	}
	event, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return err
	}
	o.event = event
	r, _, _ = syscall.SyscallN(o.client.vtable[setEventHandle], client, uintptr(event))
	if int32(r) < 0 {
		return wasapiError("set event handle", r)
	}
	r, _, _ = syscall.SyscallN(o.client.vtable[getAudioService], client, uintptr(unsafe.Pointer(&renderClientIID)), uintptr(unsafe.Pointer(&o.render)))
	if int32(r) < 0 {
		return wasapiError("get render client", r)
	}
	return nil
}

func (o *wasapiOutput) updatePadding() error {
	r, _, _ := syscall.SyscallN(o.client.vtable[getCurrentPadding], uintptr(unsafe.Pointer(o.client)), uintptr(unsafe.Pointer(&o.padding)))
	if int32(r) < 0 {
		return wasapiError("get current padding", r)
	}
	return nil
}

func (o *wasapiOutput) wait() error {
	status, err := windows.WaitForSingleObject(o.event, 1000)
	if err == nil && status == uint32(windows.WAIT_TIMEOUT) {
		return fmt.Errorf("audio: WASAPI buffer timed out")
	}
	return err
}

func (o *wasapiOutput) write(samples []float32) error {
	for len(samples) != 0 {
		if err := o.updatePadding(); err != nil {
			return err
		}
		frames := min(int(o.capacity-o.padding), len(samples)/outputChannels)
		if frames == 0 {
			if err := o.wait(); err != nil {
				return err
			}
			continue
		}
		render := uintptr(unsafe.Pointer(o.render))
		r, _, _ := syscall.SyscallN(o.render.vtable[getRenderBuffer], render, uintptr(frames), uintptr(unsafe.Pointer(&o.buffer)))
		if int32(r) < 0 {
			return wasapiError("get render buffer", r)
		}
		n := frames * outputChannels
		copy(unsafe.Slice(o.buffer, n), samples[:n])
		r, _, _ = syscall.SyscallN(o.render.vtable[releaseRenderBuffer], render, uintptr(frames), 0)
		if int32(r) < 0 {
			return wasapiError("release render buffer", r)
		}
		if !o.started {
			r, _, _ = syscall.SyscallN(o.client.vtable[startAudioClient], uintptr(unsafe.Pointer(o.client)))
			if int32(r) < 0 {
				return wasapiError("start", r)
			}
			o.started = true
		}
		samples = samples[n:]
	}
	return nil
}

func (o *wasapiOutput) stop(drain bool) error {
	var err error
	if o.started {
		for drain && err == nil {
			err = o.updatePadding()
			if err != nil || o.padding == 0 {
				break
			}
			err = o.wait()
		}
		r, _, _ := syscall.SyscallN(o.client.vtable[stopAudioClient], uintptr(unsafe.Pointer(o.client)))
		if err == nil && int32(r) < 0 {
			err = wasapiError("stop", r)
		}
		o.started = false
	}
	return err
}

func (o *wasapiOutput) idle() error { return o.stop(true) }

func (o *wasapiOutput) close(drain bool) error {
	err := o.stop(drain)
	o.render.release()
	o.client.release()
	if o.event != 0 {
		windows.CloseHandle(o.event)
	}
	windows.CoUninitialize()
	return err
}

func wasapiError(op string, code uintptr) error {
	return fmt.Errorf("audio: WASAPI %s: HRESULT %#08x", op, uint32(code))
}
