package window

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestNormalizeFileDialogOptions(t *testing.T) {
	options, err := normalizeFileDialogOptions(FileDialogOptions{
		DefaultPath: "./assets",
		Mode:        FileDialogMultipleFiles,
		Filters:     []FileDialogFilter{{Name: "Images", Extensions: []string{"*.png", ".jpg"}}},
	})
	if err != nil {
		t.Fatalf("normalize file dialog options: %v", err)
	}
	if options.Mode != FileDialogMultipleFiles {
		t.Fatalf("multiple options = mode %d", options.Mode)
	}
	if options.DefaultPath != filepath.Clean("./assets") {
		t.Fatalf("default path = %q", options.DefaultPath)
	}
	if options.Title != "Select files" {
		t.Fatalf("default title = %q", options.Title)
	}
}

func TestNormalizeFileDialogOptionsRejectsInvalidValues(t *testing.T) {
	cases := []FileDialogOptions{
		{Mode: FileDialogMode(99)},
		{Filters: []FileDialogFilter{{Name: "Images"}}},
		{Filters: []FileDialogFilter{{Name: "Images", Extensions: []string{"png;jpg"}}}},
		{Filters: []FileDialogFilter{{Name: "Images", Extensions: []string{"png/jpg"}}}},
		{Filters: []FileDialogFilter{{Name: "Images", Extensions: []string{`png\jpg`}}}},
		{Filters: []FileDialogFilter{{Name: "Images", Extensions: []string{"png\x00jpg"}}}},
	}
	for index, options := range cases {
		if _, err := normalizeFileDialogOptions(options); !errors.Is(err, ErrFileDialogInvalidOptions) {
			t.Errorf("case %d error = %v, want ErrFileDialogInvalidOptions", index, err)
		}
	}
}

func TestFileDialogFiltersReachBackend(t *testing.T) {
	original := fileDialogBackend
	t.Cleanup(func() { RegisterFileDialogBackend(original) })
	const selected = "tileset.png"
	calls := 0
	RegisterFileDialogBackend(func(_ *Context, options FileDialogOptions) ([]string, error) {
		calls++
		if options.Mode != FileDialogFiles || len(options.Filters) != 1 || len(options.Filters[0].Extensions) != 7 {
			t.Fatalf("unexpected dialog options: %+v", options)
		}
		return []string{selected}, nil
	})
	path, err := OpenFile(FileDialogOptions{
		Filters: []FileDialogFilter{{Name: "Images", Extensions: []string{"png", "ktx", "*.ktx2", "ontex", ".exr", "fbx", "jpeg2000"}}},
	})
	if err != nil || path != selected || calls != 1 {
		t.Fatalf("file picker = %q, %v; backend calls = %d, want %q, nil, 1", path, err, calls, selected)
	}
}

func TestFileDialogDefaultTitles(t *testing.T) {
	for mode, want := range map[FileDialogMode]string{
		FileDialogFiles:         "Select a file",
		FileDialogFolder:        "Select a folder",
		FileDialogMultipleFiles: "Select files",
	} {
		options, err := normalizeFileDialogOptions(FileDialogOptions{Mode: mode})
		if err != nil {
			t.Fatal(err)
		}
		if options.Title != want {
			t.Fatalf("mode %d title = %q, want %q", mode, options.Title, want)
		}
	}
}

func TestFileDialogGlob(t *testing.T) {
	for _, extension := range []string{"png", ".png", "*.png"} {
		if got := fileDialogGlob(extension); got != "*.png" {
			t.Fatalf("file dialog glob for %q = %q, want %q", extension, got, "*.png")
		}
	}
}
