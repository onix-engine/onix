package resource

import (
	"crypto/sha256"
	"errors"
	"hash/crc32"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sync"
	"sync/atomic"
)

// directorySource exposes a project directory as a resource filesystem. Its
// blobs are mapped from the file instead of copied into the heap, so a decoded
// font keeps its outlines available on demand and can hand resident pages back
// to the kernel once its tables are parsed.
type directorySource struct {
	fs.FS
	root string
}

// OpenDirectory returns the filesystem rooted at root. It matches os.DirFS for
// every fs.FS operation and additionally serves OpenBlob.
func OpenDirectory(root string) fs.FS {
	return &directorySource{FS: os.DirFS(root), root: root}
}

func (source *directorySource) OpenBlob(name string) (*Blob, error) {
	if source == nil {
		return nil, fs.ErrInvalid
	}
	if err := validateReadPath(name); err != nil || name == "." {
		if err == nil {
			err = errors.New("is a directory")
		}
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	return openMappedBlob(name, filepath.Join(source.root, filepath.FromSlash(name)))
}

// Sub returns the subtree rooted at dir, keeping OpenBlob so nested module
// resources stay mapped instead of being read into the heap.
func (source *directorySource) Sub(dir string) (fs.FS, error) {
	if source == nil {
		return nil, fs.ErrInvalid
	}
	if !fs.ValidPath(dir) {
		return nil, &fs.PathError{Op: "sub", Path: dir, Err: fs.ErrInvalid}
	}
	if dir == "." {
		return source, nil
	}
	return OpenDirectory(filepath.Join(source.root, filepath.FromSlash(dir))), nil
}

// Sub returns the subtree of source rooted at dir without dropping OpenBlob. A
// source that serves mapped blobs keeps serving them, so packaged builds and
// authoring directories resolve resources through the same path.
func Sub(source fs.FS, dir string) (fs.FS, error) {
	sub, err := fs.Sub(source, dir)
	if err != nil {
		return nil, err
	}
	provider, ok := source.(interface {
		OpenBlob(string) (*Blob, error)
	})
	if !ok || dir == "." {
		return sub, nil
	}
	if _, ok := sub.(interface {
		OpenBlob(string) (*Blob, error)
	}); ok {
		return sub, nil
	}
	return &subBlobFS{FS: sub, parent: provider, dir: dir}, nil
}

type subBlobFS struct {
	fs.FS
	parent interface {
		OpenBlob(string) (*Blob, error)
	}
	dir string
}

func (source *subBlobFS) OpenBlob(name string) (*Blob, error) {
	// Validate before joining: path.Join would clean ".." away and silently
	// let a module reach a sibling's resources.
	if err := validateReadPath(name); err != nil || name == "." {
		if err == nil {
			err = errors.New("is a directory")
		}
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	return source.parent.OpenBlob(path.Join(source.dir, name))
}

// mappedFileOwner unmaps one file-backed blob when its last holder closes it.
type mappedFileOwner struct {
	source PackageSource
	once   sync.Once
	err    error
}

func (owner *mappedFileOwner) releaseBlob(int64) error {
	owner.once.Do(func() { owner.err = owner.source.Closer.Close() })
	return owner.err
}

// lazyAssetIdentity computes a blob's checksum and content ID on first
// observation. Mapped blobs stay unhashed while a resource is only decoded,
// which keeps the largest project assets off the startup path.
type lazyAssetIdentity struct {
	once   sync.Once
	source io.ReaderAt
	size   int64
	name   string
	info   AssetInfo
}

func (identity *lazyAssetIdentity) resolve() AssetInfo {
	identity.once.Do(func() {
		identity.info.Path = identity.name
		identity.info.Size = identity.size
		remaining := identity.size
		if remaining <= 0 {
			return
		}
		hash := sha256.New()
		checksum := crc32.NewIEEE()
		buffer := verifyBufferPool.Get().([]byte)
		defer verifyBufferPool.Put(buffer)
		var offset int64
		for remaining > 0 {
			chunk := buffer
			if int64(len(chunk)) > remaining {
				chunk = chunk[:remaining]
			}
			if err := readAtFull(identity.source, chunk, offset); err != nil {
				return
			}
			_, _ = hash.Write(chunk)
			_, _ = checksum.Write(chunk)
			offset += int64(len(chunk))
			remaining -= int64(len(chunk))
		}
		identity.info.Checksum = checksum.Sum32()
		var digest [sha256.Size]byte
		hash.Sum(digest[:0])
		identity.info.ContentID = ContentID(digest[:ContentIDSize])
	})
	return identity.info
}

// openMappedBlob opens file as a read-only mapping when the platform maps
// package files, and falls back to an in-memory blob otherwise.
func openMappedBlob(name, file string) (*Blob, error) {
	info, err := os.Stat(file)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, &fs.PathError{Op: "open", Path: name, Err: errors.New("is a directory")}
	}
	// An empty file has no pages to map, and some platforms reject a zero
	// length mapping, so serve it from memory like any other empty resource.
	if info.Size() == 0 {
		return NewMemoryBlob(name, nil), nil
	}
	packageSource, err := openPackageFile(file)
	if err != nil {
		return nil, err
	}
	viewer, ok := packageSource.ReaderAt.(byteViewer)
	if !ok {
		_ = packageSource.Closer.Close()
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		return NewMemoryBlob(name, data), nil
	}
	verified := new(atomic.Bool)
	verified.Store(true)
	return &Blob{
		source: packageSource.ReaderAt, viewer: viewer,
		info:     AssetInfo{Path: name, Size: packageSource.Size},
		verified: verified, release: &mappedFileOwner{source: packageSource},
		identity: &lazyAssetIdentity{source: packageSource.ReaderAt, size: packageSource.Size, name: name},
	}, nil
}
