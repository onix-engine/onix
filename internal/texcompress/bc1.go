package texcompress

const (
	bc1BlockBytes = 8
	bc4BlockBytes = 8
	// BC3 is an alpha block followed by a colour block; BC5 is a red block
	// followed by a green block.
	bc3BlockBytes = 2 * bc1BlockBytes
	bc5BlockBytes = 2 * bc4BlockBytes
)

// bc1Weights are the palette blend weights for the opaque four-colour mode,
// expressed in thirds so endpoint refinement stays in integer arithmetic.
// Index 0 is endpoint zero, index 1 is endpoint one, and indices 2 and 3 are the
// two interpolants.
var bc1Weights = [4][2]int{{3, 0}, {0, 3}, {2, 1}, {1, 2}}

// encodeBC1 writes one 4x4 block in BC1 (DXT1) layout: two RGB565 endpoints
// followed by sixteen 2-bit palette indices.
//
// forceFourColor keeps the block in the opaque four-colour mode. BC3 needs that
// because it reads colour from this block while alpha comes from a separate
// block, so index 3 must not decode to transparent black.
func encodeBC1(dst []byte, scratch *blockScratch, forceFourColor bool) {
	var low, high [3]uint8
	low = [3]uint8{255, 255, 255}
	for index := range blockSize * blockSize {
		offset := index * 4
		for channel := range 3 {
			value := scratch[offset+channel]
			low[channel] = min(low[channel], value)
			high[channel] = max(high[channel], value)
		}
	}

	// Start from the bounding box diagonal, then refine the endpoints against
	// the index assignment. A raw bounding box spends most of the palette on
	// blocks whose extremes are outliers.
	first, second := high, low
	indices := bc1Assign(first, second, scratch, forceFourColor)
	var refined [2][3]uint8
	for range 2 {
		if !bc1Refine(&refined, scratch, indices) {
			break
		}
		first, second = refined[0], refined[1]
		indices = bc1Assign(first, second, scratch, forceFourColor)
	}

	endpoint0, endpoint1 := bc1Quantize(first, second, forceFourColor)
	dst[0], dst[1] = uint8(endpoint0), uint8(endpoint0>>8)
	dst[2], dst[3] = uint8(endpoint1), uint8(endpoint1>>8)

	var bits uint32
	for index := range blockSize * blockSize {
		bits |= uint32(indices[index]) << (2 * index)
	}
	dst[4], dst[5] = uint8(bits), uint8(bits>>8)
	dst[6], dst[7] = uint8(bits>>16), uint8(bits>>24)
}

// bc1Quantize packs two endpoints and guarantees the mode the decoder will use.
// Endpoints are ordered so the result is the opaque four-colour mode whenever
// the caller requires it.
func bc1Quantize(endpoint0, endpoint1 [3]uint8, forceFourColor bool) (uint16, uint16) {
	high := pack565(endpoint0[0], endpoint0[1], endpoint0[2])
	low := pack565(endpoint1[0], endpoint1[1], endpoint1[2])
	if high < low {
		high, low = low, high
	}
	if forceFourColor && high == low {
		if high == 0xffff {
			low = high - 1
		} else {
			high++
		}
	}
	return high, low
}

// bc1Palette expands two packed endpoints into the four colours the sampler
// reconstructs. In the three-colour mode the last entry is transparent black,
// which is what makes BC1 usable for punch-through alpha.
func bc1Palette(endpoint0, endpoint1 uint16) [4][4]uint8 {
	var palette [4][4]uint8
	colors := [2][3]uint8{}
	colors[0][0], colors[0][1], colors[0][2] = unpack565(endpoint0)
	colors[1][0], colors[1][1], colors[1][2] = unpack565(endpoint1)
	threeColor := endpoint0 <= endpoint1
	for index := range 4 {
		if threeColor && index == 3 {
			continue
		}
		weight := bc1Weights[index]
		for channel := range 3 {
			palette[index][channel] = uint8((weight[0]*int(colors[0][channel]) + weight[1]*int(colors[1][channel])) / 3)
		}
		palette[index][3] = 255
	}
	return palette
}

func bc1Assign(endpoint0, endpoint1 [3]uint8, scratch *blockScratch, forceFourColor bool) [blockSize * blockSize]uint8 {
	high, low := bc1Quantize(endpoint0, endpoint1, forceFourColor)
	palette := bc1Palette(high, low)
	var indices [blockSize * blockSize]uint8
	for index := range blockSize * blockSize {
		offset := index * 4
		r, g, b, a := int(scratch[offset]), int(scratch[offset+1]), int(scratch[offset+2]), int(scratch[offset+3])
		best, bestDistance := 0, 1<<30
		for candidate := range 4 {
			dr := r - int(palette[candidate][0])
			dg := g - int(palette[candidate][1])
			db := b - int(palette[candidate][2])
			da := a - int(palette[candidate][3])
			if distance := dr*dr + dg*dg + db*db + da*da; distance < bestDistance {
				bestDistance, best = distance, candidate
			}
		}
		indices[index] = uint8(best)
	}
	return indices
}

// bc1Refine solves the least-squares endpoint fit for a fixed index assignment
// and reports whether the result was usable.
func bc1Refine(refined *[2][3]uint8, scratch *blockScratch, indices [blockSize * blockSize]uint8) bool {
	var normal [3][3]int
	var rhs [3][2]int
	for index := range blockSize * blockSize {
		weight := bc1Weights[indices[index]]
		a, b := weight[0], weight[1]
		normal[0][0] += a * a
		normal[0][1] += a * b
		normal[1][1] += b * b
		for channel := range 3 {
			value := 3 * int(scratch[index*4+channel])
			rhs[channel][0] += a * value
			rhs[channel][1] += b * value
		}
	}
	determinant := normal[0][0]*normal[1][1] - normal[0][1]*normal[0][1]
	if determinant == 0 {
		return false
	}
	for channel := range 3 {
		first := (rhs[channel][0]*normal[1][1] - rhs[channel][1]*normal[0][1]) / determinant
		second := (rhs[channel][1]*normal[0][0] - rhs[channel][0]*normal[0][1]) / determinant
		refined[0][channel] = uint8(min(max(first, 0), 255))
		refined[1][channel] = uint8(min(max(second, 0), 255))
	}
	return true
}
