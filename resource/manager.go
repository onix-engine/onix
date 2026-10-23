package resource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrPackageNotLoaded = errors.New("resource package is not loaded")
	ErrPackageInUse     = errors.New("resource package is in use")
	ErrRequiredPackage  = errors.New("required resource package cannot be unloaded")
)

type PackageNotLoadedError struct {
	Package string
	Path    string
}

func (e *PackageNotLoadedError) Error() string {
	return fmt.Sprintf("resource: package %q is not loaded for %q", e.Package, e.Path)
}

func (e *PackageNotLoadedError) Unwrap() error { return ErrPackageNotLoaded }

type PackageSource struct {
	ReaderAt io.ReaderAt
	Size     int64
	Closer   io.Closer
}

// PackageLoader opens one package. It runs without the Manager mutex held and
// must honor cancellation whenever its backing transport supports it.
type PackageLoader func(context.Context, PackageInfo) (PackageSource, error)

type packageStatus uint8

const (
	packageUnloaded packageStatus = iota
	packageLoading
	packageLoaded
)

type packageState struct {
	info       PackageInfo
	status     packageStatus
	wait       chan struct{}
	mounted    *mountedPackage
	explicit   bool
	dependents int
}

type mountedPackage struct {
	manager      *Manager
	name         string
	reader       *Reader
	closer       io.Closer
	pins         int
	pendingClose bool
	closed       bool
}

type ownedAsset struct {
	info         AssetInfo
	packageName  string
	packageIndex int
}

type Manager struct {
	mu       sync.Mutex
	catalog  Catalog
	packages map[string]*packageState
	assets   []ownedAsset
	loader   PackageLoader
	closed   bool

	loadedPackages int
	residentBytes  int64
	pinnedBlobs    int
	pinnedBytes    int64
	loads          atomic.Uint64
	unloads        atomic.Uint64
	blobAcquires   atomic.Uint64
	loadNanos      atomic.Uint64
}

type ManagerStatistics struct {
	LoadedPackages         int
	ResidentPackageBytes   int64
	PinnedBlobs            int
	PinnedBytes            int64
	PackageLoads           uint64
	PackageUnloads         uint64
	BlobAcquires           uint64
	PackageLoadNanoseconds uint64
}

func NewManager(catalog Catalog, loader PackageLoader) (*Manager, error) {
	if err := catalog.Validate(); err != nil {
		return nil, err
	}
	if loader == nil {
		return nil, errors.New("resource: package loader is nil")
	}
	catalog = cloneCatalog(catalog)
	manager := &Manager{
		catalog: catalog, packages: make(map[string]*packageState, len(catalog.Packages)), loader: loader,
	}
	assetCount := 0
	for index := range catalog.Packages {
		info := catalog.Packages[index]
		manager.packages[info.Name] = &packageState{info: info}
		assetCount += len(info.Assets)
	}
	manager.assets = make([]ownedAsset, 0, assetCount)
	for _, info := range catalog.Packages {
		for index, asset := range info.Assets {
			manager.assets = append(manager.assets, ownedAsset{info: asset, packageName: info.Name, packageIndex: index})
		}
	}
	sort.Slice(manager.assets, func(i, j int) bool { return manager.assets[i].info.Path < manager.assets[j].info.Path })
	return manager, nil
}

func OpenManager(directory string) (*Manager, error) {
	data, err := os.ReadFile(filepath.Join(directory, CatalogFile))
	if err != nil {
		return nil, fmt.Errorf("resource: read catalog: %w", err)
	}
	catalog, err := ParseCatalog(data)
	if err != nil {
		return nil, err
	}
	manager, err := NewManager(catalog, func(ctx context.Context, info PackageInfo) (PackageSource, error) {
		if err := ctx.Err(); err != nil {
			return PackageSource{}, err
		}
		return openPackageFile(filepath.Join(directory, filepath.FromSlash(info.File)))
	})
	if err != nil {
		return nil, err
	}
	if err := manager.LoadRequiredContext(context.Background()); err != nil {
		_ = manager.Close()
		return nil, err
	}
	return manager, nil
}

// NewMemoryManager takes ownership of every package byte slice in packages.
// The slices must not be mutated for the lifetime of the Manager.
func NewMemoryManager(catalogData []byte, packages map[string][]byte) (*Manager, error) {
	catalog, err := ParseCatalog(catalogData)
	if err != nil {
		return nil, err
	}
	manager, err := NewManager(catalog, func(ctx context.Context, info PackageInfo) (PackageSource, error) {
		if err := ctx.Err(); err != nil {
			return PackageSource{}, err
		}
		data, ok := packages[info.File]
		if !ok {
			return PackageSource{}, fmt.Errorf("resource: package data %q is unavailable", info.File)
		}
		return PackageSource{ReaderAt: newMemorySource(data), Size: int64(len(data))}, nil
	})
	if err != nil {
		return nil, err
	}
	if err := manager.LoadRequiredContext(context.Background()); err != nil {
		_ = manager.Close()
		return nil, err
	}
	return manager, nil
}

