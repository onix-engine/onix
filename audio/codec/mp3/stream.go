package mp3

import (
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"math"

	"github.com/onix-engine/onix/audio"
	"github.com/onix-engine/onix/audio/codec/mp3/internal/layer3"
)

// streamReader exposes resampled stereo float32 PCM as little-endian bytes.
type streamReader struct {
	pcm                      *layer3.Reader
	rate                     int
	frames, length, position int64
	sourceFrame, outputFrame int64
	left, right              [2]float32
	output                   [audio.BytesPerFrame]byte
}

func newStreamReader(track *layer3.Track, encoded io.ReadSeekCloser) *streamReader {
	return &streamReader{
		pcm:         track.Open(encoded),
		rate:        track.SampleRate(),
		frames:      track.Frames(),
		length:      audio.ResampledFrameCount(track.Frames(), track.SampleRate()) * audio.BytesPerFrame,
		sourceFrame: -1,
		outputFrame: -1,
	}
}

func (s *streamReader) Read(dst []byte) (int, error) {
	if s.pcm.Closed() {
		return 0, fs.ErrClosed
	}
	if len(dst) == 0 {
		return 0, nil
	}
	if s.position >= s.length {
		return 0, io.EOF
	}
	written := 0
	for written < len(dst) && s.position < s.length {
		frame := s.position / audio.BytesPerFrame
		if s.outputFrame != frame {
			if err := s.render(frame); err != nil {
				return written, err
			}
			s.outputFrame = frame
		}
		n := copy(dst[written:], s.output[s.position%audio.BytesPerFrame:])
		s.position += int64(n)
		written += n
	}
	return written, nil
}

func (s *streamReader) Seek(offset int64, whence int) (int64, error) {
	if s.pcm.Closed() {
		return 0, fs.ErrClosed
	}
	var base int64
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		base = s.position
	case io.SeekEnd:
		base = s.length
	default:
		return 0, fmt.Errorf("audio: invalid MP3 seek mode %d", whence)
	}
	if offset < -base {
		return 0, fmt.Errorf("audio: negative MP3 position %d", base+offset)
	}
	s.position = base + min(offset, s.length-base)
	return s.position, nil
}

func (s *streamReader) render(frame int64) error {
	position := float64(frame) * float64(s.rate) / audio.OutputSampleRate
	leftFrame := min(int64(position), s.frames-1)
	if leftFrame != s.sourceFrame {
		left := s.right
		if s.sourceFrame < 0 || leftFrame != s.sourceFrame+1 {
			var err error
			left, err = s.pcm.StereoAt(leftFrame)
			if err != nil {
				return err
			}
		}
		right := left
		if leftFrame+1 < s.frames {
			var err error
			right, err = s.pcm.StereoAt(leftFrame + 1)
			if err != nil {
				return err
			}
		}
		s.left, s.right, s.sourceFrame = left, right, leftFrame
	}
	fraction := float32(position - float64(leftFrame))
	for ch := range audio.OutputChannels {
		v := s.left[ch] + (s.right[ch]-s.left[ch])*fraction
		binary.LittleEndian.PutUint32(s.output[ch*audio.BytesPerSample:], math.Float32bits(v))
	}
	return nil
}

func (s *streamReader) Close() error {
	return s.pcm.Close()
}
