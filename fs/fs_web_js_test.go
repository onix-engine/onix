//go:build js && wasm && !wechat

package fs

import (
	"errors"
	iofs "io/fs"
	"reflect"
	"testing"
)

func TestIndexedDBFileSystemLifecycle(t *testing.T) {
	fileSystem, err := New("test-lifecycle")
	if err != nil {
		t.Fatalf("create IndexedDB filesystem: %v", err)
	}
	if err := fileSystem.RemoveAll("."); err != nil {
		t.Fatalf("clear IndexedDB filesystem: %v", err)
	}

	if err := fileSystem.WriteFile("profiles/one/save.dat", []byte("save")); err != nil {
		t.Fatalf("write IndexedDB file: %v", err)
	}
	data, err := fileSystem.ReadFile("profiles/one/save.dat")
	if err != nil {
		t.Fatalf("read IndexedDB file: %v", err)
	}
	if string(data) != "save" {
		t.Fatalf("IndexedDB data = %q, want save", data)
	}
	reopened, err := New("test-lifecycle")
	if err != nil {
		t.Fatalf("reopen IndexedDB filesystem: %v", err)
	}
	if _, err := reopened.ReadFile("profiles/one/save.dat"); err != nil {
		t.Fatalf("read persisted IndexedDB file: %v", err)
	}
	if err := fileSystem.WriteFile("blocker", nil); err != nil {
		t.Fatalf("write IndexedDB parent file: %v", err)
	}
	if err := fileSystem.WriteFile("blocker/child", nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("write below IndexedDB file error = %v, want ErrInvalid", err)
	}

	entries, err := fileSystem.ReadDir("profiles")
	if err != nil {
		t.Fatalf("read IndexedDB directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "one" || !entries[0].IsDir() {
		t.Fatalf("unexpected IndexedDB entries: %+v", entries)
	}

	if err := fileSystem.Rename("profiles/one", "archive/slot-1"); err != nil {
		t.Fatalf("rename IndexedDB directory: %v", err)
	}
	if _, err := fileSystem.ReadFile("archive/slot-1/save.dat"); err != nil {
		t.Fatalf("read renamed IndexedDB file: %v", err)
	}
	if err := fileSystem.Remove("archive"); !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("remove non-empty IndexedDB directory error = %v, want ErrNotEmpty", err)
	}
	if err := fileSystem.RemoveAll("archive"); err != nil {
		t.Fatalf("remove IndexedDB directory tree: %v", err)
	}
	if _, err := fileSystem.Stat("archive"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("stat removed IndexedDB directory error = %v, want ErrNotExist", err)
	}
}

func TestIndexedDBFileSystemSupportsWalkDir(t *testing.T) {
	fileSystem, err := New("test-walk")
	if err != nil {
		t.Fatalf("create IndexedDB filesystem: %v", err)
	}
	if err := fileSystem.RemoveAll("."); err != nil {
		t.Fatalf("clear IndexedDB filesystem: %v", err)
	}
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
		t.Fatalf("walk IndexedDB filesystem: %v", err)
	}
	want := []string{".", "a", "a/b", "a/b/two", "a/one", "c", "c/three"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("walk paths = %v, want %v", paths, want)
	}
}