func cloneCatalog(catalog Catalog) Catalog {
	result := Catalog{Version: catalog.Version, Packages: make([]PackageInfo, len(catalog.Packages))}
	for index, info := range catalog.Packages {
		info.Dependencies = append([]string(nil), info.Dependencies...)
		info.Assets = append([]AssetInfo(nil), info.Assets...)
		result.Packages[index] = info
	}
	return result
}

func (m *Manager) Catalog() Catalog {
	if m == nil {
		return Catalog{}
	}
	m.mu.Lock()
	result := cloneCatalog(m.catalog)
	m.mu.Unlock()
	return result
}

func (m *Manager) Statistics() ManagerStatistics {
	if m == nil {
		return ManagerStatistics{}
	}
	m.mu.Lock()
	statistics := ManagerStatistics{
		LoadedPackages: m.loadedPackages, ResidentPackageBytes: m.residentBytes,
		PinnedBlobs: m.pinnedBlobs, PinnedBytes: m.pinnedBytes,
	}
	m.mu.Unlock()
	statistics.PackageLoads = m.loads.Load()
	statistics.PackageUnloads = m.unloads.Load()
	statistics.BlobAcquires = m.blobAcquires.Load()
	statistics.PackageLoadNanoseconds = m.loadNanos.Load()
	return statistics
}

func (m *Manager) LoadRequired() error {
	return m.LoadRequiredContext(context.Background())
}

