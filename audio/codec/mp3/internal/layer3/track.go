package layer3

import (
	"io"
	"io/fs"
	"sync/atomic"
)

// Track indexes compressed packets once for all players of a streaming clip.
type Track struct {
	packets             []packet
	rate, packetSamples int
}

type packet struct {
	offset    int64
	dataBytes int
}

func Inspect(r io.Reader) (*Track, error) {
	var track Track
	scan := newScanner(r)
	for {
		f, err := scan.next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(track.packets) == 0 {
			if _, err := readSide(f); err != nil {
				return nil, err
			}
			track.rate = rates[f.rateIndex]
			track.packetSamples = f.granules * granuleSamples
		} else if track.rate != rates[f.rateIndex] {
			return nil, errRateChange
		}
		track.packets = append(track.packets, packet{scan.offset, len(f.data)})
	}
	if len(track.packets) == 0 {
		return nil, errNoFrames
	}
	return &track, nil
}

func (t *Track) SampleRate() int { return t.rate }
func (t *Track) Frames() int64   { return int64(len(t.packets)) * int64(t.packetSamples) }

// Open takes ownership of an independent reader positioned at the start of the track.
func (t *Track) Open(r io.ReadSeekCloser) *Reader {
	return &Reader{track: t, encoded: r, scan: newScanner(r), current: -1}
}

type Reader struct {
	encoded io.ReadSeekCloser
	track   *Track
	scan    scanner
	decoder decoder
	pcm     [maxPCM]float32
	current int
	closed  atomic.Bool
}

func (r *Reader) Closed() bool { return r.closed.Load() }

// StereoAt returns one stereo sample frame at the original sample rate.
func (r *Reader) StereoAt(frame int64) ([2]float32, error) {
	if r.Closed() {
		return [2]float32{}, fs.ErrClosed
	}
	if frame < 0 || frame >= r.track.Frames() {
		return [2]float32{}, io.EOF
	}
	size := int64(r.track.packetSamples)
	target := int(frame / size)
	if target != r.current {
		if err := r.load(target); err != nil {
			r.current = -2 // discard partially decoded state before retrying
			return [2]float32{}, err
		}
	}
	i := 2 * int(frame%size)
	return [2]float32{r.pcm[i], r.pcm[i+1]}, nil
}

func (r *Reader) load(target int) error {
	first, decodeFrom := target, target
	if target != r.current+1 {
		// Overlap-add needs one previous granule. The polyphase filter needs
		// another 15 slots of 32 samples. Earlier packets supply reservoir bytes only.
		const history = granuleSamples + 15*32
		previous := (history + r.track.packetSamples - 1) / r.track.packetSamples
		decodeFrom = max(0, target-previous)
		first = decodeFrom
		for stored := 0; first > 0 && stored < maxReservoir; {
			first--
			stored += r.track.packets[first].dataBytes
		}
		offset := r.track.packets[first].offset
		if _, err := r.encoded.Seek(offset, io.SeekStart); err != nil {
			return err
		}
		r.scan.input.Reset(r.encoded)
		r.scan.offset, r.scan.pending = offset, 0
		r.decoder = decoder{}
	}
	for i := first; i <= target; i++ {
		f, err := r.scan.next()
		if err == io.EOF {
			return io.ErrUnexpectedEOF
		}
		if err != nil {
			return err
		}
		p := r.track.packets[i]
		if r.scan.offset != p.offset || len(f.data) != p.dataBytes || rates[f.rateIndex] != r.track.rate {
			return errFrame
		}
		if i < decodeFrom {
			r.decoder.appendData(f.data)
		} else if err := r.decoder.decode(f, r.pcm[:]); err != nil {
			return err
		}
	}
	r.current = target
	return nil
}

func (r *Reader) Close() error {
	if r.closed.Swap(true) {
		return nil
	}
	return r.encoded.Close()
}
