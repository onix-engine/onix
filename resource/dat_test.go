package resource

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func byteInputs(files map[string][]byte) []PackInput {
	inputs := make([]PackInput, 0, len(files))
	for name, data := range files {
		inputs = append(inputs, BytesInput(name, data))
	}
	return inputs
}

func packBytes(t testing.TB, files map[string][]byte) ([]byte, PackResult) {
	t.Helper()
	var output bytes.Buffer
	result, err := Pack(&output, byteInputs(files))
	if err != nil {
		t.Fatal(err)
	}
	return output.Bytes(), result
}

func TestPackageRoundTrip(t *testing.T) {
	random := make([]byte, 4096)
	for i := range random {
		random[i] = byte(rand.Uint32())
	}
	files := map[string][]byte{
		"resources/project.bin":      bytes.Repeat([]byte{0x11, 0x22, 0x33}, 200),
		"resources/images/noise.bin": random,
		"resources/empty.bin":        {},
	}
	data, result := packBytes(t, files)
	if result.Stats.Files != len(files) || result.Stats.Stored != len(files) || result.Stats.Deduplicated != 0 {
		t.Fatalf("unexpected pack stats: %+v", result.Stats)
	}
	reader, err := NewReader(bytes.NewReader(data), int64(len(data)), result.Assets)
	if err != nil {
		t.Fatalf("open package: %v", err)
	}
	defer reader.Close()
	for name, want := range files {
		got, err := fs.ReadFile(reader, name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("unexpected data for %s", name)
		}
	}
	entries, err := fs.ReadDir(reader, "resources")
	if err != nil {
		t.Fatalf("read resources: %v", err)
	}
	gotNames := make([]string, len(entries))
	for i, entry := range entries {
		gotNames[i] = entry.Name()
	}
	if want := []string{"empty.bin", "images", "project.bin"}; !slices.Equal(gotNames, want) {
		t.Fatalf("resource entries = %v, want %v", gotNames, want)
	}
}

func TestPackageIsDeterministic(t *testing.T) {
	files := map[string][]byte{"b.bin": bytes.Repeat([]byte("b"), 2048), "a.bin": bytes.Repeat([]byte("a"), 2048)}
	first, firstResult := packBytes(t, files)
	second, secondResult := packBytes(t, files)
	if !bytes.Equal(first, second) || !slices.Equal(firstResult.Assets, secondResult.Assets) {
		t.Fatal("same inputs produced different package metadata or bytes")
	}
}

func TestPackageDeduplicatesIdenticalBlobs(t *testing.T) {
	payload := bytes.Repeat([]byte("shared payload"), 512)
	data, result := packBytes(t, map[string][]byte{"a.bin": slices.Clone(payload), "b.bin": slices.Clone(payload)})
	if result.Stats.Stored != 1 || result.Stats.Deduplicated != 1 || result.Stats.DeduplicatedBytes != int64(len(payload)) {
		t.Fatalf("dedup stats = %+v", result.Stats)
	}
	reader, err := NewReader(bytes.NewReader(data), int64(len(data)), result.Assets)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if reader.offsets[0] != reader.offsets[1] {
		t.Fatalf("duplicate entries use different offsets: %v", reader.offsets)
	}
	for _, name := range []string{"a.bin", "b.bin"} {
		got, err := reader.ReadFile(name)
		if err != nil || !bytes.Equal(got, payload) {
			t.Fatalf("read %s: data=%d err=%v", name, len(got), err)
		}
	}
}

func TestPackageReadsRawBlobWithoutDecode(t *testing.T) {
	payload := bytes.Repeat([]byte("this remains a compact binary blob\x00"), 1024)
	data, result := packBytes(t, map[string][]byte{"resource.bin": payload})
	source := &countingReaderAt{data: data}
	reader, err := NewReader(source, int64(len(data)), result.Assets)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	source.reads, source.bytes = 0, 0
	got, err := reader.ReadFile("resource.bin")
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("read raw blob: size=%d err=%v", len(got), err)
	}
	if source.reads != 1 || source.bytes != len(payload) {
		t.Fatalf("raw read count = %d, bytes = %d, want one read of %d", source.reads, source.bytes, len(payload))
	}
}