func (m *Manager) LoadRequiredContext(ctx context.Context) error {
	if m == nil {
		return fs.ErrClosed
	}
	for _, info := range m.catalog.Packages {
		if info.Required {
			if err := m.loadContext(ctx, info.Name, false, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Manager) Load(name string) error {
	return m.LoadContext(context.Background(), name)
}

func (m *Manager) LoadContext(ctx context.Context, name string) error {
	return m.loadContext(ctx, name, true, nil)
}

// MountBytes takes ownership of data and mounts it without copying. The caller
// must not mutate data after this method is called.
func (m *Manager) MountBytes(name string, data []byte) error {
	return m.MountBytesContext(context.Background(), name, data)
}

func (m *Manager) MountBytesContext(ctx context.Context, name string, data []byte) error {
	source := PackageSource{ReaderAt: newMemorySource(data), Size: int64(len(data))}
	return m.loadContext(ctx, name, true, &source)
}

func (m *Manager) loadContext(ctx context.Context, name string, explicit bool, supplied *PackageSource) error {
	if m == nil {
		return fs.ErrClosed
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return fs.ErrClosed
		}
		state, ok := m.packages[name]
		if !ok {
			m.mu.Unlock()
			return fmt.Errorf("resource: package %q is not in the catalog", name)
		}
		switch state.status {
		case packageLoaded:
			if explicit {
				state.explicit = true
			}
			m.mu.Unlock()
			return nil
		case packageLoading:
			wait := state.wait
			m.mu.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-wait:
				continue
			}
		}
		state.status = packageLoading
		state.wait = make(chan struct{})
		info := state.info
		m.mu.Unlock()

		started := time.Now()
		dependencies := make([]string, 0, len(info.Dependencies))
		var loadErr error
		for _, dependency := range info.Dependencies {
			if loadErr = m.retainDependency(ctx, dependency); loadErr != nil {
				loadErr = fmt.Errorf("resource: load package %q dependency %q: %w", name, dependency, loadErr)
				break
			}
			dependencies = append(dependencies, dependency)
		}
		var mounted *mountedPackage
		if loadErr == nil {
			var source PackageSource
			if supplied != nil {
				source = *supplied
			} else {
				source, loadErr = m.loader(ctx, info)
				if loadErr != nil {
					loadErr = fmt.Errorf("resource: open package %q: %w", name, loadErr)
				}
			}
			if loadErr == nil {
				mounted, loadErr = openMountedPackage(m, info, source)
			}
		}
		m.loadNanos.Add(uint64(max(time.Since(started), 0)))

		m.mu.Lock()
		state = m.packages[name]
		wait := state.wait
		if loadErr == nil && m.closed {
			loadErr = fs.ErrClosed
		}
		if loadErr == nil {
			state.status = packageLoaded
			state.mounted = mounted
			state.explicit = state.explicit || explicit
			m.loadedPackages++
			m.residentBytes += info.Size
			m.loads.Add(1)
		} else {
			state.status = packageUnloaded
		}
		state.wait = nil
		close(wait)
		m.mu.Unlock()

		if loadErr != nil {
			if mounted != nil {
				_ = closeMountedPackage(mounted)
			}
			m.releaseDependencies(dependencies)
			return loadErr
		}
		return nil
	}
}

func openMountedPackage(manager *Manager, info PackageInfo, source PackageSource) (*mountedPackage, error) {
	closeSource := func() {
		if source.Closer != nil {
			_ = source.Closer.Close()
		}
	}
	if source.ReaderAt == nil || source.Size != info.Size {
		closeSource()
		return nil, fmt.Errorf("resource: package %q size is %d, want %d", info.Name, source.Size, info.Size)
	}
	reader, err := NewReader(source.ReaderAt, source.Size, info.Assets)
	if err != nil {
		closeSource()
		return nil, fmt.Errorf("resource: open package %q: %w", info.Name, err)
	}
	return &mountedPackage{manager: manager, name: info.Name, reader: reader, closer: source.Closer}, nil
}

func (m *Manager) retainDependency(ctx context.Context, name string) error {
	for {
		if err := m.loadContext(ctx, name, false, nil); err != nil {
			return err
		}
		m.mu.Lock()
		state := m.packages[name]
		if state.status == packageLoaded {
			state.dependents++
			m.mu.Unlock()
			return nil
		}
		m.mu.Unlock()
	}
}

func (m *Manager) releaseDependencies(names []string) {
	var closing []*mountedPackage
	m.mu.Lock()
	for index := len(names) - 1; index >= 0; index-- {
		state := m.packages[names[index]]
		if state.dependents > 0 {
			state.dependents--
		}
		closing = append(closing, m.pruneLocked(state)...)
	}
	m.mu.Unlock()
	for _, mounted := range closing {
		_ = closeMountedPackage(mounted)
	}
}

func (m *Manager) Unload(name string) error {
	if m == nil {
		return fs.ErrClosed
	}
	for {
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return fs.ErrClosed
		}
		state, ok := m.packages[name]
		if !ok {
			m.mu.Unlock()
			return fmt.Errorf("resource: package %q is not in the catalog", name)
		}
		if state.info.Required {
			m.mu.Unlock()
			return fmt.Errorf("resource: package %q: %w", name, ErrRequiredPackage)
		}
		if state.status == packageLoading {
			wait := state.wait
			m.mu.Unlock()
			<-wait
			continue
		}
		if state.status != packageLoaded {
			m.mu.Unlock()
			return nil
		}
		if state.dependents != 0 || state.mounted.pins != 0 {
			m.mu.Unlock()
			return fmt.Errorf("resource: package %q: %w", name, ErrPackageInUse)
		}
		state.explicit = false
		closing := m.pruneLocked(state)
		m.mu.Unlock()
		return closeMountedPackages(closing)
	}
}

func (m *Manager) pruneLocked(initial *packageState) []*mountedPackage {
	stack := []*packageState{initial}
	closing := make([]*mountedPackage, 0, 1)
	for len(stack) > 0 {
		state := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if state == nil || state.status != packageLoaded || state.info.Required || state.explicit || state.dependents != 0 || state.mounted.pins != 0 {
			continue
		}
		mounted := state.mounted
		state.status = packageUnloaded
		state.mounted = nil
		m.loadedPackages--
		m.residentBytes -= state.info.Size
		m.unloads.Add(1)
		closing = append(closing, mounted)
		for _, name := range state.info.Dependencies {
			dependency := m.packages[name]
			if dependency.dependents > 0 {
				dependency.dependents--
			}
			stack = append(stack, dependency)
		}
	}
	return closing
}

func closeMountedPackages(packages []*mountedPackage) error {
	var result error
	for _, mounted := range packages {
		result = errors.Join(result, closeMountedPackage(mounted))
	}
	return result
}

func closeMountedPackage(mounted *mountedPackage) error {
	if mounted == nil || mounted.closed {
		return nil
	}
	mounted.closed = true
	var result error
	if mounted.reader != nil {
		result = errors.Join(result, mounted.reader.Close())
	}
	if mounted.closer != nil {
		result = errors.Join(result, mounted.closer.Close())
	}
	return result
}

func (m *Manager) Loaded(name string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	state := m.packages[name]
	loaded := state != nil && state.status == packageLoaded
	m.mu.Unlock()
	return loaded
}

