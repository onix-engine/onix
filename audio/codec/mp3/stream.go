package mp3

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/diiyw/onix/audio"
	"github.com/hajimehoshi/go-mp3"
)

type pcm16StereoStream struct {
	source       io.ReadSeeker
	sourceRate   int64
	sourceFrames int64
	outputFrames int64
	position     int64
	cachedFrame  int64
	left         [2]float32
	right        [2]float32
	output       [audio.BytesPerFrame]byte
	outputFrame  int64
	input        [4096]byte
	inputOffset  int
	inputLength  int
}

func inspectStream(source audio.Source) (int, int64, error) {
	reader, err := source.Open()
	if err != nil {
		return 0, 0, err
	}
	defer reader.Close()
	decoder, err := mp3.NewDecoder(reader)
	if err != nil {
		return 0, 0, err
	}
	length := decoder.Length()
	if length <= 0 || length%4 != 0 {
		return 0, 0, fmt.Errorf("invalid stereo PCM byte count %d", length)
	}
	return decoder.SampleRate(), length / 4, nil
}

func newStreamReader(encoded audio.Source, sourceRate int, sourceFrames int64) (*pcm16StereoStream, error) {
	reader, err := encoded.Open()
	if err != nil {
		return nil, err
	}
	source, err := newMP3PCMSource(reader, sourceFrames*4)
	if err != nil {
		_ = reader.Close()
		return nil, err
	}
	return newPCM16StereoStream(source, sourceRate, sourceFrames), nil
}

func newPCM16StereoStream(source io.ReadSeeker, sourceRate int, sourceFrames int64) *pcm16StereoStream {
	return &pcm16StereoStream{
		source:       source,
		sourceRate:   int64(sourceRate),
		sourceFrames: sourceFrames,
		outputFrames: audio.ResampledFrameCount(sourceFrames, sourceRate),
		cachedFrame:  -1,
		outputFrame:  -1,
	}
}

type mp3PCMSource struct {
	encoded  io.ReadSeekCloser
	decoder  *mp3.Decoder
	position int64
	length   int64
}

func newMP3PCMSource(encoded io.ReadSeekCloser, length int64) (*mp3PCMSource, error) {
	source := &mp3PCMSource{encoded: encoded, length: length}
	if err := source.rewind(); err != nil {
		return nil, err
	}
	return source, nil
}

func (s *mp3PCMSource) Read(target []byte) (int, error) {
	count, err := s.decoder.Read(target)
	s.position += int64(count)
	return count, err
}

func (s *mp3PCMSource) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += s.position
	case io.SeekEnd:
		offset += s.length
	default:
		return 0, fmt.Errorf("audio: invalid MP3 seek mode %d", whence)
	}
	if offset < 0 {
		return 0, fmt.Errorf("audio: negative MP3 position %d", offset)
	}
	offset = min(offset, s.length)
	if offset < s.position {
		if err := s.rewind(); err != nil {
			return 0, err
		}
	}
	if offset > s.position {
		if _, err := io.CopyN(io.Discard, s.decoder, offset-s.position); err != nil {
			return 0, err
		}
		s.position = offset
	}
	return s.position, nil
}

func (s *mp3PCMSource) rewind() error {
	if _, err := s.encoded.Seek(0, io.SeekStart); err != nil {
		return err
	}
	decoder, err := mp3.NewDecoder(s.encoded)
	if err != nil {
		return err
	}
	s.decoder = decoder
	s.position = 0
	return nil
}

func (s *mp3PCMSource) Close() error {
	if s == nil || s.encoded == nil {
		return nil
	}
	encoded := s.encoded
	s.encoded = nil
	s.decoder = nil
	return encoded.Close()
}

func (s *pcm16StereoStream) Read(target []byte) (int, error) {
	length := s.outputFrames * audio.BytesPerFrame
	if s.position >= length {
		return 0, io.EOF
	}
	written := 0
	for len(target) > 0 && s.position < length {
		frame := s.position / audio.BytesPerFrame
		if s.outputFrame != frame {
			if err := s.render(frame); err != nil {
				return written, err
			}
			s.outputFrame = frame
		}
		offset := s.position % audio.BytesPerFrame
		count := copy(target, s.output[offset:])
		target = target[count:]
		s.position += int64(count)
		written += count
	}
	return written, nil
}

func (s *pcm16StereoStream) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += s.position
	case io.SeekEnd:
		offset += s.outputFrames * audio.BytesPerFrame
	default:
		return 0, fmt.Errorf("audio: invalid stream seek mode %d", whence)
	}
	if offset < 0 {
		return 0, fmt.Errorf("audio: negative stream position %d", offset)
	}
	s.position = min(offset, s.outputFrames*audio.BytesPerFrame)
	s.cachedFrame = -1
	s.outputFrame = -1
	return s.position, nil
}

func (s *pcm16StereoStream) Close() error {
	if s == nil || s.source == nil {
		return nil
	}
	if closer, ok := s.source.(io.Closer); ok {
		err := closer.Close()
		s.source = nil
		return err
	}
	s.source = nil
	return nil
}

func (s *pcm16StereoStream) render(frame int64) error {
	numerator := frame * s.sourceRate
	leftFrame := numerator / audio.OutputSampleRate
	if err := s.ensureSourceFrame(leftFrame); err != nil {
		return err
	}
	fraction := float32(numerator%audio.OutputSampleRate) / audio.OutputSampleRate
	for channel := range audio.OutputChannels {
		sample := s.left[channel] + (s.right[channel]-s.left[channel])*fraction
		binary.LittleEndian.PutUint32(s.output[channel*audio.BytesPerSample:], math.Float32bits(sample))
	}
	return nil
}

func (s *pcm16StereoStream) ensureSourceFrame(frame int64) error {
	if s.cachedFrame < 0 || frame < s.cachedFrame || frame > s.cachedFrame+1 {
		if _, err := s.source.Seek(frame*4, io.SeekStart); err != nil {
			return err
		}
		s.inputOffset, s.inputLength = 0, 0
		left, err := s.readSourceFrame()
		if err != nil {
			return err
		}
		s.left, s.right, s.cachedFrame = left, left, frame
		if frame+1 < s.sourceFrames {
			s.right, err = s.readSourceFrame()
			if err != nil {
				return err
			}
		}
		return nil
	}
	if frame == s.cachedFrame+1 {
		s.left = s.right
		s.cachedFrame = frame
		if frame+1 < s.sourceFrames {
			right, err := s.readSourceFrame()
			if err != nil {
				return err
			}
			s.right = right
		} else {
			s.right = s.left
		}
	}
	return nil
}

func (s *pcm16StereoStream) readSourceFrame() ([2]float32, error) {
	for s.inputLength-s.inputOffset < 4 {
		remaining := copy(s.input[:], s.input[s.inputOffset:s.inputLength])
		count, err := s.source.Read(s.input[remaining:])
		s.inputOffset = 0
		s.inputLength = remaining + count
		if s.inputLength < 4 {
			if err != nil {
				return [2]float32{}, err
			}
			if count == 0 {
				return [2]float32{}, io.ErrNoProgress
			}
		}
	}
	data := s.input[s.inputOffset : s.inputOffset+4]
	s.inputOffset += 4
	return [2]float32{
		audio.NormalizeSigned(int16(binary.LittleEndian.Uint16(data[0:2])), 16),
		audio.NormalizeSigned(int16(binary.LittleEndian.Uint16(data[2:4])), 16),
	}, nil
}