func TestPackageRejectsMismatchedAliasMetadata(t *testing.T) {
	payload := []byte("alias payload")
	data, result := packBytes(t, map[string][]byte{"a.bin": payload, "b.bin": slices.Clone(payload)})
	assets := slices.Clone(result.Assets)
	assets[1].ContentID[0] ^= 0xff
	if _, err := NewReader(bytes.NewReader(data), int64(len(data)), assets); err == nil {
		t.Fatal("reader accepted an alias with mismatched catalog metadata")
	}
}

type countingReaderAt struct {
	data  []byte
	reads int
	bytes int
}

var packageReadSink []byte

func (r *countingReaderAt) ReadAt(p []byte, offset int64) (int, error) {
	r.reads++
	r.bytes += len(p)
	if offset < 0 || offset >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[offset:])
	if n != len(p) {
		return n, io.EOF
	}
	return n, nil
}

func TestPackageReadAllocatesOnlyResult(t *testing.T) {
	payload := bytes.Repeat([]byte("allocation check"), 256)
	data, result := packBytes(t, map[string][]byte{"resource.bin": payload})
	reader, err := NewReader(bytes.NewReader(data), int64(len(data)), result.Assets)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	allocations := testing.AllocsPerRun(100, func() {
		packageReadSink, err = reader.ReadFile("resource.bin")
		if err != nil {
			t.Fatal(err)
		}
	})
	if allocations != 1 {
		t.Fatalf("package read allocated %.0f times, want only the result buffer", allocations)
	}
}

func TestMemoryBlobBorrowsPackageBytes(t *testing.T) {
	payload := []byte("borrowed")
	data, result := packBytes(t, map[string][]byte{"resource.bin": payload})
	source := newMemorySource(data)
	reader, err := NewReader(source, int64(len(data)), result.Assets)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := reader.OpenBlob("resource.bin")
	if err != nil {
		t.Fatal(err)
	}
	borrowed, err := blob.Bytes()
	if err != nil || string(borrowed) != "borrowed" {
		t.Fatalf("borrowed bytes = %q, err = %v", borrowed, err)
	}
	if &borrowed[0] != &data[reader.dataOffset] {
		t.Fatal("memory blob did not return a package-backed view")
	}
	_ = blob.Close()
}

func TestMappedBlobRemainsReadableAfterReleasingResidentPages(t *testing.T) {
	payload := bytes.Repeat([]byte("mapped font data"), 1<<16)
	data, result := packBytes(t, map[string][]byte{"font.bin": payload})
	name := filepath.Join(t.TempDir(), "base.dat")
	if err := os.WriteFile(name, data, 0o644); err != nil {
		t.Fatal(err)
	}
	source, err := openPackageFile(name)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Closer.Close()
	reader, err := NewReader(source.ReaderAt, source.Size, result.Assets)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	blob, err := reader.OpenBlob("font.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer blob.Close()
	if _, err := blob.Bytes(); err != nil {
		t.Fatal(err)
	}
	if err := blob.ReleaseResidentPages(); err != nil {
		t.Fatal(err)
	}
	got, err := blob.ReadAll()
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("read after releasing pages: size=%d, err=%v", len(got), err)
	}
}

func TestPackageStreamsFileInput(t *testing.T) {
	payload := bytes.Repeat([]byte("file backed"), 1024)
	filename := filepath.Join(t.TempDir(), "asset.bin")
	if err := os.WriteFile(filename, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	result, err := Pack(&output, []PackInput{FileInput("asset.bin", filename)})
	if err != nil {
		t.Fatal(err)
	}
	reader, err := NewReader(bytes.NewReader(output.Bytes()), int64(output.Len()), result.Assets)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reader.ReadFile("asset.bin")
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("file-backed input = %d bytes, err = %v", len(got), err)
	}
}