func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	var closing []*mountedPackage
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	for _, state := range m.packages {
		if state.status != packageLoaded || state.mounted == nil {
			continue
		}
		mounted := state.mounted
		state.status = packageUnloaded
		state.mounted = nil
		if mounted.pins == 0 {
			closing = append(closing, mounted)
		} else {
			mounted.pendingClose = true
		}
	}
	m.loadedPackages = 0
	m.residentBytes = 0
	m.mu.Unlock()
	return closeMountedPackages(closing)
}

func (m *Manager) findAsset(name string) (ownedAsset, bool) {
	index, ok := sort.Find(len(m.assets), func(index int) int { return strings.Compare(name, m.assets[index].info.Path) })
	if !ok {
		return ownedAsset{}, false
	}
	return m.assets[index], true
}

func (m *Manager) OpenBlob(name string) (*Blob, error) {
	if err := validateReadPath(name); err != nil || name == "." {
		if err == nil {
			err = errors.New("is a directory")
		}
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	owner, ok := m.findAsset(name)
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrClosed}
	}
	state := m.packages[owner.packageName]
	if state.status != packageLoaded || state.mounted == nil {
		m.mu.Unlock()
		return nil, &fs.PathError{Op: "open", Path: name, Err: &PackageNotLoadedError{Package: owner.packageName, Path: name}}
	}
	mounted := state.mounted
	mounted.pins++
	m.pinnedBlobs++
	m.pinnedBytes += owner.info.Size
	m.blobAcquires.Add(1)
	blob := mounted.reader.blob(owner.packageIndex, mounted)
	if blob == nil {
		mounted.pins--
		m.pinnedBlobs--
		m.pinnedBytes -= owner.info.Size
		m.mu.Unlock()
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrClosed}
	}
	m.mu.Unlock()
	return blob, nil
}

func (mounted *mountedPackage) releaseBlob(size int64) error {
	if mounted == nil || mounted.manager == nil {
		return nil
	}
	manager := mounted.manager
	manager.mu.Lock()
	if mounted.pins > 0 {
		mounted.pins--
		manager.pinnedBlobs--
		manager.pinnedBytes -= size
	}
	closeNow := mounted.pendingClose && mounted.pins == 0 && !mounted.closed
	manager.mu.Unlock()
	if closeNow {
		return closeMountedPackage(mounted)
	}
	return nil
}

func (m *Manager) ReadFile(name string) ([]byte, error) {
	blob, err := m.OpenBlob(name)
	if err != nil {
		return nil, err
	}
	defer blob.Close()
	return blob.ReadAll()
}

func (m *Manager) Open(name string) (fs.File, error) {
	if err := validateReadPath(name); err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	return openMemoryFile(m, name)
}

func (m *Manager) Stat(name string) (fs.FileInfo, error) {
	if err := validateReadPath(name); err != nil {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: err}
	}
	if m.hasDirectory(name) {
		return fileInfo{name: path.Base(name), mode: fs.ModeDir | 0o555}, nil
	}
	owner, ok := m.findAsset(name)
	if !ok {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrClosed}
	}
	state := m.packages[owner.packageName]
	if state.status != packageLoaded {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: &PackageNotLoadedError{Package: owner.packageName, Path: name}}
	}
	return fileInfo{name: path.Base(name), size: owner.info.Size, mode: 0o444}, nil
}

func (m *Manager) hasDirectory(name string) bool {
	if name == "." {
		return true
	}
	prefix := name + "/"
	index := sort.Search(len(m.assets), func(index int) bool { return m.assets[index].info.Path >= prefix })
	return index < len(m.assets) && strings.HasPrefix(m.assets[index].info.Path, prefix)
}

func (m *Manager) ReadDir(name string) ([]fs.DirEntry, error) {
	if err := validateReadPath(name); err != nil {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: err}
	}
	if !m.hasDirectory(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	prefix := ""
	if name != "." {
		prefix = name + "/"
	}
	entries := make([]fs.DirEntry, 0)
	previous := ""
	for _, owner := range m.assets {
		if !strings.HasPrefix(owner.info.Path, prefix) {
			continue
		}
		rest := strings.TrimPrefix(owner.info.Path, prefix)
		if rest == "" {
			continue
		}
		base, suffix, found := strings.Cut(rest, "/")
		if base == previous {
			continue
		}
		previous = base
		if found && suffix != "" {
			entries = append(entries, dirEntry{info: fileInfo{name: base, mode: fs.ModeDir | 0o555}})
		} else {
			entries = append(entries, dirEntry{info: fileInfo{name: base, size: owner.info.Size, mode: 0o444}})
		}
	}
	return entries, nil
}
