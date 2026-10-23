//go:build !js

package fs

import (
	"errors"
	iofs "io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

var (
	_ iofs.FS         = (*FileSystem)(nil)
	_ iofs.ReadFileFS = (*FileSystem)(nil)
	_ iofs.ReadDirFS  = (*FileSystem)(nil)
	_ iofs.StatFS     = (*FileSystem)(nil)
)

func TestFileSystemReadWriteAndOpen(t *testing.T) {
	fileSystem := newTestFileSystem(t)
	data := []byte("player-save")
	if err := fileSystem.WriteFile("profiles/slot-1/save.dat", data); err != nil {
		t.Fatalf("write file: %v", err)
	}

	got, err := fileSystem.ReadFile("profiles/slot-1/save.dat")
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !reflect.DeepEqual(got, data) {
		t.Fatalf("read data = %q, want %q", got, data)
	}

	file, err := fileSystem.Open("profiles/slot-1/save.dat")
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		t.Fatalf("stat open file: %v", err)
	}
	if info.IsDir() || info.Size() != int64(len(data)) {
		t.Fatalf("unexpected file info: dir=%v size=%d", info.IsDir(), info.Size())
	}
}

func TestFileSystemReadDirIsSorted(t *testing.T) {
	fileSystem := newTestFileSystem(t)
	for _, name := range []string{"saves/z.dat", "saves/a.dat", "saves/nested/value.dat"} {
		if err := fileSystem.WriteFile(name, []byte(name)); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	entries, err := fileSystem.ReadDir("saves")
	if err != nil {
		t.Fatalf("read directory: %v", err)
	}
	got := make([]string, len(entries))
	for i, entry := range entries {
		got[i] = entry.Name()
	}
	want := []string{"a.dat", "nested", "z.dat"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
}

func TestFileSystemSupportsStandardWalkDir(t *testing.T) {
	fileSystem := newTestFileSystem(t)
	for _, name := range []string{"a/one", "a/b/two", "c/three"} {
		if err := fileSystem.WriteFile(name, nil); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	var paths []string
	if err := iofs.WalkDir(fileSystem, ".", func(name string, entry iofs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		paths = append(paths, name)
		return nil
	}); err != nil {
		t.Fatalf("walk filesystem: %v", err)
	}
	want := []string{".", "a", "a/b", "a/b/two", "a/one", "c", "c/three"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("walk paths = %v, want %v", paths, want)
	}
}

func TestFileSystemRejectsPlatformAndEscapingPaths(t *testing.T) {
	fileSystem := newTestFileSystem(t)
	for _, name := range []string{"", "/absolute", "../escape", "a/../escape", `windows\\path`} {
		if _, err := fileSystem.ReadFile(name); !errors.Is(err, ErrInvalid) {
			t.Errorf("ReadFile(%q) error = %v, want ErrInvalid", name, err)
		}
	}
	if err := fileSystem.WriteFile(".", nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("WriteFile(\".\") error = %v, want ErrInvalid", err)
	}
}

func TestNilFileSystemReturnsClosedError(t *testing.T) {
	var fileSystem *FileSystem
	if _, err := fileSystem.ReadFile("save.dat"); !errors.Is(err, ErrClosed) {
		t.Fatalf("nil filesystem error = %v, want ErrClosed", err)
	}
}

func TestFileSystemRejectsFileAsParentDirectory(t *testing.T) {
	fileSystem := newTestFileSystem(t)
	if err := fileSystem.WriteFile("profile", nil); err != nil {
		t.Fatalf("write parent file: %v", err)
	}
	if err := fileSystem.WriteFile("profile/save.dat", nil); err == nil {
		t.Fatal("expected write below a file to fail")
	}
	if err := fileSystem.MkdirAll("profile/slots"); err == nil {
		t.Fatal("expected mkdir below a file to fail")
	}
}

func TestFileSystemRemoveRequiresEmptyDirectory(t *testing.T) {
	fileSystem := newTestFileSystem(t)
	if err := fileSystem.WriteFile("slots/one/save.dat", nil); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := fileSystem.Remove("slots"); !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("remove non-empty directory error = %v, want ErrNotEmpty", err)
	}
	if err := fileSystem.RemoveAll("slots"); err != nil {
		t.Fatalf("remove directory tree: %v", err)
	}
	if _, err := fileSystem.Stat("slots"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("stat removed directory error = %v, want ErrNotExist", err)
	}
	if err := fileSystem.RemoveAll("missing"); err != nil {
		t.Fatalf("remove missing tree: %v", err)
	}
}

func TestFileSystemRenameMovesDirectoryTree(t *testing.T) {
	fileSystem := newTestFileSystem(t)
	if err := fileSystem.WriteFile("profiles/old/nested/save.dat", []byte("save")); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := fileSystem.Rename("profiles/old", "archive/slot-1"); err != nil {
		t.Fatalf("rename directory: %v", err)
	}
	if _, err := fileSystem.Stat("profiles/old"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("old directory stat error = %v, want ErrNotExist", err)
	}
	data, err := fileSystem.ReadFile("archive/slot-1/nested/save.dat")
	if err != nil {
		t.Fatalf("read renamed file: %v", err)
	}
	if string(data) != "save" {
		t.Fatalf("renamed data = %q, want save", data)
	}
}

func TestFileSystemRenameRejectsExistingTargetAndOwnChild(t *testing.T) {
	fileSystem := newTestFileSystem(t)
	for _, name := range []string{"one/value", "two/value"} {
		if err := fileSystem.WriteFile(name, nil); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := fileSystem.Rename("one", "two"); !errors.Is(err, ErrExist) {
		t.Fatalf("rename over target error = %v, want ErrExist", err)
	}
	if err := fileSystem.Rename("one", "one/nested"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("rename into child error = %v, want ErrInvalid", err)
	}
}

func TestFileSystemsAreIsolated(t *testing.T) {
	first := newTestFileSystem(t)
	second := newTestFileSystem(t)
	if err := first.WriteFile("save.dat", []byte("first")); err != nil {
		t.Fatalf("write first filesystem: %v", err)
	}
	if _, err := second.ReadFile("save.dat"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("read isolated filesystem error = %v, want ErrNotExist", err)
	}
}

func TestMountReplacesPackageDefault(t *testing.T) {
	previous := Default()
	defer defaultFileSystem.Store(previous)

	root := filepath.Join(t.TempDir(), "new", "root")
	if err := Mount(root); err != nil {
		t.Fatalf("mount filesystem: %v", err)
	}
	if err := WriteFile("save.dat", []byte("mounted")); err != nil {
		t.Fatalf("write through package default: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "save.dat"))
	if err != nil {
		t.Fatalf("read mounted host file: %v", err)
	}
	if string(data) != "mounted" {
		t.Fatalf("mounted data = %q, want mounted", data)
	}
}

func TestNewRejectsFileAsRoot(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "file")
	if err := os.WriteFile(filename, nil, 0o644); err != nil {
		t.Fatalf("write root file: %v", err)
	}
	if _, err := New(filename); err == nil {
		t.Fatal("expected file root to be rejected")
	}
}

func TestFindResourcesRootFromPrefersNearestResourcesDirectory(t *testing.T) {
	base := t.TempDir()
	rootResources := filepath.Join(base, "resources")
	projectDir := filepath.Join(base, "projects", "hello")
	projectResources := filepath.Join(projectDir, "resources")
	for _, dir := range []string{rootResources, projectResources} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	root, ok, err := findResourcesRootFrom(projectDir)
	if err != nil {
		t.Fatalf("find resources root: %v", err)
	}
	if !ok || root != projectResources {
		t.Fatalf("resources root = %q, %v; want %q, true", root, ok, projectResources)
	}
}

func TestFindResourcesRootFromUsesResourcesDirectoryItself(t *testing.T) {
	resourcesDir := filepath.Join(t.TempDir(), "resources")
	if err := os.MkdirAll(resourcesDir, 0o755); err != nil {
		t.Fatalf("mkdir resources: %v", err)
	}
	root, ok, err := findResourcesRootFrom(resourcesDir)
	if err != nil {
		t.Fatalf("find resources root: %v", err)
	}
	if !ok || root != resourcesDir {
		t.Fatalf("resources root = %q, %v; want %q, true", root, ok, resourcesDir)
	}
}

func newTestFileSystem(t *testing.T) *FileSystem {
	t.Helper()
	fileSystem, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("create filesystem: %v", err)
	}
	return fileSystem
}