func TestPackageRejectsInvalidPaths(t *testing.T) {
	for _, name := range []string{".", "../secret", "/absolute", `bad\path`} {
		t.Run(name, func(t *testing.T) {
			if _, err := Pack(&bytes.Buffer{}, []PackInput{BytesInput(name, nil)}); err == nil {
				t.Fatalf("accepted invalid path %q", name)
			}
		})
	}
}

func TestPackageRejectsFileDirectoryPathConflict(t *testing.T) {
	inputs := []PackInput{BytesInput("data", []byte("file")), BytesInput("data/child.bin", []byte("child"))}
	if _, err := Pack(&bytes.Buffer{}, inputs); err == nil {
		t.Fatal("package accepted a path as both file and directory")
	}
}

func TestValidateSortedResourcePathsDoesNotAllocate(t *testing.T) {
	inputs := []PackInput{{Path: "audio/theme.ogg"}, {Path: "resources/images/icon.png"}, {Path: "resources/project.bin"}, {Path: "shaders/effect.spv"}}
	allocations := testing.AllocsPerRun(100, func() {
		if err := validateSortedResourcePaths(inputs); err != nil {
			t.Fatal(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("resource path validation allocated %.0f times", allocations)
	}
}

func TestPackageDetectsCorruptEntryOnAccess(t *testing.T) {
	data, result := packBytes(t, map[string][]byte{"data.bin": bytes.Repeat([]byte("payload"), 100)})
	corrupt := slices.Clone(data)
	corrupt[len(corrupt)-1] ^= 0xff
	reader, err := NewReader(bytes.NewReader(corrupt), int64(len(corrupt)), result.Assets)
	if err != nil {
		t.Fatalf("open structurally valid corrupt package: %v", err)
	}
	defer reader.Close()
	if _, err := reader.ReadFile("data.bin"); err == nil {
		t.Fatal("read corrupt package entry without error")
	}
}

func TestPackageRejectsImpossibleAssetCount(t *testing.T) {
	data := make([]byte, headerSize)
	copy(data, formatMagic[:])
	binary.LittleEndian.PutUint16(data[8:10], FormatVersion)
	binary.LittleEndian.PutUint16(data[10:12], headerSize)
	binary.LittleEndian.PutUint32(data[12:16], 1)
	asset := AssetInfo{Path: "asset.bin", ContentID: contentID(nil)}
	if _, err := NewReader(bytes.NewReader(data), int64(len(data)), []AssetInfo{asset}); err == nil {
		t.Fatal("reader accepted an asset count that cannot fit in the index")
	}
}

func TestPackageFileRequiresCatalogAssets(t *testing.T) {
	data, result := packBytes(t, map[string][]byte{"project.bin": []byte("{}")})
	name := filepath.Join(t.TempDir(), "base"+Extension)
	if err := os.WriteFile(name, data, 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := NewReader(file, int64(len(data)), result.Assets)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := reader.ReadFile("project.bin"); err != nil || string(got) != "{}" {
		t.Fatalf("read package file: data=%q err=%v", got, err)
	}
	_ = reader.Close()
	_ = file.Close()
	if _, err := reader.ReadFile("project.bin"); !errors.Is(err, fs.ErrClosed) {
		t.Fatalf("read closed package error = %v, want ErrClosed", err)
	}
}

func TestPackageRejectsRetiredFormatMarker(t *testing.T) {
	data := make([]byte, headerSize)
	copy(data, []byte("ONIXDAT4"))
	if _, err := NewReader(bytes.NewReader(data), int64(len(data)), nil); err == nil {
		t.Fatal("retired package v4 marker was accepted")
	}
}

func FuzzPackageReader(f *testing.F) {
	seed, result := packBytes(f, map[string][]byte{"seed.bin": []byte("seed")})
	f.Add(seed)
	f.Add([]byte("not a package"))
	f.Fuzz(func(t *testing.T, data []byte) {
		reader, err := NewReader(bytes.NewReader(data), int64(len(data)), result.Assets)
		if err != nil {
			return
		}
		defer reader.Close()
		_, _ = fs.ReadDir(reader, ".")
		_, _ = reader.ReadFile("seed.bin")
	})
}
