package layer3

import "math"

const (
	longBlock = iota
	shortBlock
	mixedBlock
)

type band struct {
	start, reordered uint16
	width, frequency uint8
	window           int8 // -1 for a long window
}

type bandLayout struct {
	count, longLines int
	band             [39]band
}

var layouts [9][3]bandLayout
var power43 [15 + (1 << 13)]float32

func init() {
	for i := range power43 {
		power43[i] = float32(math.Pow(float64(i), 4.0/3.0))
	}
	for rate := range rates {
		for kind := range 3 {
			layout := &layouts[rate][kind]
			nLong, shortStart := 22, 13
			switch kind {
			case shortBlock:
				nLong, shortStart = 0, 0
			case mixedBlock:
				nLong, shortStart = 6, 3
				if rate < 3 {
					nLong = 8
				}
			}
			line := 0
			for i, width := range longWidths[rate][:nLong] {
				layout.band[i] = band{uint16(line), uint16(line), width, uint8(i), -1}
				line += int(width)
			}
			layout.longLines, layout.count = line, nLong
			for i := shortStart; i < 13; i++ {
				width := shortWidths[rate][i]
				base := line
				for w := range 3 {
					layout.band[layout.count] = band{uint16(line), uint16(base + w), width, uint8(i), int8(w)}
					layout.count++
					line += int(width)
				}
			}
		}
	}
}

func (g *granule) kind() int {
	if g.blockType != 2 {
		return longBlock
	}
	if g.mixed {
		return mixedBlock
	}
	return shortBlock
}

// ISO/IEC 11172-3 slen1/slen2 and ISO/IEC 13818-3 scalefactor partitions.
var mpeg1ScaleBits = [16][2]int{{0, 0}, {0, 1}, {0, 2}, {0, 3}, {3, 0}, {1, 1}, {1, 2}, {1, 3}, {2, 1}, {2, 2}, {2, 3}, {3, 1}, {3, 2}, {3, 3}, {4, 2}, {4, 3}}
var scaleCounts = [3][7][4]int{
	longBlock:  {{6, 5, 5, 5}, {6, 5, 5, 5}, {6, 5, 7, 3}, {11, 10, 0, 0}, {7, 7, 7, 0}, {6, 6, 6, 3}, {8, 8, 5, 0}},
	shortBlock: {{9, 9, 6, 12}, {9, 9, 9, 9}, {9, 9, 12, 6}, {18, 18, 0, 0}, {12, 12, 12, 0}, {12, 9, 9, 6}, {15, 12, 9, 0}},
	mixedBlock: {{8, 9, 6, 12}, {6, 9, 9, 9}, {6, 9, 12, 6}, {15, 18, 0, 0}, {6, 15, 12, 0}, {6, 12, 9, 6}, {6, 18, 9, 0}},
}
var scaleRadix = [6][4]int{{5, 5, 4, 4}, {5, 5, 4, 1}, {4, 3, 1, 1}, {5, 6, 6, 1}, {4, 4, 4, 1}, {4, 3, 1, 1}}
var preemphasis = [22]uint8{11: 1, 1, 1, 1, 2, 2, 3, 3, 3, 2}

type scaleFactors struct {
	value, intensity [39]uint8
}

func (s *scaleFactors) read(b *bits, g *granule, mpeg1, intensity bool, reuse int) {
	var widths [4]int
	group := 0
	if mpeg1 {
		pair := mpeg1ScaleBits[g.compress]
		widths = [4]int{pair[0], pair[0], pair[1], pair[1]}
	} else {
		code := g.compress
		form := 0
		if intensity {
			code >>= 1
			form = 3
		}
		for {
			radix := scaleRadix[form]
			size := radix[0] * radix[1] * radix[2] * radix[3]
			if code < size {
				for i := 3; i >= 0; i-- {
					widths[i] = code % radix[i]
					code /= radix[i]
				}
				break
			}
			code -= size
			form++
		}
		group = form + 1
	}
	if g.blockType == 2 {
		reuse = 0
	}
	i := 0
	for part, count := range scaleCounts[g.kind()][group] {
		width := widths[part]
		for range count {
			if reuse&(8>>part) == 0 {
				v := uint8(b.read(width))
				s.value[i], s.intensity[i] = v, v
				if intensity && !mpeg1 && width > 0 && int(v) == 1<<width-1 {
					s.intensity[i] = 255
				}
			}
			i++
		}
	}
	clear(s.value[i:])
	clear(s.intensity[i:])
}

