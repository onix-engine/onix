package audio

import (
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	OutputSampleRate = 48000
	OutputChannels   = 2
	BytesPerSample   = 4
	BytesPerFrame    = OutputChannels * BytesPerSample
	BytesPerSecond   = OutputSampleRate * BytesPerFrame

	outputSampleRate = OutputSampleRate
	outputChannels   = OutputChannels
	bytesPerSample   = BytesPerSample
	bytesPerFrame    = BytesPerFrame
	bytesPerSecond   = BytesPerSecond
)

type Format string

const (
	FormatFLAC Format = "flac"
	FormatMP3  Format = "mp3"
	FormatOGG  Format = "ogg"
	FormatWAV  Format = "wav"
)

// Decoder decodes one statically selected collection of audio formats.
// Implementations are passed to project resource features as type arguments.
type Decoder interface {
	DecodeAudio(Source, Format, bool) (*Clip, error)
}

// FormatDecoder handles one audio format in DecoderSet.
type FormatDecoder interface {
	SupportsAudioFormat(Format) bool
	DecodeAudio(Source, Format, bool) (*Clip, error)
}

// NoDecoder fills an unused DecoderSet slot.
type NoDecoder struct{}

func (NoDecoder) SupportsAudioFormat(Format) bool { return false }

func (NoDecoder) DecodeAudio(source Source, format Format, _ bool) (*Clip, error) {
	return nil, errors.Join(fmt.Errorf("audio: unsupported format %q", format), closeSource(source))
}

// DecoderSet is a compile-time collection of audio format decoders. It does
// not register formats or allocate while choosing a decoder.
type DecoderSet[A, B, C, D FormatDecoder] struct{}

func (DecoderSet[A, B, C, D]) DecodeAudio(source Source, format Format, stream bool) (*Clip, error) {
	if source == nil {
		return nil, fmt.Errorf("audio: source is nil")
	}
	if clip, matched, err := decodeAudioFormat[A](source, format, stream); matched {
		return clip, err
	}
	if clip, matched, err := decodeAudioFormat[B](source, format, stream); matched {
		return clip, err
	}
	if clip, matched, err := decodeAudioFormat[C](source, format, stream); matched {
		return clip, err
	}
	if clip, matched, err := decodeAudioFormat[D](source, format, stream); matched {
		return clip, err
	}
	return nil, errors.Join(fmt.Errorf("audio: unsupported format %q", format), source.Close())
}

func decodeAudioFormat[D FormatDecoder](source Source, format Format, stream bool) (*Clip, bool, error) {
	var decoder D
	if !decoder.SupportsAudioFormat(format) {
		return nil, false, nil
	}
	clip, err := decoder.DecodeAudio(source, format, stream)
	return clip, true, err
}

func FormatFromPath(path string) (Format, error) {
	format := Format(strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."))
	switch format {
	case FormatFLAC, FormatMP3, FormatOGG, FormatWAV:
		return format, nil
	default:
		return "", fmt.Errorf("audio: unsupported format %q", filepath.Ext(path))
	}
}

type Clip struct {
	mu           sync.Mutex
	format       Format
	pcm          []byte
	samples      []float32
	length       int64
	duration     time.Duration
	streamSource string
	source       Source
	openStream   func(Source) (io.ReadSeekCloser, error)
	readers      int
	closed       bool
	closePending bool
}

func (c *Clip) Format() Format {
	if c == nil {
		return ""
	}
	return c.format
}

func (c *Clip) Duration() time.Duration {
	if c == nil {
		return 0
	}
	return c.duration
}

func (c *Clip) Streaming() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	streaming := !c.closed && c.openStream != nil
	c.mu.Unlock()
	return streaming
}

// DecodedAudio is the PCM data supplied by an audio format decoder.
type DecodedAudio struct {
	Samples    []float32
	SampleRate int
	Channels   int
}

// NewClip prepares decoded samples at the engine's fixed stereo sample rate.
func NewClip(format Format, decoded DecodedAudio) (*Clip, error) {
	samples, owned, err := prepareSamples(decoded)
	if err != nil {
		return nil, fmt.Errorf("audio: decode %s: %w", format, err)
	}
	pcm, webSamples := staticClipData(samples, owned)
	return &Clip{
		format:   format,
		pcm:      pcm,
		samples:  webSamples,
		length:   int64(len(samples) * BytesPerSample),
		duration: framesDuration(len(samples) / OutputChannels),
	}, nil
}

// DecodeClip decodes a static source and releases it before returning.
func DecodeClip(format Format, source Source, decode func([]byte) (DecodedAudio, error)) (*Clip, error) {
	if source == nil || decode == nil {
		return nil, errors.Join(fmt.Errorf("audio: invalid %s source decoder", format), closeSource(source))
	}
	data, err := source.Bytes()
	if err != nil {
		return nil, errors.Join(err, source.Close())
	}
	decoded, decodeErr := decode(data)
	closeErr := source.Close()
	if decodeErr != nil || closeErr != nil {
		return nil, errors.Join(decodeErr, closeErr)
	}
	return NewClip(format, decoded)
}

