package project

import (
	"errors"
	"fmt"
	iofs "io/fs"
	"strings"

	"github.com/diiyw/onix/resource"
)

type projectBlobSource interface {
	OpenBlob(string) (*resource.Blob, error)
}

func openProjectBlob(source iofs.FS, name string) (*resource.Blob, error) {
	if source == nil {
		return nil, errors.New("project resource source is nil")
	}
	if !iofs.ValidPath(name) || strings.ContainsRune(name, '\\') {
		return nil, &iofs.PathError{Op: "open", Path: name, Err: iofs.ErrInvalid}
	}
	if provider, ok := source.(projectBlobSource); ok {
		return provider.OpenBlob(name)
	}
	data, err := iofs.ReadFile(source, name)
	if err != nil {
		return nil, err
	}
	return resource.NewMemoryBlob(name, data), nil
}

func readProjectFile(source iofs.FS, name string) ([]byte, error) {
	if source == nil {
		return nil, errors.New("project resource source is nil")
	}
	if !iofs.ValidPath(name) || strings.ContainsRune(name, '\\') {
		return nil, &iofs.PathError{Op: "read", Path: name, Err: iofs.ErrInvalid}
	}
	return iofs.ReadFile(source, name)
}

func projectFileExists(source iofs.FS, name string) (bool, error) {
	if source == nil {
		return false, errors.New("project resource source is nil")
	}
	if !iofs.ValidPath(name) || strings.ContainsRune(name, '\\') {
		return false, &iofs.PathError{Op: "stat", Path: name, Err: iofs.ErrInvalid}
	}
	info, err := iofs.Stat(source, name)
	if err == nil {
		return !info.IsDir(), nil
	}
	if errors.Is(err, iofs.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("stat %s: %w", name, err)
}
