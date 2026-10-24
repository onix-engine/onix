package resource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io/fs"
	"sync"
	"testing"
	"time"
)

func TestManagerLoadsRequiredAndDependentPackages(t *testing.T) {
	base, baseData := testPackage(t, BasePackage, true, nil, map[string][]byte{
		"game/resources/project.json": []byte(`{"window":{"title":"Test"}}`),
	})
	shared, sharedData := testPackage(t, "shared", false, []string{BasePackage}, map[string][]byte{
		"shared/common.txt": []byte("common"),
	})
	dlc, dlcData := testPackage(t, "chapter-1", false, []string{BasePackage, "shared"}, map[string][]byte{
		"game/resources/chapter-1/level.txt": []byte("level"),
	})
	catalog := Catalog{Version: CatalogVersion, Packages: []PackageInfo{base, dlc, shared}}
	catalogData, err := MarshalCatalog(catalog)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewMemoryManager(catalogData, map[string][]byte{
		base.File:   baseData,
		shared.File: sharedData,
		dlc.File:    dlcData,
	})
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}
	defer manager.Close()
	if !manager.Loaded(BasePackage) || manager.Loaded("shared") || manager.Loaded("chapter-1") {
		t.Fatalf("unexpected initially loaded packages")
	}
	if data, err := manager.ReadFile("game/resources/project.json"); err != nil || len(data) == 0 {
		t.Fatalf("read base file: data=%q err=%v", data, err)
	}
	if _, err := manager.ReadFile("game/resources/chapter-1/level.txt"); !errors.Is(err, ErrPackageNotLoaded) {
		t.Fatalf("optional file error = %v, want ErrPackageNotLoaded", err)
	}
	if err := manager.Load("chapter-1"); err != nil {
		t.Fatalf("load dependent package: %v", err)
	}
	if !manager.Loaded("shared") || !manager.Loaded("chapter-1") {
		t.Fatalf("package dependencies were not loaded")
	}
	if data, err := fs.ReadFile(manager, "game/resources/chapter-1/level.txt"); err != nil || string(data) != "level" {
		t.Fatalf("read optional file: data=%q err=%v", data, err)
	}
}

func TestManagerDetectsPackageDataMismatchOnAccess(t *testing.T) {
	base, data := testPackage(t, BasePackage, true, nil, map[string][]byte{"project.json": []byte("{}")})
	catalogData, err := MarshalCatalog(Catalog{Version: CatalogVersion, Packages: []PackageInfo{base}})
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 0xff
	manager, err := NewMemoryManager(catalogData, map[string][]byte{base.File: data})
	if err != nil {
		t.Fatalf("open structurally valid package: %v", err)
	}
	defer manager.Close()
	if _, err := manager.ReadFile("project.json"); err == nil {
		t.Fatal("manager read corrupt package data")
	}
}

func TestManagerMountsDownloadedPackageBytes(t *testing.T) {
	base, baseData := testPackage(t, BasePackage, true, nil, map[string][]byte{"base.txt": []byte("base")})
	extra, extraData := testPackage(t, "extra", false, []string{BasePackage}, map[string][]byte{"extra.txt": []byte("extra")})
	catalogData, err := MarshalCatalog(Catalog{Version: CatalogVersion, Packages: []PackageInfo{base, extra}})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewMemoryManager(catalogData, map[string][]byte{base.File: baseData})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if err := manager.MountBytes("extra", extraData); err != nil {
		t.Fatalf("mount downloaded package: %v", err)
	}
	if data, err := manager.ReadFile("extra.txt"); err != nil || string(data) != "extra" {
		t.Fatalf("read mounted package: data=%q err=%v", data, err)
	}
}

func TestCatalogRejectsDependencyCycle(t *testing.T) {
	base, _ := testPackage(t, BasePackage, true, []string{"extra"}, map[string][]byte{"base.txt": []byte("base")})
	extra, _ := testPackage(t, "extra", false, []string{BasePackage}, map[string][]byte{"extra.txt": []byte("extra")})
	catalog := Catalog{Version: CatalogVersion, Packages: []PackageInfo{base, extra}}
	if err := catalog.Validate(); err == nil {
		t.Fatal("catalog accepted a dependency cycle")
	}
}

