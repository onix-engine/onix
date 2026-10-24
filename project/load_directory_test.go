package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDirectoryFromManifestFindsModuleRoot(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "apps", "game")
	if err := os.MkdirAll(filepath.Join(projectDir, "resources", "scenes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/game\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "resources", "project.json"), []byte(`{"engine":{"features":["render2d","input2d","ui-controls"],"resources":["images","fonts"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "resources", "resources.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "resources", "scenes.json"), []byte(`{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "resources", "scenes", "main.scene.json"), []byte(`{"nodes":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	bundle, err := LoadDirectoryFromManifest[ResourceFeature](projectDir)
	if err != nil {
		t.Fatalf("load project directory: %v", err)
	}
	defer bundle.Dispose()
	if bundle.EntrySceneID() != "main" {
		t.Fatalf("entry scene = %q, want main", bundle.EntrySceneID())
	}
}

func TestLoadDirectoryWithRender2DProfile(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "apps", "game")
	resourceDir := filepath.Join(projectDir, "resources")
	sceneDir := filepath.Join(resourceDir, "scenes")
	if err := os.MkdirAll(sceneDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/game\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resourceDir, "project.json"), []byte(`{"engine":{"features":["render2d","animations2d"],"resources":["fonts","shaders"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resourceDir, "resources.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resourceDir, "scenes.json"), []byte(`{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sceneDir, "main.scene.json"), []byte(`{"nodes":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	bundle, err := LoadDirectoryWithProfile[Render2DAnimationsProfile, ResourceFeature](projectDir)
	if err != nil {
		t.Fatalf("load static 2D profile: %v", err)
	}
	defer bundle.Dispose()
	if bundle.EntrySceneID() != "main" {
		t.Fatalf("entry scene = %q, want main", bundle.EntrySceneID())
	}

	if err := os.WriteFile(filepath.Join(resourceDir, "resources.json"), []byte(`{"html":[{"id":"screen","path":"screen.html"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDirectoryWithProfile[Render2DAnimationsProfile, ResourceFeature](projectDir); err == nil {
		t.Fatal("static 2D profile accepted HTML resources")
	}
}
