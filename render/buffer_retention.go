package render

const bufferTrimDelay = 30

type bufferRetention struct {
	peak   int
	frames int
}

// trimCapacity observes one completed frame, retaining twice the largest demand
// in a sustained low-load window. Zero means the current capacity stays warm.
func (r *bufferRetention) trimCapacity(capacity, used, minimum int) int {
	if capacity <= minimum || used > capacity/4 {
		*r = bufferRetention{}
		return 0
	}
	r.peak = max(r.peak, used)
	r.frames++
	if r.frames < bufferTrimDelay {
		return 0
	}
	target := max(minimum, r.peak*2)
	*r = bufferRetention{}
	return target
}

func trimUploadBuffer[T any](buffer []T, retention *bufferRetention, used, minimum int) []T {
	idle := retention.peak == 0 && used == 0
	if capacity := retention.trimCapacity(cap(buffer), used, minimum); capacity != 0 {
		if idle {
			return nil
		}
		return make([]T, 0, capacity)
	}
	return buffer[:0]
}
