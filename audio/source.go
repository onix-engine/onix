package audio

import (
	"bytes"
	"io"
	iofs "io/fs"
	"sync"
)

// Source is an immutable encoded audio resource. DecodeAudio takes ownership
// of the source whether decoding succeeds or fails.
type Source interface {
	Bytes() ([]byte, error)
	Open() (io.ReadSeekCloser, error)
	Size() int64
	Close() error
}

// MemorySource owns one immutable encoded audio byte slice.
type MemorySource struct {
	mu   sync.RWMutex
	data []byte
}

// NewMemorySource takes ownership of data. It must not be mutated afterward.
func NewMemorySource(data []byte) *MemorySource {
	return &MemorySource{data: data}
}

func (s *MemorySource) Bytes() ([]byte, error) {
	if s == nil {
		return nil, iofs.ErrClosed
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return nil, iofs.ErrClosed
	}
	return s.data, nil
}

func (s *MemorySource) Open() (io.ReadSeekCloser, error) {
	if s == nil {
		return nil, iofs.ErrClosed
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return nil, iofs.ErrClosed
	}
	return readSeekNopCloser{ReadSeeker: bytes.NewReader(s.data)}, nil
}

func (s *MemorySource) Size() int64 {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	size := int64(len(s.data))
	s.mu.RUnlock()
	return size
}

func (s *MemorySource) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	s.data = nil
	s.mu.Unlock()
	return nil
}

type readSeekNopCloser struct{ io.ReadSeeker }

func (readSeekNopCloser) Close() error { return nil }
