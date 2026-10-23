//go:build !js

package fs

import (
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"
)

const envResourcesRoot = "ONIX_RESOURCES_ROOT"

type nativeBackend struct {
	root string
}

func platformDefaultRoot() (string, error) {
	if root := os.Getenv(envResourcesRoot); root != "" {
		return root, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve filesystem working directory: %w", err)
	}
	if root, ok, err := findResourcesRootFrom(cwd); err != nil {
		return "", err
	} else if ok {
		return root, nil
	}

	if executable, err := os.Executable(); err == nil {
		if root, ok, err := findResourcesRootFrom(filepath.Dir(executable)); err != nil {
			return "", err
		} else if ok {
			return root, nil
		}
	}
	return cwd, nil
}

func newPlatformBackend(root string) (backend, error) {
	if root == "" {
		return nil, fmt.Errorf("filesystem root is empty")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve filesystem root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o755); err != nil {
		return nil, fmt.Errorf("create filesystem root: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("stat filesystem root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("filesystem root is not a directory: %s", absolute)
	}
	return &nativeBackend{root: absolute}, nil
}

func findResourcesRootFrom(start string) (string, bool, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", false, err
	}
	for {
		candidate := filepath.Join(dir, "resources")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, true, nil
		}
		if filepath.Base(dir) == "resources" {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				return dir, true, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false, nil
		}
		dir = parent
	}
}

func (b *nativeBackend) Open(name string) (iofs.File, error) {
	return os.Open(b.resolve(name))
}

func (b *nativeBackend) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(b.resolve(name))
}

func (b *nativeBackend) WriteFile(name string, data []byte) error {
	filename := b.resolve(name)
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0o644)
}

func (b *nativeBackend) MkdirAll(name string) error {
	return os.MkdirAll(b.resolve(name), 0o755)
}

func (b *nativeBackend) ReadDir(name string) ([]iofs.DirEntry, error) {
	return os.ReadDir(b.resolve(name))
}

func (b *nativeBackend) Stat(name string) (iofs.FileInfo, error) {
	return os.Stat(b.resolve(name))
}

func (b *nativeBackend) Remove(name string) error {
	info, err := os.Stat(b.resolve(name))
	if err != nil {
		return err
	}
	if info.IsDir() {
		entries, err := os.ReadDir(b.resolve(name))
		if err != nil {
			return err
		}
		if len(entries) != 0 {
			return ErrNotEmpty
		}
	}
	return os.Remove(b.resolve(name))
}

func (b *nativeBackend) RemoveAll(name string) error {
	if name == "." {
		entries, err := os.ReadDir(b.root)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := os.RemoveAll(filepath.Join(b.root, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	return os.RemoveAll(b.resolve(name))
}

func (b *nativeBackend) Rename(oldName, newName string) error {
	oldPath := b.resolve(oldName)
	newPath := b.resolve(newName)
	if _, err := os.Stat(oldPath); err != nil {
		return err
	}
	if _, err := os.Stat(newPath); err == nil {
		return ErrExist
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return err
	}
	return os.Rename(oldPath, newPath)
}

func (b *nativeBackend) resolve(name string) string {
	if name == "." {
		return b.root
	}
	return filepath.Join(b.root, filepath.FromSlash(name))
}
