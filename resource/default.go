package resource

import (
	"fmt"
	"io/fs"
	"path"
	"strings"
	"sync"
)

var defaultSource struct {
	sync.RWMutex
	source  fs.FS
	root    string
	manager *Manager
}

func SetDefault(source fs.FS, root string) error {
	return setDefault(source, root, source)
}

// SetDefaultProject mounts project-relative packaged files at their source project root.
func SetDefaultProject(source fs.FS, root string) error {
	if source == nil {
		return fmt.Errorf("resource: default source is nil")
	}
	managerSource := source
	normalized, err := normalizeProjectRoot(root)
	if err != nil {
		return err
	}
	if normalized != "." {
		source = projectRootFS{FS: source, root: normalized}
	}
	return setDefault(source, normalized, managerSource)
}

func setDefault(source fs.FS, root string, managerSource fs.FS) error {
	if source == nil {
		return fmt.Errorf("resource: default source is nil")
	}
	normalized, err := normalizeProjectRoot(root)
	if err != nil {
		return err
	}
	manager, _ := managerSource.(*Manager)
	defaultSource.Lock()
	defaultSource.source = source
	defaultSource.root = normalized
	defaultSource.manager = manager
	defaultSource.Unlock()
	return nil
}

func normalizeProjectRoot(root string) (string, error) {
	root = path.Clean(root)
	if root != "." && (!fs.ValidPath(root) || strings.ContainsRune(root, '\\')) {
		return "", fmt.Errorf("resource: invalid default project root %q", root)
	}
	return root, nil
}

type projectRootFS struct {
	fs.FS
	root string
}

func (source projectRootFS) mapPath(name string) string {
	switch {
	case name == source.root:
		return "."
	case strings.HasPrefix(name, source.root+"/"):
		return strings.TrimPrefix(name, source.root+"/")
	default:
		return name
	}
}

func (source projectRootFS) Open(name string) (fs.File, error) {
	return source.FS.Open(source.mapPath(name))
}

func (source projectRootFS) OpenBlob(name string) (*Blob, error) {
	name = source.mapPath(name)
	provider, ok := source.FS.(interface {
		OpenBlob(string) (*Blob, error)
	})
	if ok {
		return provider.OpenBlob(name)
	}
	data, err := fs.ReadFile(source.FS, name)
	if err != nil {
		return nil, err
	}
	return NewMemoryBlob(name, data), nil
}

func Default() (fs.FS, string, bool) {
	defaultSource.RLock()
	source, root := defaultSource.source, defaultSource.root
	defaultSource.RUnlock()
	return source, root, source != nil
}

func LoadDefaultPackage(name string) error {
	manager, ok := DefaultManager()
	if !ok {
		return fmt.Errorf("resource: default source is not a package manager")
	}
	return manager.Load(name)
}

func MountDefaultPackage(name string, data []byte) error {
	manager, ok := DefaultManager()
	if !ok {
		return fmt.Errorf("resource: default source is not a package manager")
	}
	return manager.MountBytes(name, data)
}

func DefaultManager() (*Manager, bool) {
	defaultSource.RLock()
	manager := defaultSource.manager
	defaultSource.RUnlock()
	return manager, manager != nil
}
