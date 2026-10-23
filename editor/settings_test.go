package editor

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestEditorSettingsRememberAndForgetRecentProjects(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	settings := editorSettings{}
	directories := make([]string, recentProjectLimit+2)
	for index := range directories {
		directories[index] = filepath.Join(home, fmt.Sprintf("project-%02d", index))
		if err := ensureDir(directories[index]); err != nil {
			t.Fatalf("create project directory: %v", err)
		}
		if err := settings.rememberRecentProject(directories[index]); err != nil {
			t.Fatalf("remember recent project: %v", err)
		}
	}

	if len(settings.RecentProjects) != recentProjectLimit {
		t.Fatalf("recent project count = %d, want %d", len(settings.RecentProjects), recentProjectLimit)
	}
	if got, want := settings.RecentProjects[0], directories[len(directories)-1]; got != want {
		t.Fatalf("newest recent project = %q, want %q", got, want)
	}
	if slices.Contains(settings.RecentProjects, directories[0]) {
		t.Fatalf("oldest project was not removed: %v", settings.RecentProjects)
	}

	moved := settings.RecentProjects[len(settings.RecentProjects)-1]
	if err := settings.rememberRecentProject(moved); err != nil {
		t.Fatalf("move recent project to front: %v", err)
	}
	if settings.RecentProjects[0] != moved || len(settings.RecentProjects) != recentProjectLimit {
		t.Fatalf("recent project was not moved to front without duplication: %v", settings.RecentProjects)
	}

	loaded, err := loadEditorSettings()
	if err != nil {
		t.Fatalf("load editor settings: %v", err)
	}
	if !slices.Equal(loaded.RecentProjects, settings.RecentProjects) {
		t.Fatalf("persisted recent projects = %v, want %v", loaded.RecentProjects, settings.RecentProjects)
	}
	if err := settings.forgetRecentProject(moved); err != nil {
		t.Fatalf("forget recent project: %v", err)
	}
	if slices.Contains(settings.RecentProjects, moved) {
		t.Fatalf("forgotten project remains in settings: %v", settings.RecentProjects)
	}
	loaded, err = loadEditorSettings()
	if err != nil {
		t.Fatalf("reload editor settings: %v", err)
	}
	if !slices.Equal(loaded.RecentProjects, settings.RecentProjects) {
		t.Fatalf("forgotten project was not persisted: %v", loaded.RecentProjects)
	}
}

func TestEditorSettingsPersistGridSnapPreference(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	settings := editorSettings{SnapToGrid: true}
	if err := saveEditorSettings(settings); err != nil {
		t.Fatalf("save editor settings: %v", err)
	}
	loaded, err := loadEditorSettings()
	if err != nil {
		t.Fatalf("load editor settings: %v", err)
	}
	if !loaded.SnapToGrid {
		t.Fatal("grid snap preference was not persisted")
	}
	path, err := editorSettingsPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"snapToGrid":true,"viewport3D":{"ground":true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err = loadEditorSettings()
	if err != nil || !loaded.SnapToGrid {
		t.Fatalf("unused settings prevented loading grid snap: %+v, %v", loaded, err)
	}
}
