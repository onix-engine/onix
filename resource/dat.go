package resource

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	iofs "io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	Extension         = ".dat"
	FormatVersion     = 5
	headerSize        = 32
	minIndexEntrySize = 2
	packBufferSize    = 64 << 10
)

var formatMagic = [8]byte{'O', 'N', 'I', 'X', 'D', 'A', 'T', '5'}

const entryAliasFlag = 1 << 0

// PackInput names either an in-memory blob or a file-backed blob. Source is
// empty for in-memory data. Callers retain ownership until Pack returns.
type PackInput struct {
	Path   string
	Data   []byte
	Source string
}

func BytesInput(path string, data []byte) PackInput {
	return PackInput{Path: path, Data: data}
}

func FileInput(path, source string) PackInput {
	return PackInput{Path: path, Source: source}
}

// PackStats describes the uncompressed resource blobs written to a package.
// Stored counts non-alias entries; aliases are counted in Deduplicated.
type PackStats struct {
	Files             int
	RawBytes          int64
	PackedBytes       int64
	Stored            int
	Deduplicated      int
	DeduplicatedBytes int64
}

type PackResult struct {
	Stats  PackStats
	Assets []AssetInfo
}

type packedEntry struct {
	input     PackInput
	asset     AssetInfo
	offset    uint64
	canonical int
	alias     bool
}

type blobKey struct {
	id   ContentID
	size int64
}

// PackProgress reports actual input inspection and writing work. Completed and
// Total include both passes, including deduplicated files. Active identifies
// the event before a potentially long operation; it clears on completion.
type PackProgress struct {
	Path      string
	Writing   bool
	Active    bool
	Completed int
	Total     int
}

// Pack writes the current indexed package format. It hashes file-backed inputs
// in a first pass, then streams only canonical blobs into the package.
func Pack(writer io.Writer, inputs []PackInput) (PackResult, error) {
	return PackWithProgress(writer, inputs, nil)
}

// PackWithProgress writes a package and reports each resource before and after
// its work, without retaining its contents or adding another input traversal.
func PackWithProgress(writer io.Writer, inputs []PackInput, progress func(PackProgress)) (PackResult, error) {
	if writer == nil {
		return PackResult{}, errors.New("resource: package writer is nil")
	}
	if uint64(len(inputs)) > uint64(^uint32(0)) {
		return PackResult{}, errors.New("resource: package contains too many files")
	}
	ordered := append([]PackInput(nil), inputs...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })
	for index, input := range ordered {
		if err := validateResourcePath(input.Path); err != nil {
			return PackResult{}, err
		}
		if index > 0 && input.Path == ordered[index-1].Path {
			return PackResult{}, fmt.Errorf("resource: duplicate package path %q", input.Path)
		}
		if input.Source != "" && input.Data != nil {
			return PackResult{}, fmt.Errorf("resource: package input %q has both data and a source file", input.Path)
		}
	}
	if err := validateSortedResourcePaths(ordered); err != nil {
		return PackResult{}, err
	}

	buffer := make([]byte, packBufferSize)
	entries := make([]packedEntry, len(ordered))
	canonical := make(map[blobKey]int, len(ordered))
	stats := PackStats{Files: len(ordered)}
	var dataSize uint64
	for index, input := range ordered {
		if progress != nil {
			progress(PackProgress{Path: input.Path, Completed: index, Active: true, Total: 2 * len(ordered)})
		}
		asset, err := inspectPackInput(input, buffer)
		if err != nil {
			return PackResult{}, fmt.Errorf("resource: inspect %s: %w", input.Path, err)
		}
		entry := packedEntry{input: input, asset: asset, offset: dataSize, canonical: index}
		key := blobKey{id: asset.ContentID, size: asset.Size}
		if previous, ok := canonical[key]; ok {
			entry.offset = entries[previous].offset
			entry.canonical = previous
			entry.alias = true
			stats.Deduplicated++
			stats.DeduplicatedBytes += asset.Size
		} else {
			if uint64(asset.Size) > ^uint64(0)-dataSize {
				return PackResult{}, errors.New("resource: package data size overflows uint64")
			}
			canonical[key] = index
			dataSize += uint64(asset.Size)
			stats.Stored++
		}
		stats.RawBytes += asset.Size
		entries[index] = entry
		if progress != nil {
			progress(PackProgress{Path: input.Path, Completed: index + 1, Total: 2 * len(ordered)})
		}
	}

	index := encodeIndex(entries)
	var header [headerSize]byte
	copy(header[:], formatMagic[:])
	binary.LittleEndian.PutUint16(header[8:10], FormatVersion)
	binary.LittleEndian.PutUint16(header[10:12], headerSize)
	binary.LittleEndian.PutUint32(header[12:16], uint32(len(entries)))
	binary.LittleEndian.PutUint64(header[16:24], uint64(len(index)))
	binary.LittleEndian.PutUint64(header[24:32], dataSize)
	if err := writeAll(writer, header[:]); err != nil {
		return PackResult{}, fmt.Errorf("resource: write package header: %w", err)
	}
	if err := writeAll(writer, index); err != nil {
		return PackResult{}, fmt.Errorf("resource: write package index: %w", err)
	}
	for index, entry := range entries {
		if progress != nil {
			progress(PackProgress{Path: entry.input.Path, Writing: true, Active: true, Completed: len(entries) + index, Total: 2 * len(entries)})
		}
		if entry.alias {
			if progress != nil {
				progress(PackProgress{Path: entry.input.Path, Writing: true, Completed: len(entries) + index + 1, Total: 2 * len(entries)})
			}
			continue
		}
		written, err := writePackInput(writer, entry.input, buffer)
		if err != nil {
			return PackResult{}, fmt.Errorf("resource: write %s: %w", entry.input.Path, err)
		}
		if written != entry.asset {
			return PackResult{}, fmt.Errorf("resource: source %q changed while packaging", entry.input.Path)
		}
		if progress != nil {
			progress(PackProgress{Path: entry.input.Path, Writing: true, Completed: len(entries) + index + 1, Total: 2 * len(entries)})
		}
	}
	stats.PackedBytes = int64(headerSize+len(index)) + int64(dataSize)
	assets := make([]AssetInfo, len(entries))
	for index := range entries {
		assets[index] = entries[index].asset
	}
	return PackResult{Stats: stats, Assets: assets}, nil
}

