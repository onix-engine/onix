//go:build (linux || darwin) && !android && !ios

package resource

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

type mappedPackageSource struct {
	memorySource
}

func openPackageFile(name string) (PackageSource, error) {
	file, err := os.Open(name)
	if err != nil {
		return PackageSource{}, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return PackageSource{}, err
	}
	if info.Size() > int64(maxInt()) {
		_ = file.Close()
		return PackageSource{}, fmt.Errorf("resource: package %q is too large to map", name)
	}
	data, err := unix.Mmap(int(file.Fd()), 0, int(info.Size()), unix.PROT_READ, unix.MAP_SHARED)
	closeErr := file.Close()
	if err != nil {
		return PackageSource{}, err
	}
	if closeErr != nil {
		_ = unix.Munmap(data)
		return PackageSource{}, closeErr
	}
	source := &mappedPackageSource{memorySource: memorySource{data: data}}
	return PackageSource{ReaderAt: source, Size: info.Size(), Closer: source}, nil
}

func (source *mappedPackageSource) Close() error {
	if source == nil || source.data == nil {
		return nil
	}
	data := source.data
	source.data = nil
	return unix.Munmap(data)
}

func (source *mappedPackageSource) releaseResidentPages(offset, size int64) error {
	page := int64(os.Getpagesize())
	start := (offset + page - 1) / page * page
	end := (offset + size) / page * page
	if start >= end {
		return nil
	}
	return unix.Madvise(source.data[start:end], unix.MADV_DONTNEED)
}
