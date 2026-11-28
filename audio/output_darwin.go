package audio

import (
	"errors"
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Prefix of AudioQueueBuffer. The first word contains mAudioDataBytesCapacity
// and its ABI padding. Audio Queue owns the memory at data.
type audioQueueBuffer struct {
	_    uintptr
	data uintptr
	size uint32
}

type audioStreamDescription struct {
	rate            float64
	format          uint32
	flags           uint32
	bytesPerPacket  uint32
	framesPerPacket uint32
	bytesPerFrame   uint32
	channels        uint32
	bits            uint32
	reserved        uint32
}

var audioQueue struct {
	newOutput func(*audioStreamDescription, uintptr, uintptr, uintptr, uintptr, uint32, *uintptr) int32
	allocate  func(uintptr, uint32, **audioQueueBuffer) int32
	start     func(uintptr, uintptr) int32
	pause     func(uintptr) int32
	dispose   func(uintptr, bool) int32
	enqueue   uintptr
	memcpy    uintptr
	callback  uintptr
}

// The native mixer owns one queue at a time. AudioQueueDispose guarantees
// that the queue will issue no further callbacks after it returns.
var freeQueueBuffers = make(chan *audioQueueBuffer, 2)

var loadAudioQueue = sync.OnceValue(func() error {
	lib, err := purego.Dlopen("/System/Library/Frameworks/AudioToolbox.framework/AudioToolbox", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return err
	}
	purego.RegisterLibFunc(&audioQueue.newOutput, lib, "AudioQueueNewOutput")
	purego.RegisterLibFunc(&audioQueue.allocate, lib, "AudioQueueAllocateBuffer")
	purego.RegisterLibFunc(&audioQueue.start, lib, "AudioQueueStart")
	purego.RegisterLibFunc(&audioQueue.pause, lib, "AudioQueuePause")
	purego.RegisterLibFunc(&audioQueue.dispose, lib, "AudioQueueDispose")
	if audioQueue.enqueue, err = purego.Dlsym(lib, "AudioQueueEnqueueBuffer"); err != nil {
		return err
	}
	if audioQueue.memcpy, err = purego.Dlsym(purego.RTLD_DEFAULT, "memcpy"); err != nil {
		return err
	}
	audioQueue.callback = purego.NewCallback(func(_, _ uintptr, buffer *audioQueueBuffer) {
		freeQueueBuffers <- buffer
	})
	return nil
})

type appleOutput struct {
	timeout *time.Timer
	queue   uintptr
	started bool
	paused  bool
}

func openNativeOutput() (*appleOutput, error) {
	if err := loadAudioQueue(); err != nil {
		return nil, fmt.Errorf("audio: load AudioToolbox: %w", err)
	}
	format := audioStreamDescription{
		rate:            outputSampleRate,
		format:          0x6c70636d, // kAudioFormatLinearPCM
		flags:           1 | 1<<3,   // kAudioFormatFlagIsFloat | kAudioFormatFlagIsPacked
		bytesPerPacket:  bytesPerFrame,
		framesPerPacket: 1,
		bytesPerFrame:   bytesPerFrame,
		channels:        outputChannels,
		bits:            32,
	}
	out := &appleOutput{}
	if code := audioQueue.newOutput(&format, audioQueue.callback, 0, 0, 0, 0, &out.queue); code != 0 {
		return nil, queueError("open", code)
	}
	out.timeout = time.NewTimer(time.Second)
	out.timeout.Stop()
	for range cap(freeQueueBuffers) {
		var buffer *audioQueueBuffer
		if code := audioQueue.allocate(out.queue, outputBlockFrames*bytesPerFrame, &buffer); code != 0 {
			out.close(false)
			return nil, queueError("allocate buffer", code)
		}
		freeQueueBuffers <- buffer
	}
	return out, nil
}

func (o *appleOutput) write(samples []float32) error {
	// A paused queue may still own both buffers. Restart it before waiting
	// for a callback to return one.
	if o.paused {
		if code := audioQueue.start(o.queue, 0); code != 0 {
			return queueError("resume", code)
		}
		o.paused = false
	}
	o.timeout.Reset(time.Second)
	var buffer *audioQueueBuffer
	select {
	case buffer = <-freeQueueBuffers:
		o.timeout.Stop()
	case <-o.timeout.C:
		return errors.New("audio: Audio Queue buffer timed out")
	}
	buffer.size = uint32(len(samples) * bytesPerSample)
	purego.SyscallN(audioQueue.memcpy, buffer.data, uintptr(unsafe.Pointer(&samples[0])), uintptr(buffer.size))
	r, _, _ := purego.SyscallN(audioQueue.enqueue, o.queue, uintptr(unsafe.Pointer(buffer)), 0, 0)
	if int32(r) != 0 {
		return queueError("enqueue", int32(r))
	}
	if !o.started {
		if code := audioQueue.start(o.queue, 0); code != 0 {
			return queueError("start", code)
		}
		o.started = true
	}
	return nil
}

func (o *appleOutput) close(drain bool) error {
	code := audioQueue.dispose(o.queue, !drain || !o.started || o.paused)
	o.timeout.Stop()
	for len(freeQueueBuffers) > 0 {
		<-freeQueueBuffers
	}
	if code != 0 {
		return queueError("dispose", code)
	}
	return nil
}

func (o *appleOutput) idle() error {
	if code := audioQueue.pause(o.queue); code != 0 {
		return queueError("pause", code)
	}
	o.paused = true
	return nil
}

func queueError(op string, code int32) error {
	return fmt.Errorf("audio: Audio Queue %s: OSStatus %d", op, code)
}