func symbol(b *bits, book int) int {
	node := int(codebookRoot[book])
	for huffmanTree[node] >= 0 {
		if b.bit() == 0 {
			node++
		} else {
			node += int(huffmanTree[node])
		}
	}
	return int(^huffmanTree[node])
}

func coefficient(b *bits, value, extra int) float32 {
	if value == 15 {
		value += b.read(extra)
	}
	if value == 0 {
		return 0
	}
	v := power43[value]
	if b.bit() != 0 {
		return -v
	}
	return v
}

func readSpectrum(b *bits, g *granule, x *[576]float32) error {
	clear(x[:])
	n := 0
	for region, end := range [3]int{g.region0, g.region1, 576} {
		end = min(end, g.pairs*2)
		book := g.book[region]
		if book == 0 {
			n = end
			continue
		}
		if codebookRoot[book] < 0 && n < end {
			return errFrame
		}
		for n < end {
			v := symbol(b, book)
			extra := int(linbits[book])
			x[n] = coefficient(b, v>>4, extra)
			x[n+1] = coefficient(b, v&15, extra)
			n += 2
		}
	}
	if b.failed {
		return errFrame
	}
	for n <= 572 && b.pos < b.end {
		v := symbol(b, g.countBook)
		var quad [4]float32
		for i := range quad {
			if v&(8>>i) != 0 {
				quad[i] = 1
				if b.bit() != 0 {
					quad[i] = -1
				}
			}
		}
		// A partial final quadruple belongs to the stuffing bits.
		if b.failed {
			break
		}
		copy(x[n:n+4], quad[:])
		n += 4
	}
	return nil
}

func requantize(x *[576]float32, g *granule, layout *bandLayout, s *scaleFactors) {
	for i, band := range layout.band[:layout.count] {
		value := int(s.value[i])
		exp := g.gain - 210
		if band.window >= 0 {
			exp -= 8 * g.subgain[band.window]
		} else if g.preflag {
			value += int(preemphasis[band.frequency])
		}
		exp -= value << g.scaleShift
		gain := quarterPower(exp)
		part := x[int(band.start) : int(band.start)+int(band.width)]
		for j := range part {
			part[j] *= gain
		}
	}
}

var expFraction = [4]float64{1, 1.189207115002721, math.Sqrt2, 1.681792830507429}

func quarterPower(exponent int) float32 {
	return float32(math.Ldexp(expFraction[exponent&3], exponent>>2))
}

var panLeft = [7]float32{0, 0.2113248654, 0.3660254038, 0.5, 0.6339745962, 0.7886751346, 1}

func midSide(left, right []float32) {
	const gain = 1 / math.Sqrt2
	for i, mid := range left {
		side := right[i]
		left[i], right[i] = (mid+side)*gain, (mid-side)*gain
	}
}

func jointStereo(x *[2][576]float32, h header, right *granule, scales *scaleFactors, layout *bandLayout) {
	if h.stereo&1 == 0 {
		midSide(x[0][:], x[1][:])
		return
	}
	// Intensity stereo starts above the last coded line of the right channel,
	// independently in each of the three short windows.
	last := [4]int{-1, -1, -1, -1}
	bands := layout.band[:layout.count]
	for i, band := range bands {
		w := int(band.window) + 1
		for _, v := range x[1][int(band.start) : int(band.start)+int(band.width)] {
			if v != 0 {
				last[w] = i
				break
			}
		}
	}
	if layout.longLines != 0 {
		top := max(last[0], last[1], last[2], last[3])
		last = [4]int{top, top, top, top}
	}
	for i, band := range bands {
		w := int(band.window) + 1
		pos := int(scales.intensity[i])
		if band.frequency == 21 || (band.window >= 0 && band.frequency == 12) {
			prev := i - 1
			if band.window >= 0 {
				prev = i - 3
			}
			pos = int(scales.intensity[prev])
			if last[w] >= prev {
				pos = 0
				if h.granules == 2 {
					pos = 3
				}
			}
		}
		begin, end := int(band.start), int(band.start)+int(band.width)
		left, r := x[0][begin:end], x[1][begin:end]
		if i <= last[w] || pos == 255 || (h.granules == 2 && pos >= 7) {
			if h.stereo&2 != 0 {
				midSide(left, r)
			}
			continue
		}
		kl, kr := float32(1), float32(1)
		if h.granules == 2 {
			kl = panLeft[pos]
			kr = 1 - kl
		} else if pos != 0 {
			ratio := quarterPower(-((pos + 1) / 2 << (right.compress & 1)))
			if pos&1 != 0 {
				kl = ratio
			} else {
				kr = ratio
			}
		}
		for j, v := range left {
			left[j], r[j] = v*kl, v*kr
		}
	}
}
