//go:build js && wasm && wechat

package fs

import (
	"errors"
	"fmt"
	iofs "io/fs"
	"path"
	"sort"
	"strings"
	"syscall/js"
	"time"
)

type wechatBackend struct {
	manager js.Value
	root    string
}

func platformDefaultRoot() (string, error) {
	return ".", nil
}

func newPlatformBackend(root string) (backend, error) {
	if !iofs.ValidPath(root) || strings.ContainsRune(root, '\\') {
		return nil, fmt.Errorf("invalid virtual filesystem root %q", root)
	}
	wx := js.Global().Get("wx")
	if !wx.Truthy() {
		return nil, errors.New("WeChat runtime is unavailable")
	}
	manager := wx.Call("getFileSystemManager")
	base := wx.Get("env").Get("USER_DATA_PATH").String()
	physicalRoot := base
	if root != "." {
		physicalRoot = path.Join(base, root)
	}
	backend := &wechatBackend{manager: manager, root: physicalRoot}
	if _, err := callJS(func() js.Value {
		manager.Call("mkdirSync", physicalRoot, true)
		return js.Undefined()
	}); err != nil && !errors.Is(err, ErrExist) {
		return nil, fmt.Errorf("create WeChat filesystem root: %w", err)
	}
	return backend, nil
}

func (b *wechatBackend) Open(name string) (iofs.File, error) {
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

func (b *wechatBackend) ReadFile(name string) ([]byte, error) {
	value, err := callJS(func() js.Value {
		return b.manager.Call("readFileSync", b.resolve(name))
	})
	if err != nil {
		return nil, err
	}
	data := make([]byte, value.Get("byteLength").Int())
	js.CopyBytesToGo(data, value)
	return data, nil
}

func (b *wechatBackend) WriteFile(name string, data []byte) error {
	if err := b.MkdirAll(path.Dir(name)); err != nil {
		return err
	}
	value := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(value, data)
	_, err := callJS(func() js.Value {
		b.manager.Call("writeFileSync", b.resolve(name), value)
		return js.Undefined()
	})
	return err
}

func (b *wechatBackend) MkdirAll(name string) error {
	_, err := callJS(func() js.Value {
		b.manager.Call("mkdirSync", b.resolve(name), true)
		return js.Undefined()
	})
	if errors.Is(err, ErrExist) {
		info, statErr := b.Stat(name)
		if statErr == nil && info.IsDir() {
			return nil
		}
	}
	return err
}

func (b *wechatBackend) ReadDir(name string) ([]iofs.DirEntry, error) {
	value, err := callJS(func() js.Value {
		return b.manager.Call("readdirSync", b.resolve(name))
	})
	if err != nil {
		return nil, err
	}
	entries := make([]iofs.DirEntry, 0, value.Length())
	for i := 0; i < value.Length(); i++ {
		child := path.Join(name, value.Index(i).String())
		info, err := b.Stat(child)
		if err != nil {
			return nil, err
		}
		entries = append(entries, dirEntry{info: info})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	return entries, nil
}

func (b *wechatBackend) Stat(name string) (iofs.FileInfo, error) {
	value, err := callJS(func() js.Value {
		return b.manager.Call("statSync", b.resolve(name))
	})
	if err != nil {
		return nil, err
	}
	mode := iofs.FileMode(0o644)
	if value.Call("isDirectory").Bool() {
		mode = iofs.ModeDir | 0o755
	}
	size := int64(0)
	if field := value.Get("size"); field.Type() == js.TypeNumber {
		size = int64(field.Float())
	}
	modified := time.Time{}
	if field := value.Get("lastModifiedTime"); field.Type() == js.TypeNumber {
		modified = time.UnixMilli(int64(field.Float()))
	}
	return fileInfo{
		name:    path.Base(name),
		size:    size,
		mode:    mode,
		modTime: modified,
	}, nil
}

func (b *wechatBackend) Remove(name string) error {
	info, err := b.Stat(name)
	if err != nil {
		return err
	}
	if info.IsDir() {
		entries, err := b.ReadDir(name)
		if err != nil {
			return err
		}
		if len(entries) != 0 {
			return ErrNotEmpty
		}
		_, err = callJS(func() js.Value {
			b.manager.Call("rmdirSync", b.resolve(name), false)
			return js.Undefined()
		})
		return err
	}
	_, err = callJS(func() js.Value {
		b.manager.Call("unlinkSync", b.resolve(name))
		return js.Undefined()
	})
	return err
}

func (b *wechatBackend) RemoveAll(name string) error {
	if name == "." {
		entries, err := b.ReadDir(name)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := b.RemoveAll(entry.Name()); err != nil {
				return err
			}
		}
		return nil
	}
	info, err := b.Stat(name)
	if errors.Is(err, ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	method := "unlinkSync"
	args := []any{b.resolve(name)}
	if info.IsDir() {
		method = "rmdirSync"
		args = append(args, true)
	}
	_, err = callJS(func() js.Value {
		b.manager.Call(method, args...)
		return js.Undefined()
	})
	return err
}

func (b *wechatBackend) Rename(oldName, newName string) error {
	if _, err := b.Stat(oldName); err != nil {
		return err
	}
	if _, err := b.Stat(newName); err == nil {
		return ErrExist
	} else if !errors.Is(err, ErrNotExist) {
		return err
	}
	if err := b.MkdirAll(path.Dir(newName)); err != nil {
		return err
	}
	_, err := callJS(func() js.Value {
		b.manager.Call("renameSync", b.resolve(oldName), b.resolve(newName))
		return js.Undefined()
	})
	return err
}

func (b *wechatBackend) resolve(name string) string {
	if name == "." {
		return b.root
	}
	return path.Join(b.root, name)
}

func callJS[T any](operation func() T) (value T, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = wechatError(fmt.Sprint(recovered))
		}
	}()
	return operation(), nil
}

func wechatError(message string) error {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "no such file"), strings.Contains(lower, "not exist"), strings.Contains(lower, "enoent"):
		return ErrNotExist
	case strings.Contains(lower, "already exists"), strings.Contains(lower, "eexist"):
		return ErrExist
	case strings.Contains(lower, "not a directory"), strings.Contains(lower, "is a directory"):
		return ErrInvalid
	default:
		return errors.New(message)
	}
}
