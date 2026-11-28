package audio

/*
#cgo LDFLAGS: -lOpenSLES
#include <SLES/OpenSLES.h>
#include <SLES/OpenSLES_Android.h>
#include <errno.h>
#include <semaphore.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

enum { bufferCount = 2 };

typedef struct {
	SLObjectItf engine, mix, player;
	SLAndroidSimpleBufferQueueItf queue;
	SLPlayItf play;
	sem_t available;
	size_t blockSamples;
	unsigned next;
	int started;
	float samples[];
} slOutput;

static void slBufferDone(SLAndroidSimpleBufferQueueItf queue, void *context) {
	slOutput *out = context;
	sem_post(&out->available);
}

static int slWaitBuffer(slOutput *out) {
	struct timespec deadline;
	clock_gettime(CLOCK_REALTIME, &deadline);
	deadline.tv_sec++;
	int result;
	do {
		result = sem_timedwait(&out->available, &deadline);
	} while (result < 0 && errno == EINTR);
	return result;
}

static SLresult slIdle(slOutput *out) {
	if (!out->started) return SL_RESULT_SUCCESS;
	for (unsigned i = 0; i < bufferCount; i++) {
		if (slWaitBuffer(out) < 0) return SL_RESULT_IO_ERROR;
	}
	SLresult result = (*out->play)->SetPlayState(out->play, SL_PLAYSTATE_STOPPED);
	for (unsigned i = 0; i < bufferCount; i++) sem_post(&out->available);
	out->started = 0;
	return result;
}

static SLresult slClose(slOutput *out, int drain) {
	SLresult result = drain ? slIdle(out) : SL_RESULT_SUCCESS;
	if (out->player) (*out->player)->Destroy(out->player);
	if (out->mix) (*out->mix)->Destroy(out->mix);
	if (out->engine) (*out->engine)->Destroy(out->engine);
	sem_destroy(&out->available);
	free(out);
	return result;
}

static SLresult slOpen(slOutput **output, unsigned rate, unsigned channels, size_t blockSamples) {
	slOutput *out = calloc(1, sizeof(*out) + bufferCount * blockSamples * sizeof(float));
	if (!out) return SL_RESULT_MEMORY_FAILURE;
	if (sem_init(&out->available, 0, bufferCount) < 0) {
		free(out);
		return SL_RESULT_RESOURCE_ERROR;
	}
	out->blockSamples = blockSamples;
	SLEngineItf engine;
	SLresult result = slCreateEngine(&out->engine, 0, NULL, 0, NULL, NULL);
	if (result != SL_RESULT_SUCCESS) goto fail;
	result = (*out->engine)->Realize(out->engine, SL_BOOLEAN_FALSE);
	if (result != SL_RESULT_SUCCESS) goto fail;
	result = (*out->engine)->GetInterface(out->engine, SL_IID_ENGINE, &engine);
	if (result != SL_RESULT_SUCCESS) goto fail;
	result = (*engine)->CreateOutputMix(engine, &out->mix, 0, NULL, NULL);
	if (result != SL_RESULT_SUCCESS) goto fail;
	result = (*out->mix)->Realize(out->mix, SL_BOOLEAN_FALSE);
	if (result != SL_RESULT_SUCCESS) goto fail;

	SLDataLocator_AndroidSimpleBufferQueue locator = { SL_DATALOCATOR_ANDROIDSIMPLEBUFFERQUEUE, bufferCount };
	SLAndroidDataFormat_PCM_EX format = {
		SL_ANDROID_DATAFORMAT_PCM_EX, channels, rate * 1000, 32, 32,
		SL_SPEAKER_FRONT_LEFT | SL_SPEAKER_FRONT_RIGHT,
		SL_BYTEORDER_LITTLEENDIAN, SL_ANDROID_PCM_REPRESENTATION_FLOAT
	};
	SLDataSource source = { &locator, &format };
	SLDataLocator_OutputMix outputMix = { SL_DATALOCATOR_OUTPUTMIX, out->mix };
	SLDataSink sink = { &outputMix, NULL };
	const SLInterfaceID queueID = SL_IID_ANDROIDSIMPLEBUFFERQUEUE;
	const SLboolean required = SL_BOOLEAN_TRUE;
	result = (*engine)->CreateAudioPlayer(engine, &out->player, &source, &sink, 1, &queueID, &required);
	if (result != SL_RESULT_SUCCESS) goto fail;
	result = (*out->player)->Realize(out->player, SL_BOOLEAN_FALSE);
	if (result != SL_RESULT_SUCCESS) goto fail;
	result = (*out->player)->GetInterface(out->player, SL_IID_PLAY, &out->play);
	if (result != SL_RESULT_SUCCESS) goto fail;
	result = (*out->player)->GetInterface(out->player, queueID, &out->queue);
	if (result != SL_RESULT_SUCCESS) goto fail;
	result = (*out->queue)->RegisterCallback(out->queue, slBufferDone, out);
	if (result != SL_RESULT_SUCCESS) goto fail;
	*output = out;
	return SL_RESULT_SUCCESS;
fail:
	slClose(out, 0);
	return result;
}

static SLresult slWrite(slOutput *out, const float *samples, unsigned bytes) {
	if (slWaitBuffer(out) < 0) return SL_RESULT_IO_ERROR;
	float *buffer = out->samples + out->next * out->blockSamples;
	memcpy(buffer, samples, bytes);
	SLresult result = (*out->queue)->Enqueue(out->queue, buffer, bytes);
	if (result != SL_RESULT_SUCCESS) return result;
	out->next = (out->next + 1) % bufferCount;
	if (!out->started) {
		result = (*out->play)->SetPlayState(out->play, SL_PLAYSTATE_PLAYING);
		out->started = result == SL_RESULT_SUCCESS;
	}
	return result;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type androidOutput struct{ stream *C.slOutput }

func openNativeOutput() (androidOutput, error) {
	var out androidOutput
	code := C.slOpen(&out.stream, outputSampleRate, outputChannels, outputBlockFrames*outputChannels)
	return out, slError("open", code)
}

func (o androidOutput) write(samples []float32) error {
	code := C.slWrite(o.stream, (*C.float)(unsafe.Pointer(&samples[0])), C.uint(len(samples)*bytesPerSample))
	return slError("enqueue", code)
}

func (o androidOutput) close(drain bool) error {
	var wait C.int
	if drain {
		wait = 1
	}
	return slError("close", C.slClose(o.stream, wait))
}

func (o androidOutput) idle() error { return slError("idle", C.slIdle(o.stream)) }

func slError(op string, code C.SLresult) error {
	if code != C.SL_RESULT_SUCCESS {
		return fmt.Errorf("audio: OpenSL ES %s: result %d", op, code)
	}
	return nil
}