func inspectPackInput(input PackInput, buffer []byte) (AssetInfo, error) {
	if input.Source == "" {
		digest := sha256.Sum256(input.Data)
		return AssetInfo{
			Path: input.Path, Size: int64(len(input.Data)), Checksum: crc32.ChecksumIEEE(input.Data),
			ContentID: ContentID(digest[:ContentIDSize]),
		}, nil
	}
	file, err := os.Open(input.Source)
	if err != nil {
		return AssetInfo{}, err
	}
	defer file.Close()
	return inspectReader(input.Path, file, buffer)
}

func inspectReader(name string, reader io.Reader, buffer []byte) (AssetInfo, error) {
	hash := sha256.New()
	checksum := crc32.NewIEEE()
	var size int64
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			chunk := buffer[:count]
			_, _ = hash.Write(chunk)
			_, _ = checksum.Write(chunk)
			size += int64(count)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return AssetInfo{}, err
		}
		if count == 0 {
			return AssetInfo{}, io.ErrNoProgress
		}
	}
	var digest [sha256.Size]byte
	hash.Sum(digest[:0])
	return AssetInfo{
		Path: name, Size: size, Checksum: checksum.Sum32(),
		ContentID: ContentID(digest[:ContentIDSize]),
	}, nil
}

func writePackInput(writer io.Writer, input PackInput, buffer []byte) (AssetInfo, error) {
	if input.Source == "" {
		if err := writeAll(writer, input.Data); err != nil {
			return AssetInfo{}, err
		}
		return inspectPackInput(input, buffer)
	}
	file, err := os.Open(input.Source)
	if err != nil {
		return AssetInfo{}, err
	}
	defer file.Close()
	hash := sha256.New()
	checksum := crc32.NewIEEE()
	var size int64
	for {
		count, readErr := file.Read(buffer)
		if count > 0 {
			chunk := buffer[:count]
			if err := writeAll(writer, chunk); err != nil {
				return AssetInfo{}, err
			}
			_, _ = hash.Write(chunk)
			_, _ = checksum.Write(chunk)
			size += int64(count)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return AssetInfo{}, readErr
		}
		if count == 0 {
			return AssetInfo{}, io.ErrNoProgress
		}
	}
	var digest [sha256.Size]byte
	hash.Sum(digest[:0])
	return AssetInfo{
		Path: input.Path, Size: size, Checksum: checksum.Sum32(),
		ContentID: ContentID(digest[:ContentIDSize]),
	}, nil
}

