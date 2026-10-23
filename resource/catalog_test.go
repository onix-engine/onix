package resource

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"hash/crc32"
	"reflect"
	"testing"
)

func testCatalog() Catalog {
	baseData := []byte("project")
	chapterData := []byte("chapter")
	baseDigest := sha256.Sum256([]byte("base package"))
	chapterDigest := sha256.Sum256([]byte("chapter package"))
	return Catalog{Version: CatalogVersion, Packages: []PackageInfo{
		{Name: BasePackage, File: "base.dat", SHA256: baseDigest, Size: headerSize, Required: true, Assets: []AssetInfo{{Path: "resources/project.json", Size: int64(len(baseData)), Checksum: crc32.ChecksumIEEE(baseData), ContentID: contentID(baseData)}}},
		{Name: "chapter", File: "chapter.dat", SHA256: chapterDigest, Size: headerSize + 10, Dependencies: []string{BasePackage}, Assets: []AssetInfo{{Path: "resources/chapter/map.bin", Size: int64(len(chapterData)), Checksum: crc32.ChecksumIEEE(chapterData), ContentID: contentID(chapterData)}}},
	}}
}

func TestCatalogBinaryRoundTripIsDeterministic(t *testing.T) {
	want := testCatalog()
	first, err := MarshalCatalog(want)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MarshalCatalog(want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("catalog encoding is not deterministic")
	}
	jsonData, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) >= len(jsonData) {
		t.Fatalf("binary catalog size = %d, JSON size = %d", len(first), len(jsonData))
	}
	t.Logf("catalog: JSON=%d bytes binary=%d bytes", len(jsonData), len(first))
	got, err := ParseCatalog(first)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded catalog = %#v, want %#v", got, want)
	}
}

func TestParseCatalogRejectsJSONAndCorruption(t *testing.T) {
	if _, err := ParseCatalog([]byte(`{"version":2}`)); err == nil {
		t.Fatal("JSON catalog was accepted")
	}
	if _, err := ParseCatalog([]byte("ONIXCAT1")); err == nil {
		t.Fatal("retired catalog v1 marker was accepted")
	}
	data, err := MarshalCatalog(testCatalog())
	if err != nil {
		t.Fatal(err)
	}
	for length := range len(data) {
		if _, err := ParseCatalog(data[:length]); err == nil {
			t.Fatalf("truncated catalog length %d was accepted", length)
		}
	}
	if _, err := ParseCatalog(append(data, 0)); err == nil {
		t.Fatal("catalog with trailing data was accepted")
	}
}
