//go:build js && wasm && !wechat

package fs

import (
	"errors"
	"fmt"
	iofs "io/fs"
	"path"
	"sort"
	"strings"
	"sync"
	"syscall/js"
	"time"
)

const (
	indexedDBName    = "onix-filesystem"
	indexedDBVersion = 1
	indexedDBStore   = "entries"
)

type webBackend struct {
	root string
	mu   sync.Mutex
	db   js.Value
}

type webRecord struct {
	path      string
	directory bool
	data      []byte
	size      int64
	modified  int64
}

func platformDefaultRoot() (string, error) {
	return ".", nil
}

func newPlatformBackend(root string) (backend, error) {
	if !iofs.ValidPath(root) || strings.ContainsRune(root, '\\') {
		return nil, fmt.Errorf("invalid virtual filesystem root %q", root)
	}
	return &webBackend{root: root}, nil
}

func (b *webBackend) Open(name string) (iofs.File, error) {
	info, err := b.Stat(name)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return newMemoryFile(nil, info), nil
	}
	data, err := b.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return newMemoryFile(data, info), nil
}

func (b *webBackend) ReadFile(name string) ([]byte, error) {
	record, found, err := b.record(name)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNotExist
	}
	if record.directory {
		return nil, ErrInvalid
	}
	return append([]byte(nil), record.data...), nil
}

func (b *webBackend) WriteFile(name string, data []byte) error {
	records, err := b.records()
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.path == name && record.directory {
			return ErrInvalid
		}
	}
	if err := validateWebDirectories(records, path.Dir(name)); err != nil {
		return err
	}
	db, err := b.database()
	if err != nil {
		return err
	}
	return runTransaction(db, func(store js.Value) {
		b.putParents(store, path.Dir(name))
		store.Call("put", b.recordValue(webRecord{
			path:     name,
			data:     append([]byte(nil), data...),
			size:     int64(len(data)),
			modified: time.Now().UnixMilli(),
		}))
	})
}

func (b *webBackend) MkdirAll(name string) error {
	if name == "." {
		return nil
	}
	records, err := b.records()
	if err != nil {
		return err
	}
	if err := validateWebDirectories(records, name); err != nil {
		return err
	}
	db, err := b.database()
	if err != nil {
		return err
	}
	return runTransaction(db, func(store js.Value) {
		b.putParents(store, name)
	})
}

func (b *webBackend) ReadDir(name string) ([]iofs.DirEntry, error) {
	info, err := b.Stat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, ErrInvalid
	}
	records, err := b.records()
	if err != nil {
		return nil, err
	}
	entries := make([]iofs.DirEntry, 0)
	for _, record := range records {
		if record.path == "." || path.Dir(record.path) != name {
			continue
		}
		entries = append(entries, dirEntry{info: record.info()})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	return entries, nil
}

func (b *webBackend) Stat(name string) (iofs.FileInfo, error) {
	if name == "." {
		return fileInfo{name: ".", mode: iofs.ModeDir | 0o755}, nil
	}
	record, found, err := b.record(name)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNotExist
	}
	return record.info(), nil
}

func (b *webBackend) Remove(name string) error {
	records, err := b.records()
	if err != nil {
		return err
	}
	var target *webRecord
	for i := range records {
		if records[i].path == name {
			target = &records[i]
			break
		}
	}
	if target == nil {
		return ErrNotExist
	}
	if target.directory {
		prefix := name + "/"
		for _, record := range records {
			if strings.HasPrefix(record.path, prefix) {
				return ErrNotEmpty
			}
		}
	}
	db, err := b.database()
	if err != nil {
		return err
	}
	return runTransaction(db, func(store js.Value) {
		store.Call("delete", b.key(name))
	})
}

