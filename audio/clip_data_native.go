//go:build !js

package audio

import (
	"encoding/binary"
	"math"
)

func staticClipData(samples []float32, _ bool) ([]byte, []float32) {
	pcm := make([]byte, len(samples)*BytesPerSample)
	for index, sample := range samples {
		binary.LittleEndian.PutUint32(pcm[index*BytesPerSample:], math.Float32bits(sample))
	}
	return pcm, nil
}
