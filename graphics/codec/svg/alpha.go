package svg

import (
	"image"
	"sync"

	"github.com/onix-engine/onix/internal/reuse"
)

type transparentColorScratch struct {
	distances []uint16
	retention reuse.Retention
}

// Bound each idle workspace to a 2048x2048 distance table. The pool can
// discard idle workspaces at GC; concurrent decodes borrow independent ones.
const transparentColorScratchLimit = 8 << 20

var transparentColorScratchPool = sync.Pool{New: func() any { return &transparentColorScratch{} }}

func acquireTransparentColorScratch(length int) *transparentColorScratch {
	scratch := transparentColorScratchPool.Get().(*transparentColorScratch)
	scratch.distances = reuse.Resize(scratch.distances, length)
	return scratch
}

func (s *transparentColorScratch) trim() {
	if cap(s.distances) > transparentColorScratchLimit/2 {
		s.distances = nil
		s.retention = reuse.Retention{}
	} else if capacity := s.retention.TrimCapacity(cap(s.distances), len(s.distances), 4096); capacity > 0 {
		// The next transform initializes every entry, so no copy is needed.
		s.distances = make([]uint16, 0, capacity)
	}
}

func releaseTransparentColorScratch(scratch *transparentColorScratch) {
	scratch.trim()
	transparentColorScratchPool.Put(scratch)
}

// The sprite pipeline samples straight alpha and premultiplies afterwards.
// Extend edge colors through transparent texels so filtering and mip generation
// do not mix the contour with transparent black. Alpha and visible colors stay
// intact. The two dependent sweeps compute Manhattan distance in linear time.
func extendTransparentColors(img *image.NRGBA) {
	width, height := img.Rect.Dx(), img.Rect.Dy()
	if width == 0 || height == 0 {
		return
	}
	// Most fully opaque SVGs need no scratch storage.
	transparent := false
	visible := false
	for offset := 3; offset < len(img.Pix); offset += 4 {
		transparent = transparent || img.Pix[offset] == 0
		visible = visible || img.Pix[offset] != 0
		if transparent && visible {
			break
		}
	}
	if !transparent || !visible {
		return
	}
	// The sentinel is width+height. Convert distances to int before adding
	// one below, including when the uint16 sentinel is 65535.
	if width+height > 65535 {
		extendTransparentColorsWithDistances(img, make([]int, width*height))
		return
	}
	scratch := acquireTransparentColorScratch(width * height)
	extendTransparentColorsWithDistances(img, scratch.distances)
	releaseTransparentColorScratch(scratch)
}

func extendTransparentColorsWithDistances[T uint16 | int](img *image.NRGBA, distances []T) {
	width, height := img.Rect.Dx(), img.Rect.Dy()
	copyColor := func(target, source int) {
		copy(img.Pix[target*4:target*4+3], img.Pix[source*4:source*4+3])
	}
	for y := range height {
		for x := range width {
			index := y*width + x
			if img.Pix[index*4+3] != 0 {
				distances[index] = 0
				continue
			}
			distances[index] = T(width + height)
			if x > 0 && int(distances[index-1])+1 < int(distances[index]) {
				distances[index] = distances[index-1] + 1
				copyColor(index, index-1)
			}
			if y > 0 && int(distances[index-width])+1 < int(distances[index]) {
				distances[index] = distances[index-width] + 1
				copyColor(index, index-width)
			}
		}
	}
	for y := height - 1; y >= 0; y-- {
		for x := width - 1; x >= 0; x-- {
			index := y*width + x
			if distances[index] == 0 {
				continue
			}
			if x+1 < width && int(distances[index+1])+1 < int(distances[index]) {
				distances[index] = distances[index+1] + 1
				copyColor(index, index+1)
			}
			if y+1 < height && int(distances[index+width])+1 < int(distances[index]) {
				distances[index] = distances[index+width] + 1
				copyColor(index, index+width)
			}
		}
	}
}
