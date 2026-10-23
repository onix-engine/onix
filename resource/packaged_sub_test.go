package resource

import (
	"bytes"
	"testing"
)

// TestPackagedSubKeepsBlobViews pins the release path: a package-backed source
// composed with Sub must keep serving mapped blobs, so fonts stay file-backed
// after packaging instead of being copied into the heap.
func TestPackagedSubKeepsBlobViews(t *testing.T) {
	base, data := testPackage(t, BasePackage, true, nil, map[string][]byte{
		"workbench/assets/fonts/ui.ttf": []byte("font-bytes"),
	})
	catalogData, err := MarshalCatalog(Catalog{Version: CatalogVersion, Packages: []PackageInfo{base}})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewMemoryManager(catalogData, map[string][]byte{base.File: data})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	scoped, err := Sub(manager, "workbench")
	if err != nil {
		t.Fatal(err)
	}
	provider, ok := scoped.(interface {
		OpenBlob(string) (*Blob, error)
	})
	if !ok {
		t.Fatal("Sub dropped OpenBlob for a package source")
	}
	blob, err := provider.OpenBlob("assets/fonts/ui.ttf")
	if err != nil {
		t.Fatal(err)
	}
	defer blob.Close()
	view, ok := blob.View()
	if !ok {
		t.Fatal("packaged blob lost its mapped view")
	}
	if !bytes.Equal(view, []byte("font-bytes")) {
		t.Fatalf("view = %q", view)
	}
	// The scoped source still behaves as an ordinary fs.FS.
	sub, err := Sub(scoped, "assets")
	if err != nil {
		t.Fatal(err)
	}
	nested, err := sub.(interface {
		OpenBlob(string) (*Blob, error)
	}).OpenBlob("fonts/ui.ttf")
	if err != nil {
		t.Fatal(err)
	}
	defer nested.Close()
	if view, ok := nested.View(); !ok || !bytes.Equal(view, []byte("font-bytes")) {
		t.Fatal("nested blob did not stay mapped")
	}
}
