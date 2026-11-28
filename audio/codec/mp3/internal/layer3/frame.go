// Package layer3 decodes MPEG Layer III frames to interleaved stereo PCM.
package layer3

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
)

const (
	granuleSamples = 576
	maxFrameBytes  = 1441
	maxReservoir   = 511
)

var (
	errNoFrames   = errors.New("mp3: no Layer III audio frames")
	errFrame      = errors.New("mp3: invalid Layer III frame")
	errRateChange = errors.New("mp3: sample rate changes within the track")
)

var rates = [9]int{44100, 48000, 32000, 22050, 24000, 16000, 11025, 12000, 8000}
var bitrates = [2][15]int{
	{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320},
	{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160},
}

type header struct {
	size, rateIndex      int
	channels, granules   int
	sideStart, sideBytes int
	stereo               int // joint stereo bits: intensity (1) and mid/side (2)
}

func parseHeader(word uint32) (header, bool) {
	version := (word >> 19) & 3
	rate := int((word >> 10) & 3)
	bitrate := (word >> 12) & 15
	if word>>21 != 0x7ff || (word>>17)&3 != 1 || version == 1 || rate == 3 || bitrate == 0 || bitrate == 15 || word&3 == 2 {
		return header{}, false
	}
	h := header{rateIndex: rate, channels: 2, granules: 2, sideStart: 4}
	mode := (word >> 6) & 3
	if mode == 3 {
		h.channels = 1
	}
	if mode == 1 {
		h.stereo = int((word >> 4) & 3)
	}
	group := 0
	if version == 3 {
		h.sideBytes = 17 + 15*(h.channels-1)
	} else {
		group, h.granules = 1, 1
		h.rateIndex += 3
		if version == 0 {
			h.rateIndex += 3
		}
		h.sideBytes = 9 + 8*(h.channels-1)
	}
	if (word>>16)&1 == 0 {
		h.sideStart += 2
	}
	h.size = 72000*h.granules*bitrates[group][bitrate]/rates[h.rateIndex] + int((word>>9)&1)
	return h, h.size >= h.sideStart+h.sideBytes
}

type frame struct {
	side, data []byte
	header
}

type scanner struct {
	input   *bufio.Reader
	offset  int64
	pending int
}

func newScanner(r io.Reader) scanner { return scanner{input: bufio.NewReader(r)} }

func (s *scanner) discard(count int) error {
	n, err := s.input.Discard(count)
	s.offset += int64(n)
	if err == io.EOF {
		return io.ErrUnexpectedEOF
	}
	return err
}

// next lends the frame bytes until the next call. ID3 artwork can contain
// sync-like byte patterns, so consume the whole tag before looking for audio.
func (s *scanner) next() (frame, error) {
	if s.pending != 0 {
		if err := s.discard(s.pending); err != nil {
			return frame{}, err
		}
		s.pending = 0
	}
	for {
		prefix, err := s.input.Peek(4)
		if err != nil {
			return frame{}, err
		}
		switch string(prefix[:3]) {
		case "ID3":
			tag, err := s.input.Peek(10)
			if err != nil {
				if err == io.EOF {
					err = io.ErrUnexpectedEOF
				}
				return frame{}, err
			}
			if (tag[6]|tag[7]|tag[8]|tag[9])&0x80 != 0 {
				return frame{}, errors.New("mp3: invalid ID3 tag size")
			}
			size := int(tag[6])<<21 | int(tag[7])<<14 | int(tag[8])<<7 | int(tag[9])
			size += 10
			if tag[3] == 4 && tag[5]&0x10 != 0 {
				size += 10
			}
			if err := s.discard(size); err != nil {
				return frame{}, err
			}
			continue
		case "TAG":
			if err := s.discard(128); err != nil {
				return frame{}, err
			}
			continue
		}
		h, valid := parseHeader(binary.BigEndian.Uint32(prefix))
		if !valid {
			if err := s.discard(1); err != nil {
				return frame{}, err
			}
			continue
		}
		packet, err := s.input.Peek(h.size)
		if err != nil {
			// Only complete audio frames contribute to the track duration.
			return frame{}, err
		}
		s.pending = h.size
		end := h.sideStart + h.sideBytes
		return frame{header: h, side: packet[h.sideStart:end], data: packet[end:]}, nil
	}
}

// bits is limited to one side-info or part2_3 field, including when its end
// falls in the middle of a byte shared with the following channel.
type bits struct {
	data     []byte
	pos, end int
	failed   bool
}

func (b *bits) read(n int) int {
	if b.pos+n > b.end {
		b.failed = true
		return 0
	}
	value := 0
	for n > 0 {
		remaining := 8 - b.pos%8
		take := min(n, remaining)
		value = value<<take | int(b.data[b.pos/8]>>(remaining-take))&((1<<take)-1)
		b.pos += take
		n -= take
	}
	return value
}

func (b *bits) bit() int {
	if b.pos >= b.end {
		b.failed = true
		return 0
	}
	pos := b.pos
	b.pos++
	return int(b.data[pos/8]>>(7-pos%8)) & 1
}

type granule struct {
	length, pairs, gain, compress int
	blockType, region0, region1   int
	book, subgain                 [3]int
	countBook, scaleShift         int
	mixed, preflag                bool
}

type sideInfo struct {
	channel [2][2]granule
	reuse   [2]int // MPEG-1 scalefactor selection information (scfsi)
	back    int    // main_data_begin, in bytes before this packet's main data
}

func readSide(f frame) (sideInfo, error) {
	b := bits{data: f.side, end: len(f.side) * 8}
	var side sideInfo
	if f.granules == 2 {
		side.back = b.read(9)
		b.read(7 - 2*f.channels)
		for ch := range f.channels {
			side.reuse[ch] = b.read(4)
		}
	} else {
		side.back = b.read(8)
		b.read(f.channels)
	}
	for gr := range f.granules {
		for ch := range f.channels {
			g := &side.channel[gr][ch]
			g.length, g.pairs, g.gain = b.read(12), b.read(9), b.read(8)
			if g.pairs > 288 {
				return side, errFrame
			}
			if f.granules == 2 {
				g.compress = b.read(4)
			} else {
				g.compress = b.read(9)
			}
			if b.bit() != 0 {
				g.blockType, g.mixed = b.read(2), b.bit() != 0
				if g.blockType == 0 {
					return side, errFrame
				}
				g.book[0], g.book[1] = b.read(5), b.read(5)
				for w := range g.subgain {
					g.subgain[w] = b.read(3)
				}
				g.region0, g.region1 = 36, 576
				if f.granules == 1 && g.blockType != 2 {
					g.region0 = 54
				}
				if rates[f.rateIndex] == 8000 {
					g.region0 *= 2
				}
			} else {
				for i := range g.book {
					g.book[i] = b.read(5)
				}
				r0, r1 := b.read(4)+1, b.read(3)+1
				layout := &layouts[f.rateIndex][longBlock]
				g.region0 = int(layout.band[r0].start)
				g.region1 = granuleSamples
				if r0+r1 < layout.count {
					g.region1 = int(layout.band[r0+r1].start)
				}
			}
			if f.granules == 2 {
				g.preflag = b.bit() != 0
			} else {
				g.preflag = g.compress >= 500 && (ch == 0 || f.stereo&1 == 0)
			}
			g.scaleShift = 1 + b.bit()
			g.countBook = 32 + b.bit()
		}
	}
	if b.failed {
		return side, errFrame
	}
	return side, nil
}
