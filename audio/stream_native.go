//go:build !js

package audio

import (
	"encoding/binary"
	"io"
	"math"
	"sync"
)

// The producer reserves one ring slot, releases mu for I/O, then publishes
// the completed block. Consuming earlier slots never moves the reserved tail.
type nativeStream struct {
	reader                            *clipStreamReader
	changed                           sync.Cond
	blocks                            [4]streamBlock
	remaining                         int64
	head, queued, offset              int
	mu                                sync.Mutex
	running, reading, seeking, closed bool
}

type streamBlock struct {
	err   error
	count int
	data  [outputBlockFrames * bytesPerFrame]byte
}

func newNativeStream(reader *clipStreamReader) *nativeStream {
	s := &nativeStream{reader: reader}
	s.changed.L = &s.mu
	return s
}

func (s *nativeStream) start(frames int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running && s.queued == 0 && frames > 0 {
		s.remaining = frames
		s.running = true
		go s.decode()
	}
}

func (s *nativeStream) decode() {
	s.mu.Lock()
	// Keep the producer available for seeks while its final block is queued.
	// Once the mixer consumes the tail, the goroutine can exit.
	for !s.closed && (s.remaining > 0 || s.queued > 0) {
		if s.seeking || s.remaining == 0 || s.queued == len(s.blocks) {
			s.changed.Wait()
			continue
		}
		block := &s.blocks[(s.head+s.queued)%len(s.blocks)]
		frames := min(s.remaining, outputBlockFrames)
		s.remaining -= frames
		s.reading = true
		s.mu.Unlock()
		n, err := io.ReadFull(s.reader, block.data[:frames*bytesPerFrame])
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		s.mu.Lock()
		block.count, block.err = n/bytesPerFrame*outputChannels, err
		s.queued++
		s.reading = false
		if err != nil {
			s.remaining = 0
		}
		if s.seeking || s.remaining == 0 {
			s.changed.Broadcast()
		}
	}
	s.running = false
	s.changed.Broadcast()
	s.mu.Unlock()
}

// The player is detached from the mixer throughout the seek. A live producer
// resumes with the same ring as soon as the new cursor is established.
func (s *nativeStream) seek(frame, total int64, playing bool) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seeking = true
	for s.reading {
		s.changed.Wait()
	}
	offset, err := s.reader.Seek(frame*bytesPerFrame, io.SeekStart)
	position := offset / bytesPerFrame
	s.head, s.queued, s.offset = 0, 0, 0
	s.remaining = 0
	if playing && err == nil {
		s.remaining = total - position
	}
	s.seeking = false
	s.changed.Broadcast()
	return position, err
}

func (s *nativeStream) close() error {
	s.reader.interrupt()
	s.mu.Lock()
	s.closed = true
	s.changed.Broadcast()
	for s.running {
		s.changed.Wait()
	}
	s.mu.Unlock()
	return s.reader.Close()
}

func (s *nativeStream) mix(out []float32, gain float32) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	written := 0
	for written < len(out) && s.queued != 0 {
		block := &s.blocks[s.head]
		n := min(block.count-s.offset, len(out)-written)
		for i := range n {
			bits := binary.LittleEndian.Uint32(block.data[(s.offset+i)*bytesPerSample:])
			out[written+i] += gain * math.Float32frombits(bits)
		}
		written += n
		s.offset += n
		if s.offset == block.count {
			s.head = (s.head + 1) % len(s.blocks)
			s.queued--
			s.offset = 0
			s.changed.Signal()
			if block.err != nil {
				return written, block.err
			}
		}
	}
	return written, nil
}
