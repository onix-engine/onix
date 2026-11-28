package layer3

import "math"

var longIMDCT [4][36][18]float32
var shortIMDCT [12][6]float32
var synthCos [16][2][32]float32
var aliasCos, aliasSin [8]float32

func init() {
	for n := range 36 {
		t := float64(n) + 0.5
		long := math.Sin(math.Pi * t / 36)
		start, stop := long, long
		if n >= 18 {
			switch {
			case n < 24:
				start = 1
			case n < 30:
				start = math.Sin(math.Pi * (t - 18) / 12)
			default:
				start = 0
			}
		} else {
			switch {
			case n < 6:
				stop = 0
			case n < 12:
				stop = math.Sin(math.Pi * (t - 6) / 12)
			default:
				stop = 1
			}
		}
		for k := range 18 {
			basis := math.Cos(math.Pi / 36 * (t + 9) * float64(2*k+1))
			longIMDCT[0][n][k] = float32(basis * long)
			longIMDCT[1][n][k] = float32(basis * start)
			longIMDCT[3][n][k] = float32(basis * stop)
		}
	}
	for n := range shortIMDCT {
		t := float64(n) + 0.5
		for k := range shortIMDCT[n] {
			shortIMDCT[n][k] = float32(math.Sin(math.Pi*t/12) * math.Cos(math.Pi/12*(t+3)*float64(2*k+1)))
		}
	}
	for i := range synthCos {
		for k := range 32 {
			synthCos[i][0][k] = float32(math.Cos(math.Pi / 64 * float64((16+i)*(2*k+1))))
			synthCos[i][1][k] = float32(math.Cos(math.Pi / 64 * float64((49+i)*(2*k+1))))
		}
	}
	for i, c := range [...]float64{-0.6, -0.535, -0.33, -0.185, -0.095, -0.041, -0.0142, -0.0037} {
		cos := 1 / math.Sqrt(1+c*c)
		aliasCos[i], aliasSin[i] = float32(cos), float32(c*cos)
	}
}

type filterbank struct {
	overlap [32][18]float32
	history [1024]float32
	head    int
}

func (f *filterbank) synthesize(x *[576]float32, g *granule, rateIndex int, pcm []float32, ch int) {
	layout := &layouts[rateIndex][g.kind()]
	if g.blockType == 2 {
		reorderShort(x, layout)
	}
	for boundary := 18; boundary < layout.longLines; boundary += 18 {
		for i, c := range aliasCos {
			a, b := x[boundary-1-i], x[boundary+i]
			x[boundary-1-i] = a*c - b*aliasSin[i]
			x[boundary+i] = b*c + a*aliasSin[i]
		}
	}
	// The lowest subbands in a mixed block keep their normal window
	// through the start, short and stop blocks.
	normalBands := 0
	if g.mixed {
		normalBands = layouts[rateIndex][mixedBlock].longLines / 18
	}
	for sb := range 32 {
		part := x[sb*18 : sb*18+18]
		var block [36]float32
		window := g.blockType
		if sb < normalBands {
			window = 0
		}
		imdct(part, window, &block)
		for i := range 18 {
			sample := block[i] + f.overlap[sb][i]
			if sb&i&1 != 0 {
				sample = -sample
			}
			part[i] = sample
			f.overlap[sb][i] = block[i+18]
		}
	}
	f.polyphase(x, pcm, ch)
}

func reorderShort(x *[576]float32, layout *bandLayout) {
	var ordered [576]float32
	for _, band := range layout.band[:layout.count] {
		if band.window < 0 {
			continue
		}
		dest := int(band.reordered)
		for _, v := range x[int(band.start) : int(band.start)+int(band.width)] {
			ordered[dest] = v
			dest += 3
		}
	}
	copy(x[layout.longLines:], ordered[layout.longLines:])
}

func imdct(part []float32, window int, block *[36]float32) {
	if window == 2 {
		for win := range 3 {
			for n := range shortIMDCT {
				var sum float32
				for k, c := range shortIMDCT[n] {
					sum += part[3*k+win] * c
				}
				block[6+6*win+n] += sum
			}
		}
		return
	}
	for n := range block {
		var sum float32
		for k, c := range longIMDCT[window][n] {
			sum += part[k] * c
		}
		block[n] = sum
	}
}

func (f *filterbank) polyphase(x *[576]float32, pcm []float32, ch int) {
	for slot := range 18 {
		var subband [32]float32
		for sb := range subband {
			subband[sb] = x[sb*18+slot]
		}
		f.head = (f.head - 64) & 1023
		row := f.history[f.head : f.head+64]
		// V[32-i] = -V[i], V[96-i] = V[i], and V[16] = 0.
		row[16] = 0
		for i := range synthCos {
			var low, high float32
			for j, sample := range subband {
				low += synthCos[i][0][j] * sample
				high += synthCos[i][1][j] * sample
			}
			row[i], row[32-i] = low, -low
			row[i+33], row[63-i] = high, high
		}
		var samples [32]float32
		for phase := range 8 {
			lo := (f.head + phase*128) & 1023
			hi := (lo + 96) & 1023
			for i := range samples {
				samples[i] += f.history[lo+i]*synthesisWindow[phase*64+i] + f.history[hi+i]*synthesisWindow[phase*64+32+i]
			}
		}
		for i, v := range samples {
			pcm[2*(slot*32+i)+ch] = max(-1, min(v/65536, 1))
		}
	}
}
