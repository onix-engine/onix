package render

import "unsafe"

// BufferMemoryStatistics reports reserved transient buffers, including all
// frames in flight. It excludes textures, resident meshes and driver allocations.
// NativeScratchBytes reports native buffer, upload and descriptor metadata,
// retirement queues and CPU snapshots.
// ScratchBytes is Go memory and is already included in runtime.HeapAlloc.
type BufferMemoryStatistics struct {
	InstanceBytes      uint64
	StagingBytes       uint64
	ReadbackBytes      uint64
	NativeScratchBytes uint64
	ScratchBytes       uint64
}

func (s BufferMemoryStatistics) NativeBytes() uint64 {
	return s.InstanceBytes + s.StagingBytes + s.ReadbackBytes + s.NativeScratchBytes
}

// CurrentBufferMemoryStatistics must be called on the rendering thread.
func CurrentBufferMemoryStatistics() BufferMemoryStatistics {
	if driver, ok := Current().(interface{ BufferMemoryStatistics() BufferMemoryStatistics }); ok {
		return driver.BufferMemoryStatistics()
	}
	return BufferMemoryStatistics{}
}

func sliceCapacityBytes[T any](values []T) uint64 {
	var value T
	return uint64(cap(values)) * uint64(unsafe.Sizeof(value))
}
