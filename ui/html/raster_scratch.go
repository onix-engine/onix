package html

import (
	"image"
	"sync"
)

// Only intermediates belong here. Raster results and texture pixels outlive a
// paint operation and must never be returned to this pool.
type htmlRasterScratch struct {
	htmlFilterScratch
	paint image.RGBA
}

const htmlRasterScratchByteLimit = 32 << 20

var htmlRasterScratchPool = sync.Pool{New: func() any { return &htmlRasterScratch{} }}

func acquireHTMLRasterScratch() *htmlRasterScratch {
	return htmlRasterScratchPool.Get().(*htmlRasterScratch)
}

func releaseHTMLRasterScratch(scratch *htmlRasterScratch) {
	bytes := cap(scratch.rows)*32 + cap(scratch.kernel)*8 + cap(scratch.matrices)*160 +
		cap(scratch.shadow.Pix) + cap(scratch.mask)*8 + cap(scratch.maskLine)*8 + cap(scratch.paint.Pix)
	if bytes > htmlRasterScratchByteLimit {
		*scratch = htmlRasterScratch{}
	}
	htmlRasterScratchPool.Put(scratch)
}
