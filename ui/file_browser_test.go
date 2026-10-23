package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileBrowserListsDirectoriesThenFilteredFiles(t *testing.T) {
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "Folder"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"z.txt", "a.json", "skip.png"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	browser, err := NewFileBrowser(directory, "json", ".txt", "JSON")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := browser.Entries()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || !entries[0].IsDir || entries[0].Name != "Folder" || entries[1].Name != "a.json" || entries[2].Name != "z.txt" {
		t.Fatalf("entries = %#v", entries)
	}
	entries[0].Name = "mutated"
	again, _ := browser.Entries()
	if again[0].Name != "Folder" {
		t.Fatal("Entries returned mutable cached state")
	}
}

func TestFileBrowserNavigatesAndSelectsFiles(t *testing.T) {
	directory := t.TempDir()
	child := filepath.Join(directory, "child")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(child, "state.json")
	if err := os.WriteFile(file, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	browser, err := NewFileBrowser(directory, ".json")
	if err != nil {
		t.Fatal(err)
	}
	if err := browser.Select(child); err != nil || browser.CurrentDir != child || browser.SelectedPath != "" {
		t.Fatalf("open child: browser=%#v err=%v", browser, err)
	}
	selected, err := browser.OpenValue("state.json")
	if err != nil || selected != file {
		t.Fatalf("selected=%q err=%v", selected, err)
	}
	if err := browser.OpenParent(); err != nil || browser.CurrentDir != directory {
		t.Fatalf("parent=%q err=%v", browser.CurrentDir, err)
	}
	if _, err := browser.OpenValue(child); err == nil || browser.CurrentDir != child {
		t.Fatalf("directory confirmation: current=%q err=%v", browser.CurrentDir, err)
	}
	if err := browser.OpenParent(); err != nil {
		t.Fatal(err)
	}
	if _, err := browser.OpenValue(filepath.Join(directory, "missing.json")); err == nil {
		t.Fatal("missing file was selected")
	}
}

func TestFileBrowserResolvesSavePathsAndExtensions(t *testing.T) {
	directory := t.TempDir()
	browser, err := NewFileBrowser(directory, ".json")
	if err != nil {
		t.Fatal(err)
	}
	path, err := browser.SaveValue("backup")
	if err != nil || path != filepath.Join(directory, "backup.json") {
		t.Fatalf("path=%q err=%v", path, err)
	}
	if _, err := browser.SaveValue("backup.txt"); err == nil {
		t.Fatal("disallowed save extension was accepted")
	}
	if _, err := browser.SaveValue(""); err == nil {
		t.Fatal("empty save path was accepted")
	}
}

func TestFileBrowserStartsFromFileAndFollowsDirectorySymlink(t *testing.T) {
	directory := t.TempDir()
	file := filepath.Join(directory, "input.txt")
	if err := os.WriteFile(file, []byte("input"), 0o600); err != nil {
		t.Fatal(err)
	}
	browser, err := NewFileBrowser(file, ".txt")
	if err != nil || browser.CurrentDir != directory || browser.SelectedPath != file {
		t.Fatalf("browser=%#v err=%v", browser, err)
	}
	target := filepath.Join(directory, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(directory, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	entries, err := browser.Entries()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name == "linked" && entry.IsDir {
			return
		}
	}
	t.Fatal("directory symlink was not listed as a directory")
}
