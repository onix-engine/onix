package fs

import (
	"errors"
	iofs "io/fs"
	"strings"
	"sync/atomic"
)

var (
	ErrInvalid  = iofs.ErrInvalid
	ErrNotExist = iofs.ErrNotExist
	ErrExist    = iofs.ErrExist
	ErrClosed   = iofs.ErrClosed
	ErrNotEmpty = errors.New("directory not empty")
)

// FileSystem exposes the same rooted, slash-separated virtual paths on
// every platform. Browser builds persist entries in IndexedDB.
type FileSystem struct {
	backend backend
}

type backend interface {
	Open(name string) (iofs.File, error)
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte) error
	MkdirAll(name string) error
	ReadDir(name string) ([]iofs.DirEntry, error)
	Stat(name string) (iofs.FileInfo, error)
	Remove(name string) error
	RemoveAll(name string) error
	Rename(oldName, newName string) error
}

var defaultFileSystem atomic.Pointer[FileSystem]

func init() {
	root, err := platformDefaultRoot()
	if err != nil {
		panic(err)
	}
	fileSystem, err := New(root)
	if err != nil {
		panic(err)
	}
	defaultFileSystem.Store(fileSystem)
}

// New creates an isolated filesystem rooted at root. Names passed to its
// methods are always virtual io/fs paths, never host absolute paths.
func New(root string) (*FileSystem, error) {
	storage, err := newPlatformBackend(root)
	if err != nil {
		return nil, err
	}
	return &FileSystem{backend: storage}, nil
}

// Default returns the process-wide filesystem used by package-level helpers.
func Default() *FileSystem {
	return defaultFileSystem.Load()
}

// Mount replaces the process-wide filesystem with one rooted at root.
func Mount(root string) error {
	fileSystem, err := New(root)
	if err != nil {
		return err
	}
	defaultFileSystem.Store(fileSystem)
	return nil
}

func (f *FileSystem) Open(name string) (iofs.File, error) {
	return withPath(f, "open", name, func(name string) (iofs.File, error) {
		return f.backend.Open(name)
	})
}

func (f *FileSystem) ReadFile(name string) ([]byte, error) {
	return withPath(f, "read", name, func(name string) ([]byte, error) {
		return f.backend.ReadFile(name)
	})
}

func (f *FileSystem) WriteFile(name string, data []byte) error {
	if name == "." {
		return newPathError("write", name, ErrInvalid)
	}
	_, err := withPath(f, "write", name, func(name string) (struct{}, error) {
		return struct{}{}, f.backend.WriteFile(name, data)
	})
	return err
}

func (f *FileSystem) MkdirAll(name string) error {
	_, err := withPath(f, "mkdir", name, func(name string) (struct{}, error) {
		return struct{}{}, f.backend.MkdirAll(name)
	})
	return err
}

func (f *FileSystem) ReadDir(name string) ([]iofs.DirEntry, error) {
	return withPath(f, "readdir", name, func(name string) ([]iofs.DirEntry, error) {
		return f.backend.ReadDir(name)
	})
}

func (f *FileSystem) Stat(name string) (iofs.FileInfo, error) {
	return withPath(f, "stat", name, func(name string) (iofs.FileInfo, error) {
		return f.backend.Stat(name)
	})
}

func (f *FileSystem) Remove(name string) error {
	if name == "." {
		return newPathError("remove", name, ErrInvalid)
	}
	_, err := withPath(f, "remove", name, func(name string) (struct{}, error) {
		return struct{}{}, f.backend.Remove(name)
	})
	return err
}

func (f *FileSystem) RemoveAll(name string) error {
	_, err := withPath(f, "removeall", name, func(name string) (struct{}, error) {
		return struct{}{}, f.backend.RemoveAll(name)
	})
	return err
}

func (f *FileSystem) Rename(oldName, newName string) error {
	if err := validatePath("rename", oldName); err != nil {
		return err
	}
	if err := validatePath("rename", newName); err != nil {
		return err
	}
	if oldName == "." || newName == "." || strings.HasPrefix(newName, oldName+"/") {
		return newPathError("rename", oldName, ErrInvalid)
	}
	if f == nil || f.backend == nil {
		return newPathError("rename", oldName, ErrClosed)
	}
	if err := f.backend.Rename(oldName, newName); err != nil {
		return normalizePathError("rename", oldName, err)
	}
	return nil
}

func Open(name string) (iofs.File, error)          { return Default().Open(name) }
func ReadFile(name string) ([]byte, error)         { return Default().ReadFile(name) }
func WriteFile(name string, data []byte) error     { return Default().WriteFile(name, data) }
func MkdirAll(name string) error                   { return Default().MkdirAll(name) }
func ReadDir(name string) ([]iofs.DirEntry, error) { return Default().ReadDir(name) }
func Stat(name string) (iofs.FileInfo, error)      { return Default().Stat(name) }
func Remove(name string) error                     { return Default().Remove(name) }
func RemoveAll(name string) error                  { return Default().RemoveAll(name) }
func Rename(oldName, newName string) error         { return Default().Rename(oldName, newName) }

func withPath[T any](f *FileSystem, op, name string, operation func(string) (T, error)) (T, error) {
	var zero T
	if err := validatePath(op, name); err != nil {
		return zero, err
	}
	if f == nil || f.backend == nil {
		return zero, newPathError(op, name, ErrClosed)
	}
	value, err := operation(name)
	if err != nil {
		return zero, normalizePathError(op, name, err)
	}
	return value, nil
}

func validatePath(op, name string) error {
	if !iofs.ValidPath(name) || strings.ContainsRune(name, '\\') {
		return newPathError(op, name, ErrInvalid)
	}
	return nil
}

func normalizePathError(op, name string, err error) error {
	var pathErr *iofs.PathError
	if errors.As(err, &pathErr) {
		return newPathError(op, name, pathErr.Err)
	}
	return newPathError(op, name, err)
}

func newPathError(op, name string, err error) error {
	return &iofs.PathError{Op: op, Path: name, Err: err}
}
