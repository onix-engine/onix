package physics

import "sync"

// Bound individual buffers and tables so large imported maps do not leave
// their temporary storage cached after cooking.
const maxCachedCookEntries = 64 << 10

type triangleMeshCookScratch struct {
	unique map[Vector3D]uint32
	remap  []uint32
	refs   []triangleMeshRef
}

var triangleMeshCookPool = sync.Pool{New: func() any { return new(triangleMeshCookScratch) }}

func releaseTriangleMeshCookScratch(scratch *triangleMeshCookScratch) {
	// The map is initially sized for the input, even when most vertices are
	// duplicates. Its reserved size is bounded by the remap capacity.
	if cap(scratch.remap) > maxCachedCookEntries {
		scratch.unique = nil
	} else {
		clear(scratch.unique)
	}
	scratch.remap = resetCookBuffer(scratch.remap)
	scratch.refs = resetCookBuffer(scratch.refs)
	triangleMeshCookPool.Put(scratch)
}

type horizonEdge struct{ a, b, count uint32 }

type convexHullCookScratch struct {
	horizon     map[[2]uint32]horizonEdge
	seen        map[[2]uint32]struct{}
	faces       []ConvexFace
	horizonPeak int
}

var convexHullCookPool = sync.Pool{New: func() any { return new(convexHullCookScratch) }}

func releaseConvexHullCookScratch(scratch *convexHullCookScratch) {
	scratch.faces = resetCookBuffer(scratch.faces)
	if scratch.horizonPeak > maxCachedCookEntries {
		scratch.horizon = nil
	} else {
		clear(scratch.horizon)
	}
	scratch.horizonPeak = 0
	if len(scratch.seen) > maxCachedCookEntries {
		scratch.seen = nil
	} else {
		clear(scratch.seen)
	}
	convexHullCookPool.Put(scratch)
}

// Cooking buffers contain no references and are overwritten before use.
func resetCookBuffer[T any](values []T) []T {
	if cap(values) > maxCachedCookEntries {
		return nil
	}
	return values[:0]
}
