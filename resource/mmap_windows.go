//go:build windows

package resource

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

type mappedPackageSource struct {
	memorySource
	address uintptr
	mapping windows.Handle
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
	mapping, err := windows.CreateFileMapping(windows.Handle(file.Fd()), nil, windows.PAGE_READONLY, 0, 0, nil)
	if err != nil {
		_ = file.Close()
		return PackageSource{}, err
	}
	address, err := windows.MapViewOfFile(mapping, windows.FILE_MAP_READ, 0, 0, uintptr(info.Size()))
	closeErr := file.Close()
	if err != nil {
		_ = windows.CloseHandle(mapping)
		return PackageSource{}, err
	}
	if closeErr != nil {
		_ = windows.UnmapViewOfFile(address)
		_ = windows.CloseHandle(mapping)
		return PackageSource{}, closeErr
	}
	data := unsafe.Slice((*byte)(unsafe.Pointer(address)), int(info.Size()))
	source := &mappedPackageSource{memorySource: memorySource{data: data}, address: address, mapping: mapping}
	return PackageSource{ReaderAt: source, Size: info.Size(), Closer: source}, nil
}

func (source *mappedPackageSource) Close() error {
	if source == nil || source.address == 0 {
		return nil
	}
	address, mapping := source.address, source.mapping
	source.address, source.mapping, source.data = 0, 0, nil
	unmapErr := windows.UnmapViewOfFile(address)
	closeErr := windows.CloseHandle(mapping)
	if unmapErr != nil {
		return unmapErr
	}
	return closeErr
}
