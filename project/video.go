package project

import (
	"errors"
	"io"
	"sync"

	"github.com/diiyw/onix/resource"
)

// Video owns a pinned encoded video blob. Bytes returns an immutable borrowed
// view; ReadAll returns an independent mutable copy.
type Video struct {
	mu           sync.Mutex
	blob         *resource.Blob
	readers      int
	closed       bool
	closePending bool
}

func newVideo(blob *resource.Blob) *Video {
	return &Video{blob: blob}
}

func (v *Video) Info() resource.AssetInfo {
	if v == nil {
		return resource.AssetInfo{}
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed || v.blob == nil {
		return resource.AssetInfo{}
	}
	return v.blob.Info()
}

func (v *Video) Bytes() ([]byte, error) {
	if v == nil {
		return nil, io.ErrClosedPipe
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed || v.blob == nil {
		return nil, io.ErrClosedPipe
	}
	return v.blob.Bytes()
}

func (v *Video) ReadAll() ([]byte, error) {
	if v == nil {
		return nil, io.ErrClosedPipe
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed || v.blob == nil {
		return nil, io.ErrClosedPipe
	}
	return v.blob.ReadAll()
}

func (v *Video) Open() (io.ReadSeekCloser, error) {
	if v == nil {
		return nil, io.ErrClosedPipe
	}
	v.mu.Lock()
	if v.closed || v.blob == nil {
		v.mu.Unlock()
		return nil, io.ErrClosedPipe
	}
	reader, err := v.blob.Open()
	if err != nil {
		v.mu.Unlock()
		return nil, err
	}
	v.readers++
	v.mu.Unlock()
	return &videoReader{ReadSeekCloser: reader, video: v}, nil
}

func (v *Video) Close() error {
	if v == nil {
		return nil
	}
	v.mu.Lock()
	if v.closed {
		v.mu.Unlock()
		return nil
	}
	v.closed = true
	if v.readers != 0 {
		v.closePending = true
		v.mu.Unlock()
		return nil
	}
	blob := v.blob
	v.blob = nil
	v.mu.Unlock()
	if blob == nil {
		return nil
	}
	return blob.Close()
}

func (v *Video) releaseReader() error {
	v.mu.Lock()
	if v.readers > 0 {
		v.readers--
	}
	if v.readers != 0 || !v.closePending || v.blob == nil {
		v.mu.Unlock()
		return nil
	}
	blob := v.blob
	v.blob = nil
	v.closePending = false
	v.mu.Unlock()
	return blob.Close()
}

type videoReader struct {
	io.ReadSeekCloser
	video *Video
	once  sync.Once
	err   error
}

func (r *videoReader) Close() error {
	if r == nil {
		return nil
	}
	r.once.Do(func() {
		var readerErr, videoErr error
		if r.ReadSeekCloser != nil {
			readerErr = r.ReadSeekCloser.Close()
		}
		if r.video != nil {
			videoErr = r.video.releaseReader()
		}
		r.ReadSeekCloser = nil
		r.video = nil
		r.err = errors.Join(readerErr, videoErr)
	})
	return r.err
}