func encodeIndex(entries []packedEntry) []byte {
	size := 0
	for _, entry := range entries {
		size += uvarintSize(entry.offset) + 1
	}
	index := make([]byte, 0, size)
	for _, entry := range entries {
		index = binary.AppendUvarint(index, entry.offset)
		flags := byte(0)
		if entry.alias {
			flags = entryAliasFlag
		}
		index = append(index, flags)
	}
	return index
}

func uvarintSize(value uint64) int {
	size := 1
	for value >= 1<<7 {
		value >>= 7
		size++
	}
	return size
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		count, err := writer.Write(data)
		if err != nil {
			return err
		}
		if count == 0 {
			return io.ErrShortWrite
		}
		data = data[count:]
	}
	return nil
}

func validateSortedResourcePaths(inputs []PackInput) error {
	for _, input := range inputs {
		name := input.Path
		for offset := 0; ; offset++ {
			slash := strings.IndexByte(name[offset:], '/')
			if slash < 0 {
				break
			}
			offset += slash
			parent := name[:offset]
			if _, exists := sort.Find(len(inputs), func(index int) int { return strings.Compare(parent, inputs[index].Path) }); exists {
				return fmt.Errorf("resource: package path %q is both a file and directory", parent)
			}
		}
	}
	return nil
}

type blobIdentity struct {
	size     int64
	checksum uint32
	id       ContentID
}

type byteViewer interface {
	View(offset, size int64) ([]byte, bool)
}

type Reader struct {
	mu         sync.RWMutex
	source     io.ReaderAt
	viewer     byteViewer
	dataOffset int64
	assets     []AssetInfo
	offsets    []uint64
	verified   []atomic.Bool
}

func NewReader(source io.ReaderAt, size int64, assets []AssetInfo) (*Reader, error) {
	if source == nil {
		return nil, errors.New("resource: package reader is nil")
	}
	if size < headerSize {
		return nil, errors.New("resource: package header is truncated")
	}
	var header [headerSize]byte
	if _, err := source.ReadAt(header[:], 0); err != nil {
		return nil, fmt.Errorf("resource: read package header: %w", err)
	}
	if !bytes.Equal(header[:8], formatMagic[:]) {
		return nil, errors.New("resource: invalid package format marker")
	}
	if version := binary.LittleEndian.Uint16(header[8:10]); version != FormatVersion {
		return nil, fmt.Errorf("resource: unsupported package version %d", version)
	}
	if declared := binary.LittleEndian.Uint16(header[10:12]); declared != headerSize {
		return nil, fmt.Errorf("resource: invalid package header size %d", declared)
	}
	count := binary.LittleEndian.Uint32(header[12:16])
	if uint64(count) != uint64(len(assets)) {
		return nil, fmt.Errorf("resource: package contains %d assets, catalog contains %d", count, len(assets))
	}
	indexSize := binary.LittleEndian.Uint64(header[16:24])
	dataSize := binary.LittleEndian.Uint64(header[24:32])
	if indexSize > uint64(size-headerSize) || dataSize > uint64(size-headerSize)-indexSize || uint64(headerSize)+indexSize+dataSize != uint64(size) {
		return nil, errors.New("resource: package size fields are invalid")
	}
	if indexSize > uint64(maxInt()) {
		return nil, errors.New("resource: package index is too large")
	}
	if uint64(count) > indexSize/minIndexEntrySize {
		return nil, errors.New("resource: package asset count exceeds index capacity")
	}
	indexData := make([]byte, int(indexSize))
	if _, err := source.ReadAt(indexData, headerSize); err != nil {
		return nil, fmt.Errorf("resource: read package index: %w", err)
	}
	offsets, err := decodeIndex(indexData, assets, dataSize)
	if err != nil {
		return nil, err
	}
	reader := &Reader{
		source: source, dataOffset: int64(headerSize) + int64(indexSize),
		assets: assets, offsets: offsets, verified: make([]atomic.Bool, len(assets)),
	}
	reader.viewer, _ = source.(byteViewer)
	return reader, nil
}