func (b *webBackend) RemoveAll(name string) error {
	records, err := b.records()
	if err != nil {
		return err
	}
	prefix := name + "/"
	keys := make([]string, 0)
	for _, record := range records {
		if name == "." || record.path == name || strings.HasPrefix(record.path, prefix) {
			keys = append(keys, b.key(record.path))
		}
	}
	if len(keys) == 0 {
		return nil
	}
	db, err := b.database()
	if err != nil {
		return err
	}
	return runTransaction(db, func(store js.Value) {
		for _, key := range keys {
			store.Call("delete", key)
		}
	})
}

func (b *webBackend) Rename(oldName, newName string) error {
	records, err := b.records()
	if err != nil {
		return err
	}
	var found bool
	for _, record := range records {
		if record.path == newName {
			return ErrExist
		}
		if record.path == oldName {
			found = true
		}
	}
	if !found {
		return ErrNotExist
	}
	if err := validateWebDirectories(records, path.Dir(newName)); err != nil {
		return err
	}

	prefix := oldName + "/"
	moving := make([]webRecord, 0)
	for _, record := range records {
		if record.path != oldName && !strings.HasPrefix(record.path, prefix) {
			continue
		}
		record.path = newName + strings.TrimPrefix(record.path, oldName)
		moving = append(moving, record)
	}
	db, err := b.database()
	if err != nil {
		return err
	}
	return runTransaction(db, func(store js.Value) {
		b.putParents(store, path.Dir(newName))
		for _, record := range moving {
			oldPath := oldName + strings.TrimPrefix(record.path, newName)
			store.Call("delete", b.key(oldPath))
		}
		for _, record := range moving {
			store.Call("put", b.recordValue(record))
		}
	})
}

func (b *webBackend) database() (js.Value, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.db.Truthy() {
		return b.db, nil
	}
	indexedDB := js.Global().Get("indexedDB")
	if !indexedDB.Truthy() {
		return js.Undefined(), errors.New("IndexedDB is unavailable")
	}
	request := indexedDB.Call("open", indexedDBName, indexedDBVersion)
	upgrade := js.FuncOf(func(this js.Value, args []js.Value) any {
		database := request.Get("result")
		if !database.Get("objectStoreNames").Call("contains", indexedDBStore).Bool() {
			options := js.Global().Get("Object").New()
			options.Set("keyPath", "path")
			database.Call("createObjectStore", indexedDBStore, options)
		}
		return nil
	})
	request.Set("onupgradeneeded", upgrade)
	value, err := awaitRequest(request)
	upgrade.Release()
	if err != nil {
		return js.Undefined(), err
	}
	b.db = value
	return b.db, nil
}

func (b *webBackend) record(name string) (webRecord, bool, error) {
	db, err := b.database()
	if err != nil {
		return webRecord{}, false, err
	}
	request := db.Call("transaction", indexedDBStore, "readonly").Call("objectStore", indexedDBStore).Call("get", b.key(name))
	value, err := awaitRequest(request)
	if err != nil {
		return webRecord{}, false, err
	}
	if value.IsUndefined() {
		return webRecord{}, false, nil
	}
	return b.decodeRecord(value), true, nil
}

func (b *webBackend) records() ([]webRecord, error) {
	db, err := b.database()
	if err != nil {
		return nil, err
	}
	request := db.Call("transaction", indexedDBStore, "readonly").Call("objectStore", indexedDBStore).Call("getAll")
	value, err := awaitRequest(request)
	if err != nil {
		return nil, err
	}
	records := make([]webRecord, 0, value.Length())
	for i := 0; i < value.Length(); i++ {
		record := b.decodeRecord(value.Index(i))
		local, ok := b.localPath(record.path)
		if !ok {
			continue
		}
		record.path = local
		records = append(records, record)
	}
	return records, nil
}

func (b *webBackend) decodeRecord(value js.Value) webRecord {
	record := webRecord{
		path:      value.Get("path").String(),
		directory: value.Get("directory").Bool(),
		size:      int64(value.Get("size").Float()),
		modified:  int64(value.Get("modified").Float()),
	}
	data := value.Get("data")
	if data.Truthy() {
		record.data = make([]byte, data.Get("byteLength").Int())
		js.CopyBytesToGo(record.data, data)
	}
	return record
}

