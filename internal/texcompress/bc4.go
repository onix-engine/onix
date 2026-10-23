package texcompress

// bc4Weights are the palette blend weights for the eight-value mode, expressed
// in sevenths so endpoint refinement stays in integer arithmetic.
var bc4Weights = [8][2]int{{7, 0}, {0, 7}, {6, 1}, {5, 2}, {4, 3}, {3, 4}, {2, 5}, {1, 6}}

// bc4SixWeights are the weights for the six-value mode, in fifths.
var bc4SixWeights = [6][2]int{{0, 0}, {0, 0}, {4, 1}, {3, 2}, {2, 3}, {1, 4}}

// encodeBC4 writes one 4x4 block in BC4 layout: two single-channel endpoints
// followed by sixteen 3-bit indices. channel selects which source channel is
// compressed, so BC5 can reuse this encoder for each of its two planes.
func encodeBC4(dst []byte, scratch *blockScratch, channel int) {
	var values [blockSize * blockSize]uint8
	low, high := uint8(255), uint8(0)
	for index := range blockSize * blockSize {
		value := scratch[index*4+channel]
		values[index] = value
		low, high = min(low, value), max(high, value)
	}

	endpoint0, endpoint1 := bc4Quantize(high, low)
	indices := bc4Assign(endpoint0, endpoint1, &values)
	var refined [2]uint8
	if endpoint0 > endpoint1 {
		// Refinement assumes the eight-value palette, so it only applies while
		// the endpoints stay ordered.
		for range 2 {
			if !bc4Refine(&refined, &values, indices) {
				break
			}
			endpoint0, endpoint1 = bc4Quantize(refined[0], refined[1])
			indices = bc4Assign(endpoint0, endpoint1, &values)
		}
	}

	dst[0], dst[1] = endpoint0, endpoint1
	var bits uint64
	for index := range blockSize * blockSize {
		bits |= uint64(indices[index]) << (3 * index)
	}
	for index := range 6 {
		dst[2+index] = uint8(bits >> (8 * index))
	}
}

// bc4Quantize orders the endpoints so the decoder picks the eight-value mode,
// which interpolates with seven steps instead of five.
func bc4Quantize(endpoint0, endpoint1 uint8) (uint8, uint8) {
	if endpoint0 < endpoint1 {
		return endpoint1, endpoint0
	}
	return endpoint0, endpoint1
}

// bc4Palette expands two endpoints into the eight values the sampler
// reconstructs. The six-value mode also reserves the last two entries for zero
// and full scale, which is what makes BC4 usable for signed data.
func bc4Palette(endpoint0, endpoint1 uint8) [8]uint8 {
	var palette [8]uint8
	palette[0], palette[1] = endpoint0, endpoint1
	if endpoint0 > endpoint1 {
		for index := 2; index < 8; index++ {
			weight := bc4Weights[index]
			palette[index] = uint8((weight[0]*int(endpoint0) + weight[1]*int(endpoint1)) / 7)
		}
		return palette
	}
	for index := 2; index < 6; index++ {
		weight := bc4SixWeights[index]
		palette[index] = uint8((weight[0]*int(endpoint0) + weight[1]*int(endpoint1)) / 5)
	}
	palette[6], palette[7] = 0, 255
	return palette
}

func bc4Assign(endpoint0, endpoint1 uint8, values *[blockSize * blockSize]uint8) [blockSize * blockSize]uint8 {
	palette := bc4Palette(endpoint0, endpoint1)
	var indices [blockSize * blockSize]uint8
	for index := range blockSize * blockSize {
		value := int(values[index])
		best, bestDistance := 0, 1<<30
		for candidate := range 8 {
			difference := value - int(palette[candidate])
			if distance := difference * difference; distance < bestDistance {
				bestDistance, best = distance, candidate
			}
		}
		indices[index] = uint8(best)
	}
	return indices
}

// bc4Refine solves the least-squares endpoint fit for a fixed index assignment.
func bc4Refine(refined *[2]uint8, values *[blockSize * blockSize]uint8, indices [blockSize * blockSize]uint8) bool {
	var normal [3]int
	var rhs [2]int
	for index := range blockSize * blockSize {
		weight := bc4Weights[min(indices[index], 7)]
		a, b := weight[0], weight[1]
		normal[0] += a * a
		normal[1] += a * b
		normal[2] += b * b
		value := 7 * int(values[index])
		rhs[0] += a * value
		rhs[1] += b * value
	}
	determinant := normal[0]*normal[2] - normal[1]*normal[1]
	if determinant == 0 {
		return false
	}
	first := (rhs[0]*normal[2] - rhs[1]*normal[1]) / determinant
	second := (rhs[1]*normal[0] - rhs[0]*normal[1]) / determinant
	refined[0] = uint8(min(max(first, 0), 255))
	refined[1] = uint8(min(max(second, 0), 255))
	return true
}

// encodeBC5 writes two BC4 planes, red first then green. Normal maps store X and
// Y in those two channels, and the eight-value mode leaves enough precision that
// Z can be reconstructed from the unit-length constraint.
func encodeBC5(dst []byte, scratch *blockScratch) {
	encodeBC4(dst[:bc4BlockBytes], scratch, 0)
	encodeBC4(dst[bc4BlockBytes:bc5BlockBytes], scratch, 1)
}