// NewStreamingClip creates a clip whose format implementation supplies its
// stream reader. The callback is installed once while loading the resource
// and is not a runtime registry.
func NewStreamingClip(format Format, source Source, sampleRate int, sourceFrames int64, open func(Source) (io.ReadSeekCloser, error)) (*Clip, error) {
	if source == nil || source.Size() <= 0 || sampleRate <= 0 || sourceFrames <= 0 || open == nil {
		return nil, errors.Join(fmt.Errorf("audio: invalid streaming %s clip", format), closeSource(source))
	}
	outputFrames := ResampledFrameCount(sourceFrames, sampleRate)
	return &Clip{
		format:     format,
		length:     outputFrames * BytesPerFrame,
		duration:   framesDuration(outputFrames),
		source:     source,
		openStream: open,
	}, nil
}

func (c *Clip) openStreamingReader() (io.ReadSeekCloser, error) {
	if c == nil {
		return nil, fmt.Errorf("audio: stream reader is unavailable")
	}
	c.mu.Lock()
	if c.closed || c.source == nil || c.openStream == nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("audio: stream reader is unavailable")
	}
	source, open := c.source, c.openStream
	c.readers++
	c.mu.Unlock()
	reader, err := open(source)
	if err != nil {
		_ = c.releaseSource()
		return nil, err
	}
	return &clipStreamReader{ReadSeekCloser: reader, clip: c}, nil
}

func (c *Clip) borrowEncoded() ([]byte, func() error, error) {
	if c == nil {
		return nil, nil, fmt.Errorf("audio: encoded source is unavailable")
	}
	c.mu.Lock()
	if c.closed || c.source == nil {
		c.mu.Unlock()
		return nil, nil, fmt.Errorf("audio: encoded source is unavailable")
	}
	source := c.source
	c.readers++
	c.mu.Unlock()
	data, err := source.Bytes()
	if err != nil {
		_ = c.releaseSource()
		return nil, nil, err
	}
	return data, c.releaseSource, nil
}

func (c *Clip) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.pcm = nil
	c.samples = nil
	c.openStream = nil
	if c.readers != 0 {
		c.closePending = true
		c.mu.Unlock()
		return nil
	}
	source := c.source
	c.source = nil
	c.mu.Unlock()
	return closeSource(source)
}

func (c *Clip) releaseSource() error {
	c.mu.Lock()
	if c.readers > 0 {
		c.readers--
	}
	if c.readers != 0 || !c.closePending || c.source == nil {
		c.mu.Unlock()
		return nil
	}
	source := c.source
	c.source = nil
	c.closePending = false
	c.mu.Unlock()
	return source.Close()
}

type clipStreamReader struct {
	io.ReadSeekCloser
	clip *Clip
	once sync.Once
	err  error
}

func (r *clipStreamReader) Close() error {
	if r == nil {
		return nil
	}
	r.once.Do(func() {
		var readerErr, sourceErr error
		if r.ReadSeekCloser != nil {
			readerErr = r.ReadSeekCloser.Close()
		}
		if r.clip != nil {
			sourceErr = r.clip.releaseSource()
		}
		r.ReadSeekCloser = nil
		r.clip = nil
		r.err = errors.Join(readerErr, sourceErr)
	})
	return r.err
}

func closeSource(source Source) error {
	if source == nil {
		return nil
	}
	return source.Close()
}

func prepareSamples(decoded DecodedAudio) ([]float32, bool, error) {
	if decoded.SampleRate <= 0 {
		return nil, false, fmt.Errorf("invalid sample rate %d", decoded.SampleRate)
	}
	if decoded.Channels < 1 || decoded.Channels > OutputChannels {
		return nil, false, fmt.Errorf("unsupported channel count %d", decoded.Channels)
	}
	if len(decoded.Samples) == 0 || len(decoded.Samples)%decoded.Channels != 0 {
		return nil, false, fmt.Errorf("invalid sample count %d for %d channels", len(decoded.Samples), decoded.Channels)
	}

	if decoded.SampleRate == outputSampleRate {
		if decoded.Channels == OutputChannels {
			return decoded.Samples, false, nil
		}
		stereo := make([]float32, len(decoded.Samples)*OutputChannels)
		for index, sample := range decoded.Samples {
			stereo[index*OutputChannels], stereo[index*OutputChannels+1] = sample, sample
		}
		return stereo, true, nil
	}
	return resampleSamples(decoded.Samples, decoded.SampleRate, decoded.Channels), true, nil
}

func resampleSamples(samples []float32, sourceRate, channels int) []float32 {
	sourceFrames := len(samples) / channels
	targetFrames := max(1, int(math.Round(float64(sourceFrames)*outputSampleRate/float64(sourceRate))))
	target := make([]float32, targetFrames*outputChannels)
	for frame := range targetFrames {
		position := float64(frame) * float64(sourceRate) / outputSampleRate
		left := min(int(position), sourceFrames-1)
		right := min(left+1, sourceFrames-1)
		fraction := float32(position - float64(left))
		for channel := range outputChannels {
			index := min(channel, channels-1)
			from := samples[left*channels+index]
			to := samples[right*channels+index]
			target[frame*outputChannels+channel] = from + (to-from)*fraction
		}
	}
	return target
}

func NormalizeSigned[T ~int | ~int16 | ~int32](sample T, bits int) float32 {
	return float32(float64(sample) / math.Ldexp(1, bits-1))
}

func ResampledFrameCount(sourceFrames int64, sourceRate int) int64 {
	if sourceRate <= 0 {
		return 0
	}
	return max(int64(1), int64(math.Round(float64(sourceFrames)*OutputSampleRate/float64(sourceRate))))
}

func framesDuration[T ~int | ~int64](frames T) time.Duration {
	return time.Duration(frames) * time.Second / outputSampleRate
}