func (b *webBackend) recordValue(record webRecord) js.Value {
	value := js.Global().Get("Object").New()
	value.Set("path", b.key(record.path))
	value.Set("directory", record.directory)
	value.Set("size", float64(record.size))
	value.Set("modified", float64(record.modified))
	if !record.directory {
		data := js.Global().Get("Uint8Array").New(len(record.data))
		js.CopyBytesToJS(data, record.data)
		value.Set("data", data)
	}
	return value
}

func (b *webBackend) putParents(store js.Value, name string) {
	for name != "." {
		store.Call("put", b.recordValue(webRecord{
			path:      name,
			directory: true,
			modified:  time.Now().UnixMilli(),
		}))
		name = path.Dir(name)
	}
}

func (b *webBackend) key(name string) string {
	if b.root == "." {
		return name
	}
	if name == "." {
		return b.root
	}
	return b.root + "/" + name
}

func (b *webBackend) localPath(key string) (string, bool) {
	if b.root == "." {
		return key, true
	}
	if key == b.root {
		return ".", true
	}
	prefix := b.root + "/"
	if !strings.HasPrefix(key, prefix) {
		return "", false
	}
	return strings.TrimPrefix(key, prefix), true
}

func (r webRecord) info() iofs.FileInfo {
	mode := iofs.FileMode(0o644)
	if r.directory {
		mode = iofs.ModeDir | 0o755
	}
	return fileInfo{
		name:    path.Base(r.path),
		size:    r.size,
		mode:    mode,
		modTime: time.UnixMilli(r.modified),
	}
}

func validateWebDirectories(records []webRecord, name string) error {
	for name != "." {
		for _, record := range records {
			if record.path == name && !record.directory {
				return ErrInvalid
			}
		}
		name = path.Dir(name)
	}
	return nil
}

type jsResult struct {
	value js.Value
	err   error
}

func awaitRequest(request js.Value) (js.Value, error) {
	done := make(chan jsResult, 1)
	success := js.FuncOf(func(this js.Value, args []js.Value) any {
		done <- jsResult{value: request.Get("result")}
		return nil
	})
	failure := js.FuncOf(func(this js.Value, args []js.Value) any {
		done <- jsResult{err: indexedDBError(request.Get("error"))}
		return nil
	})
	request.Set("onsuccess", success)
	request.Set("onerror", failure)
	result := <-done
	success.Release()
	failure.Release()
	return result.value, result.err
}

func runTransaction(db js.Value, operations func(js.Value)) error {
	transaction := db.Call("transaction", indexedDBStore, "readwrite")
	done := make(chan error, 1)
	complete := js.FuncOf(func(this js.Value, args []js.Value) any {
		sendTransactionResult(done, nil)
		return nil
	})
	failure := js.FuncOf(func(this js.Value, args []js.Value) any {
		sendTransactionResult(done, indexedDBError(transaction.Get("error")))
		return nil
	})
	transaction.Set("oncomplete", complete)
	transaction.Set("onerror", failure)
	transaction.Set("onabort", failure)
	operations(transaction.Call("objectStore", indexedDBStore))
	err := <-done
	complete.Release()
	failure.Release()
	return err
}

func sendTransactionResult(done chan<- error, err error) {
	select {
	case done <- err:
	default:
	}
}

func indexedDBError(value js.Value) error {
	if !value.Truthy() {
		return errors.New("IndexedDB operation failed")
	}
	switch value.Get("name").String() {
	case "NotFoundError":
		return ErrNotExist
	case "ConstraintError":
		return ErrExist
	case "InvalidStateError":
		return ErrClosed
	default:
		return errors.New(value.Get("message").String())
	}
}