func TestSetDefaultManagerLoadsNamedPackage(t *testing.T) {
	base, baseData := testPackage(t, BasePackage, true, nil, map[string][]byte{
		"resources/project.json": []byte("{}"),
		"shared/font.txt":        []byte("font"),
	})
	extra, extraData := testPackage(t, "extra", false, []string{BasePackage}, map[string][]byte{"extra.txt": []byte("extra")})
	catalogData, err := MarshalCatalog(Catalog{Version: CatalogVersion, Packages: []PackageInfo{base, extra}})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewMemoryManager(catalogData, map[string][]byte{base.File: baseData, extra.File: extraData})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if err := SetDefaultProject(manager, "apps/game"); err != nil {
		t.Fatal(err)
	}
	source, root, ok := Default()
	if !ok || root != "apps/game" {
		t.Fatalf("default project source root = %q, mounted = %t", root, ok)
	}
	for name, want := range map[string]string{
		"apps/game/resources/project.json": "{}",
		"shared/font.txt":                  "font",
	} {
		data, err := fs.ReadFile(source, name)
		if err != nil || string(data) != want {
			t.Errorf("read mapped project file %s: data=%q err=%v", name, data, err)
		}
	}
	if err := LoadDefaultPackage("extra"); err != nil {
		t.Fatal(err)
	}
	if !manager.Loaded("extra") {
		t.Fatal("default manager did not load named package")
	}
}

func TestManagerUnloadsOptionalPackagesAndDependencies(t *testing.T) {
	base, baseData := testPackage(t, BasePackage, true, nil, map[string][]byte{"base.txt": []byte("base")})
	shared, sharedData := testPackage(t, "shared", false, []string{BasePackage}, map[string][]byte{"shared.txt": []byte("shared")})
	extra, extraData := testPackage(t, "extra", false, []string{BasePackage, "shared"}, map[string][]byte{"extra.txt": []byte("extra")})
	catalogData, err := MarshalCatalog(Catalog{Version: CatalogVersion, Packages: []PackageInfo{base, extra, shared}})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewMemoryManager(catalogData, map[string][]byte{base.File: baseData, shared.File: sharedData, extra.File: extraData})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if err := manager.Load("extra"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Unload("extra"); err != nil {
		t.Fatal(err)
	}
	if manager.Loaded("extra") || manager.Loaded("shared") || !manager.Loaded(BasePackage) {
		t.Fatalf("unexpected residency after unload: %+v", manager.Statistics())
	}
}

func TestManagerBlobPinsPackageUntilClosed(t *testing.T) {
	base, baseData := testPackage(t, BasePackage, true, nil, map[string][]byte{"base.txt": []byte("base")})
	extra, extraData := testPackage(t, "extra", false, []string{BasePackage}, map[string][]byte{"extra.txt": []byte("extra")})
	catalogData, err := MarshalCatalog(Catalog{Version: CatalogVersion, Packages: []PackageInfo{base, extra}})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewMemoryManager(catalogData, map[string][]byte{base.File: baseData, extra.File: extraData})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if err := manager.Load("extra"); err != nil {
		t.Fatal(err)
	}
	blob, err := manager.OpenBlob("extra.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Unload("extra"); !errors.Is(err, ErrPackageInUse) {
		t.Fatalf("unload pinned package error = %v", err)
	}
	if err := blob.Close(); err != nil {
		t.Fatal(err)
	}
	if err := manager.Unload("extra"); err != nil {
		t.Fatal(err)
	}
}

func TestManagerLoadsWithoutBlockingReads(t *testing.T) {
	base, baseData := testPackage(t, BasePackage, true, nil, map[string][]byte{"base.txt": []byte("base")})
	extra, extraData := testPackage(t, "extra", false, []string{BasePackage}, map[string][]byte{"extra.txt": []byte("extra")})
	catalog := Catalog{Version: CatalogVersion, Packages: []PackageInfo{base, extra}}
	started := make(chan struct{})
	release := make(chan struct{})
	manager, err := NewManager(catalog, func(_ context.Context, info PackageInfo) (PackageSource, error) {
		data := baseData
		if info.Name == "extra" {
			close(started)
			<-release
			data = extraData
		}
		return PackageSource{ReaderAt: newMemorySource(data), Size: int64(len(data))}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if err := manager.LoadRequired(); err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		if err := manager.Load("extra"); err != nil {
			t.Errorf("load extra: %v", err)
		}
	}()
	<-started
	done := make(chan error, 1)
	go func() {
		_, err := manager.ReadFile("base.txt")
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("base resource read blocked behind optional package loader")
	}
	close(release)
	wait.Wait()
}

func testPackage(t *testing.T, name string, required bool, dependencies []string, files map[string][]byte) (PackageInfo, []byte) {
	t.Helper()
	var output bytes.Buffer
	result, err := Pack(&output, byteInputs(files))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(output.Bytes())
	return PackageInfo{
		Name:         name,
		File:         name + Extension,
		SHA256:       digest,
		Size:         int64(output.Len()),
		Required:     required,
		Dependencies: dependencies,
		Assets:       result.Assets,
	}, output.Bytes()
}