func decodeIndex(data []byte, assets []AssetInfo, dataSize uint64) ([]uint64, error) {
	offsets := make([]uint64, len(assets))
	canonical := make(map[blobIdentity]uint64, len(assets))
	var expectedOffset uint64
	for index, asset := range assets {
		offset, ok := consumeUvarint(&data)
		if !ok {
			return nil, fmt.Errorf("resource: invalid package asset %q offset", asset.Path)
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("resource: package asset %q flags are truncated", asset.Path)
		}
		flags := data[0]
		data = data[1:]
		if flags&^byte(entryAliasFlag) != 0 {
			return nil, fmt.Errorf("resource: unsupported package asset flags 0x%x", flags)
		}
		if offset > dataSize || uint64(asset.Size) > dataSize-offset {
			return nil, fmt.Errorf("resource: package asset %q has invalid data bounds", asset.Path)
		}
		identity := blobIdentity{size: asset.Size, checksum: asset.Checksum, id: asset.ContentID}
		if flags&entryAliasFlag != 0 {
			canonicalOffset, exists := canonical[identity]
			if !exists || canonicalOffset != offset {
				return nil, fmt.Errorf("resource: package asset %q does not match its canonical blob", asset.Path)
			}
		} else {
			if offset != expectedOffset {
				return nil, fmt.Errorf("resource: package asset %q has invalid data offset", asset.Path)
			}
			if _, exists := canonical[identity]; exists {
				return nil, fmt.Errorf("resource: package asset %q duplicates a canonical blob", asset.Path)
			}
			canonical[identity] = offset
			expectedOffset += uint64(asset.Size)
		}
		offsets[index] = offset
	}
	if len(data) != 0 || expectedOffset != dataSize {
		return nil, errors.New("resource: package index length does not match its contents")
	}
	return offsets, nil
}

func consumeUvarint(data *[]byte) (uint64, bool) {
	value, size := binary.Uvarint(*data)
	if size <= 0 {
		return 0, false
	}
	*data = (*data)[size:]
	return value, true
}

func validateResourcePath(name string) error {
	if name == "." || !iofs.ValidPath(name) || strings.ContainsRune(name, '\\') {
		return fmt.Errorf("resource: invalid package path %q", name)
	}
	return nil
}

func maxInt() int {
	return int(^uint(0) >> 1)
}

func (r *Reader) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	r.source = nil
	r.viewer = nil
	r.mu.Unlock()
	return nil
}

