package layer3

import (
	"bytes"
	"errors"
	"io"
	"math"
)

const maxPCM = 2 * granuleSamples * 2 // two granules, stereo

type decoder struct {
	filter    [2]filterbank
	reservoir [maxReservoir + maxFrameBytes]byte
	stored    int
	mono      bool
}

// A frame's main data may begin in an earlier packet. Keep the compressed
// bytes even when starting partway through a track without enough history.
func (d *decoder) appendData(data []byte) int {
	retain := min(d.stored, maxReservoir)
	copy(d.reservoir[:retain], d.reservoir[d.stored-retain:d.stored])
	d.stored = retain + copy(d.reservoir[retain:], data)
	return retain
}

func (d *decoder) decode(f frame, pcm []float32) error {
	side, err := readSide(f)
	if err != nil {
		return err
	}
	start := d.appendData(f.data) - side.back
	if start < 0 {
		clear(pcm[:f.granules*granuleSamples*2])
		return nil
	}
	data := d.reservoir[start:d.stored]
	if d.mono && f.channels == 2 {
		d.filter[1] = filterbank{}
	}
	d.mono = f.channels == 1
	offset := 0
	var scales [2]scaleFactors
	for gr := range f.granules {
		var x [2][576]float32
		for ch := range f.channels {
			g := &side.channel[gr][ch]
			end := offset + g.length
			if end > len(data)*8 {
				return errFrame
			}
			b := bits{data: data, pos: offset, end: end}
			reuse := 0
			if gr == 1 {
				reuse = side.reuse[ch]
			}
			scales[ch].read(&b, g, f.granules == 2, ch == 1 && f.stereo&1 != 0, reuse)
			if err := readSpectrum(&b, g, &x[ch]); err != nil {
				return err
			}
			requantize(&x[ch], g, &layouts[f.rateIndex][g.kind()], &scales[ch])
			offset = end
		}
		if f.stereo != 0 {
			right := &side.channel[gr][1]
			jointStereo(&x, f.header, right, &scales[1], &layouts[f.rateIndex][right.kind()])
		}
		out := pcm[gr*granuleSamples*2 : (gr+1)*granuleSamples*2]
		for ch := range f.channels {
			g := &side.channel[gr][ch]
			d.filter[ch].synthesize(&x[ch], g, f.rateIndex, out, ch)
		}
		if f.channels == 1 {
			for i := 0; i < len(out); i += 2 {
				out[i+1] = out[i]
			}
		}
	}
	return nil
}

// Decode returns stereo float samples at their encoded sample rate.
func Decode(data []byte) ([]float32, int, error) {
	input := bytes.NewReader(data)
	scan := newScanner(input)
	count := 0
	rate := 0
	for {
		f, err := scan.next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, err
		}
		if rate != 0 && rate != rates[f.rateIndex] {
			return nil, 0, errRateChange
		}
		rate = rates[f.rateIndex]
		n := f.granules * granuleSamples * 2
		if count > math.MaxInt/4-n {
			return nil, 0, errors.New("mp3: decoded PCM exceeds addressable memory")
		}
		count += n
	}
	if rate == 0 {
		return nil, 0, errNoFrames
	}
	// A header pass gives an exact size for VBR and tagged files. Decode each
	// packet directly into the final slice, reusing the scanner's buffer.
	input.Reset(data)
	scan.input.Reset(input)
	scan.offset, scan.pending = 0, 0
	samples := make([]float32, count)
	var d decoder
	for written := 0; written < count; {
		f, err := scan.next()
		if err != nil {
			return nil, 0, err
		}
		n := f.granules * granuleSamples * 2
		if err := d.decode(f, samples[written:written+n]); err != nil {
			return nil, 0, err
		}
		written += n
	}
	return samples, rate, nil
}
