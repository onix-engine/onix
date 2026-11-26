package graphics

import "slices"

// Atlas rows arrive incrementally. Allocate touched pages once, without
// copying prior rows or retaining the gaps before a distant region.
const texturePageBytes = 8 << 10

type texturePages []*[texturePageBytes]byte

func (pages *texturePages) update(updates []TextureRegionUpdate, stride, texelBytes, end int) {
	count := (end-1)/texturePageBytes + 1
	if count > len(*pages) {
		*pages = slices.Grow(*pages, count-len(*pages))[:count]
	}
	for _, update := range updates {
		rowBytes := update.Width * texelBytes
		offset := update.Y*stride + update.X*texelBytes
		for row := range update.Height {
			src := update.Pixels[row*rowBytes : (row+1)*rowBytes]
			for dst := offset; len(src) > 0; {
				i := dst / texturePageBytes
				page := (*pages)[i]
				if page == nil {
					page = new([texturePageBytes]byte)
					(*pages)[i] = page
				}
				n := copy(page[dst%texturePageBytes:], src)
				src = src[n:]
				dst += n
			}
			offset += stride
		}
	}
}

func (pages texturePages) restore(dst []byte) {
	for _, page := range pages {
		n := min(len(dst), texturePageBytes)
		if page == nil {
			clear(dst[:n])
		} else {
			copy(dst[:n], page[:n])
		}
		dst = dst[n:]
	}
	clear(dst)
}
