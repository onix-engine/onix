package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

const MaxFileBrowserEntries = 2000

type FileBrowserEntry struct {
	Name  string
	Path  string
	IsDir bool
}

type FileBrowser struct {
	CurrentDir   string
	SelectedPath string
	Extensions   []string
	Truncated    bool

	loadedDir        string
	loadedExtensions []string
	entries          []FileBrowserEntry
}

func NewFileBrowser(start string, extensions ...string) (FileBrowser, error) {
	start = strings.TrimSpace(start)
	if start == "" {
		start = "."
	}
	absolute, err := filepath.Abs(start)
	if err != nil {
		return FileBrowser{}, fmt.Errorf("resolve file browser path: %w", err)
	}
	selected := ""
	info, err := os.Stat(absolute)
	if err == nil && !info.IsDir() {
		selected = filepath.Clean(absolute)
		absolute = filepath.Dir(absolute)
	} else if err != nil {
		absolute = filepath.Dir(absolute)
		if info, statErr := os.Stat(absolute); statErr != nil || !info.IsDir() {
			return FileBrowser{}, fmt.Errorf("open file browser directory: %w", err)
		}
	}
	browser := FileBrowser{CurrentDir: filepath.Clean(absolute), SelectedPath: selected}
	browser.Extensions = normalizeFileBrowserExtensions(extensions)
	return browser, nil
}

func (browser *FileBrowser) OpenDirectory(path string) error {
	if browser == nil {
		return errors.New("file browser is nil")
	}
	absolute, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return fmt.Errorf("resolve directory: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return fmt.Errorf("stat directory: %w", err)
	}
	if !info.IsDir() {
		return errors.New("selected path is not a directory")
	}
	browser.CurrentDir = filepath.Clean(absolute)
	browser.SelectedPath = ""
	browser.invalidate()
	return nil
}

func (browser *FileBrowser) OpenParent() error {
	if browser == nil {
		return errors.New("file browser is nil")
	}
	parent := filepath.Dir(browser.CurrentDir)
	if parent == browser.CurrentDir {
		return nil
	}
	return browser.OpenDirectory(parent)
}

func (browser *FileBrowser) Select(path string) error {
	if browser == nil {
		return errors.New("file browser is nil")
	}
	absolute, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return fmt.Errorf("resolve selected file: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return fmt.Errorf("stat selected file: %w", err)
	}
	if info.IsDir() {
		return browser.OpenDirectory(absolute)
	}
	if !info.Mode().IsRegular() {
		return errors.New("selected path is not a regular file")
	}
	if !browser.acceptsExtension(absolute) {
		return fmt.Errorf("file type %q is not available in this browser", filepath.Ext(absolute))
	}
	browser.SelectedPath = filepath.Clean(absolute)
	return nil
}

func (browser *FileBrowser) Entries() ([]FileBrowserEntry, error) {
	if browser == nil {
		return nil, errors.New("file browser is nil")
	}
	if browser.loadedDir == browser.CurrentDir && slices.Equal(browser.loadedExtensions, browser.Extensions) {
		return slices.Clone(browser.entries), nil
	}
	entries, err := os.ReadDir(browser.CurrentDir)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}
	sort.Slice(entries, func(left, right int) bool {
		leftDir := fileBrowserEntryIsDir(browser.CurrentDir, entries[left])
		rightDir := fileBrowserEntryIsDir(browser.CurrentDir, entries[right])
		if leftDir != rightDir {
			return leftDir
		}
		return strings.ToLower(entries[left].Name()) < strings.ToLower(entries[right].Name())
	})
	rows := make([]FileBrowserEntry, 0, min(len(entries), MaxFileBrowserEntries))
	truncated := false
	for _, entry := range entries {
		path := filepath.Join(browser.CurrentDir, entry.Name())
		isDir := fileBrowserEntryIsDir(browser.CurrentDir, entry)
		if !isDir && !browser.acceptsExtension(path) {
			continue
		}
		if len(rows) == MaxFileBrowserEntries {
			truncated = true
			break
		}
		rows = append(rows, FileBrowserEntry{Name: entry.Name(), Path: path, IsDir: isDir})
	}
	browser.loadedDir = browser.CurrentDir
	browser.loadedExtensions = slices.Clone(browser.Extensions)
	browser.entries = rows
	browser.Truncated = truncated
	return slices.Clone(rows), nil
}

func (browser *FileBrowser) OpenValue(value string) (string, error) {
	if browser == nil {
		return "", errors.New("file browser is nil")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		value = browser.SelectedPath
	}
	if value == "" {
		return "", errors.New("no file is selected")
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(browser.CurrentDir, value)
	}
	if err := browser.Select(value); err != nil {
		return "", err
	}
	if browser.SelectedPath == "" {
		return "", errors.New("selected path is a directory")
	}
	return browser.SelectedPath, nil
}

func (browser *FileBrowser) SaveValue(value string) (string, error) {
	if browser == nil {
		return "", errors.New("file browser is nil")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("file name is empty")
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(browser.CurrentDir, value)
	}
	if filepath.Ext(value) == "" && len(browser.Extensions) == 1 {
		value += browser.Extensions[0]
	}
	if !browser.acceptsExtension(value) {
		return "", fmt.Errorf("file type %q is not available in this browser", filepath.Ext(value))
	}
	absolute, err := filepath.Abs(filepath.Clean(value))
	if err != nil {
		return "", fmt.Errorf("resolve save path: %w", err)
	}
	parent := filepath.Dir(absolute)
	info, err := os.Stat(parent)
	if err != nil {
		return "", fmt.Errorf("stat save directory: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("save path parent is not a directory")
	}
	if info, err := os.Stat(absolute); err == nil && info.IsDir() {
		return "", errors.New("save path is a directory")
	}
	return absolute, nil
}

func (browser *FileBrowser) acceptsExtension(path string) bool {
	return len(browser.Extensions) == 0 || slices.Contains(browser.Extensions, strings.ToLower(filepath.Ext(path)))
}

func (browser *FileBrowser) invalidate() {
	browser.loadedDir = ""
	browser.loadedExtensions = nil
	browser.entries = nil
	browser.Truncated = false
}

func normalizeFileBrowserExtensions(extensions []string) []string {
	result := make([]string, 0, len(extensions))
	for _, extension := range extensions {
		extension = strings.ToLower(strings.TrimSpace(extension))
		if extension == "" {
			continue
		}
		if !strings.HasPrefix(extension, ".") {
			extension = "." + extension
		}
		if !slices.Contains(result, extension) {
			result = append(result, extension)
		}
	}
	return result
}

func fileBrowserEntryIsDir(directory string, entry os.DirEntry) bool {
	if entry.IsDir() {
		return true
	}
	if entry.Type()&os.ModeSymlink == 0 {
		return false
	}
	info, err := os.Stat(filepath.Join(directory, entry.Name()))
	return err == nil && info.IsDir()
}
