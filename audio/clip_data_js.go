//go:build js

package audio

import "slices"

func staticClipData(samples []float32, owned bool) ([]byte, []float32) {
	if !owned {
		samples = slices.Clone(samples)
	}
	return nil, samples
}
