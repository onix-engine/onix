package resource

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenDirectoryServesMappedBlobs(t *testing.T) {
	dir := t.TempDir()
	data := bytes.Repeat([]byte("onixresource"), 512)
	if err := os.WriteFile(filepath.Join(dir, "asset.bin"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	source := OpenDirectory(dir)

	blob, err := source.(interface {
		OpenBlob(string) (*Blob, error)
	}).OpenBlob("asset.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer blob.Close()

	// A mapped blob exposes the file directly instead of copying it.
	view, ok := blob.View()
	if !ok {
		t.Fatal("directory blob did not expose a mapped view")
	}
	if !bytes.Equal(view, data) {
		t.Fatal("mapped view does not match the file contents")
	}
	if got := blob.Size(); got != int64(len(data)) {
		t.Fatalf("size = %d, want %d", got, len(data))
	}
	// Identity is computed on first observation and matches an eager hash.
	want := NewMemoryBlob("asset.bin", data)
	if got, expected := blob.Info(), want.Info(); got.Checksum != expected.Checksum || got.ContentID != expected.ContentID || got.Size != expected.Size {
		t.Fatalf("info = %+v, want %+v", got, expected)
	}
	if blob.ContentID() != want.ContentID() {
		t.Fatal("content ID does not match the eager hash")
	}
}

func TestSubPreservesOpenBlob(t *testing.T) {
	dir := t.TempDir()
	modDir := filepath.Join(dir, "workbench", "assets")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte("nested")
	if err := os.WriteFile(filepath.Join(modDir, "page.html"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	sub, err := Sub(OpenDirectory(dir), "workbench")
	if err != nil {
		t.Fatal(err)
	}
	blob, err := sub.(interface {
		OpenBlob(string) (*Blob, error)
	}).OpenBlob("assets/page.html")
	if err != nil {
		t.Fatal(err)
	}
	defer blob.Close()
	got, err := blob.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("read %q, want %q", got, data)
	}
	// Sub still satisfies the ordinary fs.FS contract used by the compiler.
	if _, err := fs.Stat(sub, "assets/page.html"); err != nil {
		t.Fatal(err)
	}
}

func TestSubBlobCannotEscapeItsScope(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"workbench/assets", "other/assets"} {
		if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(name)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "other", "assets", "secret.bin"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub, err := Sub(OpenDirectory(dir), "workbench")
	if err != nil {
		t.Fatal(err)
	}
	provider := sub.(interface {
		OpenBlob(string) (*Blob, error)
	})
	// path.Join would clean these into a reachable sibling path, so the scope
	// check has to run before joining.
	for _, name := range []string{"../other/assets/secret.bin", "../../other/assets/secret.bin", "/other/assets/secret.bin"} {
		if _, err := provider.OpenBlob(name); err == nil {
			t.Fatalf("OpenBlob(%q) reached outside the module scope", name)
		}
	}
}

func TestSubFallsBackWithoutBlobProvider(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub, err := Sub(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sub.(interface {
		OpenBlob(string) (*Blob, error)
	}); ok {
		t.Fatal("a plain fs.FS must not gain OpenBlob")
	}
	if _, err := fs.Stat(sub, "a.txt"); err != nil {
		t.Fatal(err)
	}
}

func TestDirectoryBlobRejectsInvalidPaths(t *testing.T) {
	source := OpenDirectory(t.TempDir())
	provider := source.(interface {
		OpenBlob(string) (*Blob, error)
	})
	for _, name := range []string{".", "..", "../escape", "/absolute"} {
		if _, err := provider.OpenBlob(name); err == nil {
			t.Fatalf("OpenBlob(%q) succeeded, want an error", name)
		}
	}
}