func (r *Reader) ReadFile(name string) ([]byte, error) {
	index, ok := r.assetIndex(name)
	if !ok {
		return nil, &iofs.PathError{Op: "read", Path: name, Err: iofs.ErrNotExist}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.source == nil {
		return nil, &iofs.PathError{Op: "read", Path: name, Err: iofs.ErrClosed}
	}
	asset := r.assets[index]
	if asset.Size > int64(maxInt()) {
		return nil, &iofs.PathError{Op: "read", Path: name, Err: errors.New("resource is too large")}
	}
	data := make([]byte, int(asset.Size))
	if err := readAtFull(r.source, data, r.dataOffset+int64(r.offsets[index])); err != nil {
		return nil, &iofs.PathError{Op: "read", Path: name, Err: err}
	}
	if !r.verified[index].Load() {
		if err := validateAssetData(asset, data); err != nil {
			return nil, &iofs.PathError{Op: "read", Path: name, Err: err}
		}
		r.verified[index].Store(true)
	}
	return data, nil
}

func readAtFull(source io.ReaderAt, data []byte, offset int64) error {
	if len(data) == 0 {
		return nil
	}
	count, err := source.ReadAt(data, offset)
	if count != len(data) {
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		return err
	}
	return nil
}

func validateAssetData(asset AssetInfo, data []byte) error {
	if int64(len(data)) != asset.Size {
		return errors.New("resource size mismatch")
	}
	if crc32.ChecksumIEEE(data) != asset.Checksum {
		return errors.New("resource checksum mismatch")
	}
	digest := sha256.Sum256(data)
	if ContentID(digest[:ContentIDSize]) != asset.ContentID {
		return errors.New("resource content ID mismatch")
	}
	return nil
}

func (r *Reader) assetIndex(name string) (int, bool) {
	if r == nil || validateReadPath(name) != nil || name == "." {
		return 0, false
	}
	return sort.Find(len(r.assets), func(index int) int { return strings.Compare(name, r.assets[index].Path) })
}

func (r *Reader) blob(index int, release blobReleaser) *Blob {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.source == nil || index < 0 || index >= len(r.assets) {
		return nil
	}
	return &Blob{
		source: r.source, viewer: r.viewer, offset: r.dataOffset + int64(r.offsets[index]),
		info: r.assets[index], verified: &r.verified[index], release: release,
	}
}

func (r *Reader) OpenBlob(name string) (*Blob, error) {
	index, ok := r.assetIndex(name)
	if !ok {
		return nil, &iofs.PathError{Op: "open", Path: name, Err: iofs.ErrNotExist}
	}
	blob := r.blob(index, nil)
	if blob == nil {
		return nil, &iofs.PathError{Op: "open", Path: name, Err: iofs.ErrClosed}
	}
	return blob, nil
}

func (r *Reader) Open(name string) (iofs.File, error) {
	if err := validateReadPath(name); err != nil {
		return nil, &iofs.PathError{Op: "open", Path: name, Err: err}
	}
	return openMemoryFile(r, name)
}

func (r *Reader) Stat(name string) (iofs.FileInfo, error) {
	if err := validateReadPath(name); err != nil {
		return nil, &iofs.PathError{Op: "stat", Path: name, Err: err}
	}
	if index, ok := r.assetIndex(name); ok {
		return fileInfo{name: path.Base(name), size: r.assets[index].Size, mode: 0o444}, nil
	}
	if r.hasDirectory(name) {
		return fileInfo{name: path.Base(name), mode: iofs.ModeDir | 0o555}, nil
	}
	return nil, &iofs.PathError{Op: "stat", Path: name, Err: iofs.ErrNotExist}
}

func (r *Reader) hasDirectory(name string) bool {
	if name == "." {
		return true
	}
	prefix := name + "/"
	index, _ := sort.Find(len(r.assets), func(index int) int {
		if r.assets[index].Path < prefix {
			return 1
		}
		return -1
	})
	return index < len(r.assets) && strings.HasPrefix(r.assets[index].Path, prefix)
}

func (r *Reader) ReadDir(name string) ([]iofs.DirEntry, error) {
	if err := validateReadPath(name); err != nil {
		return nil, &iofs.PathError{Op: "readdir", Path: name, Err: err}
	}
	if !r.hasDirectory(name) {
		return nil, &iofs.PathError{Op: "readdir", Path: name, Err: iofs.ErrNotExist}
	}
	return directoryEntries(r.assets, name), nil
}

func directoryEntries(assets []AssetInfo, name string) []iofs.DirEntry {
	prefix := ""
	if name != "." {
		prefix = name + "/"
	}
	result := make([]iofs.DirEntry, 0)
	previous := ""
	for _, asset := range assets {
		if !strings.HasPrefix(asset.Path, prefix) {
			continue
		}
		rest := strings.TrimPrefix(asset.Path, prefix)
		if rest == "" {
			continue
		}
		base, suffix, found := strings.Cut(rest, "/")
		if base == previous {
			continue
		}
		previous = base
		if found && suffix != "" {
			result = append(result, dirEntry{info: fileInfo{name: base, mode: iofs.ModeDir | 0o555}})
		} else {
			result = append(result, dirEntry{info: fileInfo{name: base, size: asset.Size, mode: 0o444}})
		}
	}
	return result
}

func validateReadPath(name string) error {
	if name == "." {
		return nil
	}
	if !iofs.ValidPath(name) || strings.ContainsRune(name, '\\') {
		return iofs.ErrInvalid
	}
	return nil
}

type blobReleaser interface {
	releaseBlob(int64) error
}

// Blob is a pinned, immutable view of one encoded resource. Bytes may return a
// direct package view; callers must not mutate it. Close releases the package
// pin and invalidates all views obtained from the Blob.
type Blob struct {
	source   io.ReaderAt
	viewer   byteViewer
	offset   int64
	info     AssetInfo
	verified *atomic.Bool
	release  blobReleaser

	// identity supplies checksum and content ID on first observation. Mapped
	// blobs leave it set so a resource that is only decoded is never hashed.
	identity *lazyAssetIdentity

	loadOnce   sync.Once
	verifyOnce sync.Once
	data       []byte
	loadErr    error
	verifyErr  error
	closed     atomic.Bool
}

// NewMemoryBlob takes ownership of data and returns an already verified Blob.
func NewMemoryBlob(name string, data []byte) *Blob {
	asset := AssetInfo{
		Path: name, Size: int64(len(data)), Checksum: crc32.ChecksumIEEE(data), ContentID: contentID(data),
	}
	source := newMemorySource(data)
	verified := new(atomic.Bool)
	verified.Store(true)
	return &Blob{source: source, viewer: source, info: asset, verified: verified}
}

func (b *Blob) Info() AssetInfo {
	if b == nil {
		return AssetInfo{}
	}
	return b.assetInfo()
}

func (b *Blob) assetInfo() AssetInfo {
	if b.identity != nil {
		return b.identity.resolve()
	}
	return b.info
}

func (b *Blob) Size() int64 {
	if b == nil {
		return 0
	}
	return b.info.Size
}

func (b *Blob) ContentID() ContentID {
	if b == nil {
		return ContentID{}
	}
	return b.assetInfo().ContentID
}

func (b *Blob) Bytes() ([]byte, error) {
	if b == nil || b.closed.Load() || b.source == nil {
		return nil, iofs.ErrClosed
	}
	b.loadOnce.Do(func() {
		if b.viewer != nil {
			if data, ok := b.viewer.View(b.offset, b.info.Size); ok {
				b.data = data
				b.loadErr = b.validateData(data)
				return
			}
		}
		if b.info.Size > int64(maxInt()) {
			b.loadErr = errors.New("resource is too large")
			return
		}
		b.data = make([]byte, int(b.info.Size))
		if b.loadErr = readAtFull(b.source, b.data, b.offset); b.loadErr == nil {
			b.loadErr = b.validateData(b.data)
		}
	})
	return b.data, b.loadErr
}

// View returns the resource bytes without copying when the backing store can
// expose them directly. Callers must not mutate the result and must keep the
// Blob open for as long as they use it. The boolean reports whether a direct
// view exists; otherwise callers fall back to Bytes or ReadAll.
func (b *Blob) View() ([]byte, bool) {
	if b == nil || b.closed.Load() || b.viewer == nil {
		return nil, false
	}
	return b.viewer.View(b.offset, b.info.Size)
}

// ReadAll returns an owned copy of the resource. Unlike Bytes, the result may
// be mutated and remains valid after Close.
func (b *Blob) ReadAll() ([]byte, error) {
	if b == nil || b.closed.Load() || b.source == nil {
		return nil, iofs.ErrClosed
	}
	if b.info.Size > int64(maxInt()) {
		return nil, errors.New("resource is too large")
	}
	data := make([]byte, int(b.info.Size))
	if err := readAtFull(b.source, data, b.offset); err != nil {
		return nil, err
	}
	if b.verified == nil || !b.verified.Load() {
		if err := b.validateData(data); err != nil {
			return nil, err
		}
	}
	return data, nil
}

func (b *Blob) Validate() error {
	if b == nil || b.closed.Load() || b.source == nil {
		return iofs.ErrClosed
	}
	if b.verified != nil && b.verified.Load() {
		return nil
	}
	b.verifyOnce.Do(func() {
		if b.viewer != nil {
			if data, ok := b.viewer.View(b.offset, b.info.Size); ok {
				b.verifyErr = b.validateData(data)
				return
			}
		}
		b.verifyErr = validateAssetReader(b.source, b.offset, b.info)
		if b.verifyErr == nil && b.verified != nil {
			b.verified.Store(true)
		}
	})
	return b.verifyErr
}

// ReleaseResidentPages lets a mapped package discard fully covered file pages
// after a resource has been decoded. The bytes remain available on next access.
func (b *Blob) ReleaseResidentPages() error {
	if b == nil || b.closed.Load() || b.source == nil {
		return iofs.ErrClosed
	}
	if source, ok := b.source.(interface{ releaseResidentPages(int64, int64) error }); ok {
		return source.releaseResidentPages(b.offset, b.info.Size)
	}
	return nil
}

func (b *Blob) validateData(data []byte) error {
	if b.verified != nil && b.verified.Load() {
		return nil
	}
	err := validateAssetData(b.info, data)
	if err == nil && b.verified != nil {
		b.verified.Store(true)
	}
	return err
}

func (b *Blob) Open() (io.ReadSeekCloser, error) {
	if err := b.Validate(); err != nil {
		return nil, err
	}
	if b.viewer != nil {
		if data, ok := b.viewer.View(b.offset, b.info.Size); ok {
			return readSeekNopCloser{ReadSeeker: bytes.NewReader(data)}, nil
		}
	}
	return readSeekNopCloser{ReadSeeker: io.NewSectionReader(b.source, b.offset, b.info.Size)}, nil
}

func (b *Blob) Close() error {
	if b == nil || b.closed.Swap(true) {
		return nil
	}
	b.source = nil
	b.viewer = nil
	b.data = nil
	if b.release != nil {
		return b.release.releaseBlob(b.info.Size)
	}
	return nil
}

type readSeekNopCloser struct{ io.ReadSeeker }

func (readSeekNopCloser) Close() error { return nil }

var verifyBufferPool = sync.Pool{New: func() any { return make([]byte, packBufferSize) }}

func validateAssetReader(source io.ReaderAt, offset int64, asset AssetInfo) error {
	buffer := verifyBufferPool.Get().([]byte)
	defer verifyBufferPool.Put(buffer)
	hash := sha256.New()
	checksum := crc32.NewIEEE()
	remaining := asset.Size
	for remaining > 0 {
		chunk := buffer
		if int64(len(chunk)) > remaining {
			chunk = chunk[:remaining]
		}
		if err := readAtFull(source, chunk, offset); err != nil {
			return err
		}
		_, _ = hash.Write(chunk)
		_, _ = checksum.Write(chunk)
		offset += int64(len(chunk))
		remaining -= int64(len(chunk))
	}
	if checksum.Sum32() != asset.Checksum {
		return errors.New("resource checksum mismatch")
	}
	var digest [sha256.Size]byte
	hash.Sum(digest[:0])
	if ContentID(digest[:ContentIDSize]) != asset.ContentID {
		return errors.New("resource content ID mismatch")
	}
	return nil
}

type memorySource struct{ data []byte }

func newMemorySource(data []byte) *memorySource { return &memorySource{data: data} }

func (s *memorySource) ReadAt(target []byte, offset int64) (int, error) {
	if offset < 0 || offset > int64(len(s.data)) {
		return 0, io.EOF
	}
	count := copy(target, s.data[offset:])
	if count != len(target) {
		return count, io.EOF
	}
	return count, nil
}

func (s *memorySource) View(offset, size int64) ([]byte, bool) {
	if offset < 0 || size < 0 || offset > int64(len(s.data)) || size > int64(len(s.data))-offset {
		return nil, false
	}
	return s.data[offset : offset+size], true
}

type fileInfo struct {
	name string
	size int64
	mode iofs.FileMode
}

func (i fileInfo) Name() string        { return i.name }
func (i fileInfo) Size() int64         { return i.size }
func (i fileInfo) Mode() iofs.FileMode { return i.mode }
func (i fileInfo) ModTime() time.Time  { return time.Time{} }
func (i fileInfo) IsDir() bool         { return i.mode.IsDir() }
func (i fileInfo) Sys() any            { return nil }

type dirEntry struct{ info fileInfo }

func (e dirEntry) Name() string                 { return e.info.Name() }
func (e dirEntry) IsDir() bool                  { return e.info.IsDir() }
func (e dirEntry) Type() iofs.FileMode          { return e.info.Mode().Type() }
func (e dirEntry) Info() (iofs.FileInfo, error) { return e.info, nil }

type memoryFile struct {
	*bytes.Reader
	info     iofs.FileInfo
	entries  []iofs.DirEntry
	position int
}

type memoryFileSource interface {
	Stat(string) (iofs.FileInfo, error)
	ReadDir(string) ([]iofs.DirEntry, error)
	ReadFile(string) ([]byte, error)
}

func openMemoryFile[S memoryFileSource](source S, name string) (iofs.File, error) {
	info, err := source.Stat(name)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		entries, err := source.ReadDir(name)
		if err != nil {
			return nil, err
		}
		return &memoryFile{Reader: bytes.NewReader(nil), info: info, entries: entries}, nil
	}
	data, err := source.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return &memoryFile{Reader: bytes.NewReader(data), info: info}, nil
}

func (f *memoryFile) Close() error                 { return nil }
func (f *memoryFile) Stat() (iofs.FileInfo, error) { return f.info, nil }

func (f *memoryFile) ReadDir(count int) ([]iofs.DirEntry, error) {
	if !f.info.IsDir() {
		return nil, &iofs.PathError{Op: "readdir", Path: f.info.Name(), Err: errors.New("not a directory")}
	}
	if f.position >= len(f.entries) {
		if count > 0 {
			return nil, io.EOF
		}
		return []iofs.DirEntry{}, nil
	}
	end := len(f.entries)
	if count > 0 && f.position+count < end {
		end = f.position + count
	}
	result := f.entries[f.position:end]
	f.position = end
	return result, nil
}
