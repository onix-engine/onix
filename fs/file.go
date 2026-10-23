package fs

import (
	"bytes"
	iofs "io/fs"
	"time"
)

type fileInfo struct {
	name    string
	size    int64
	mode    iofs.FileMode
	modTime time.Time
}

func (i fileInfo) Name() string        { return i.name }
func (i fileInfo) Size() int64         { return i.size }
func (i fileInfo) Mode() iofs.FileMode { return i.mode }
func (i fileInfo) ModTime() time.Time  { return i.modTime }
func (i fileInfo) IsDir() bool         { return i.mode.IsDir() }
func (i fileInfo) Sys() any            { return nil }

type dirEntry struct {
	info iofs.FileInfo
}

func (e dirEntry) Name() string                 { return e.info.Name() }
func (e dirEntry) IsDir() bool                  { return e.info.IsDir() }
func (e dirEntry) Type() iofs.FileMode          { return e.info.Mode().Type() }
func (e dirEntry) Info() (iofs.FileInfo, error) { return e.info, nil }

type memoryFile struct {
	reader *bytes.Reader
	info   iofs.FileInfo
	closed bool
}

func newMemoryFile(data []byte, info iofs.FileInfo) *memoryFile {
	return &memoryFile{reader: bytes.NewReader(data), info: info}
}

func (f *memoryFile) Stat() (iofs.FileInfo, error) {
	if f.closed {
		return nil, ErrClosed
	}
	return f.info, nil
}

func (f *memoryFile) Read(data []byte) (int, error) {
	if f.closed {
		return 0, ErrClosed
	}
	if f.info.IsDir() {
		return 0, ErrInvalid
	}
	return f.reader.Read(data)
}

func (f *memoryFile) Close() error {
	f.closed = true
	return nil
}
