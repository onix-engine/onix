package render

import "slices"

// Static index conversion borrows scratch only for the duration of an upload.
var meshIndexBuffers = bufferPool[uint16]{maxCapacity: 2 << 20}

func compactMeshIndices(indices []uint32, maxIndex uint32, padToFourBytes bool) *pooledBuffer[uint16] {
	for _, index := range indices {
		if index > maxIndex {
			return nil
		}
	}
	n := len(indices)
	if padToFourBytes {
		n += n & 1
	}
	buffer := meshIndexBuffers.get()
	buffer.data = slices.Grow(buffer.data[:0], n)[:n]
	for i, index := range indices {
		buffer.data[i] = uint16(index)
	}
	clear(buffer.data[len(indices):])
	return buffer
}

func glIndexType(stride uint32) int {
	if stride == 2 {
		return 0x1403 // UNSIGNED_SHORT
	}
	return 0x1405 // UNSIGNED_INT
}
